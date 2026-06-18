package hub

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

var builtinQueries = map[string]struct{}{
	"code-hygiene-cross": {},
	"similar-feedback":   {},
	"open-debt":          {},
}

// BuiltinQueryNames returns supported cross-project query names.
func BuiltinQueryNames() []string {
	names := make([]string, 0, len(builtinQueries))
	for n := range builtinQueries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// AcrossOptions filters built-in cross-project queries.
type AcrossOptions struct {
	Query    string
	Keyword  string
	Category string
	Registry string
}

// Across runs a built-in query across registered projects.
func Across(opts AcrossOptions) ([]map[string]any, error) {
	if _, ok := builtinQueries[opts.Query]; !ok {
		return nil, fmt.Errorf("unknown query %q (try: %s)", opts.Query, strings.Join(BuiltinQueryNames(), ", "))
	}
	projects, err := ReadRegistry(opts.Registry)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	for _, p := range projects {
		if !p.Exists {
			continue
		}
		part, err := queryProject(p, opts)
		if err != nil {
			continue
		}
		rows = append(rows, part...)
	}
	if opts.Query == "code-hygiene-cross" {
		sort.Slice(rows, func(i, j int) bool {
			si := severityRank(fmt.Sprint(rows[i]["severity"]))
			sj := severityRank(fmt.Sprint(rows[j]["severity"]))
			if si != sj {
				return si > sj
			}
			return fmt.Sprint(rows[i]["created_at"]) > fmt.Sprint(rows[j]["created_at"])
		})
	}
	return rows, nil
}

func queryProject(p RegisteredProject, opts AcrossOptions) ([]map[string]any, error) {
	db, err := storage.Open(p.DBPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	switch opts.Query {
	case "code-hygiene-cross":
		return hygieneCross(db, p.Alias)
	case "similar-feedback":
		return similarFeedback(db, p.Alias, opts.Keyword, opts.Category)
	case "open-debt":
		return openDebt(db, p.Alias)
	default:
		return nil, fmt.Errorf("unknown query")
	}
}

func hygieneCross(db *sql.DB, alias string) ([]map[string]any, error) {
	rows, err := db.Query(`
		SELECT file_path, line_start, principle, title, severity, created_at
		FROM review_findings
		WHERE principle IN ('dry','kiss','separation-of-concerns','yagni')
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectRows(rows, alias)
}

func similarFeedback(db *sql.DB, alias, keyword, category string) ([]map[string]any, error) {
	clauses := []string{}
	var params []any
	if category != "" {
		clauses = append(clauses, "category = ?")
		params = append(params, category)
	}
	if keyword != "" {
		clauses = append(clauses, "(note LIKE ? OR context LIKE ?)")
		params = append(params, "%"+keyword+"%", "%"+keyword+"%")
	}
	sqlText := `SELECT role, category, severity, note, context, created_at FROM feedback`
	if len(clauses) > 0 {
		sqlText += " WHERE " + strings.Join(clauses, " AND ")
	}
	sqlText += " ORDER BY created_at DESC"
	rows, err := db.Query(sqlText, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectRows(rows, alias)
}

func openDebt(db *sql.DB, alias string) ([]map[string]any, error) {
	rows, err := db.Query(`
		SELECT 'finding' AS kind, file_path AS target, severity, title AS summary, created_at
		FROM review_findings
		WHERE status='open' AND severity IN ('high','critical')
		UNION ALL
		SELECT 'feedback' AS kind, category AS target, severity, note AS summary, created_at
		FROM feedback
		WHERE status='open' AND severity IN ('high','critical')
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectRows(rows, alias)
}

func scanProjectRows(rows *sql.Rows, alias string) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{"project": alias}
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				row[c] = string(v)
			default:
				row[c] = v
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func severityRank(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 5
	case "high":
		return 4
	case "med", "medium":
		return 3
	case "low":
		return 2
	default:
		return 1
	}
}
