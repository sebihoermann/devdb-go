package catalog

import (
	"database/sql"
	"fmt"
	"strings"
)

// Allowed tables for list/show commands.
var AllowedTables = map[string]bool{
	"goals": true, "feedback": true, "features": true, "plans": true,
	"plan_items": true, "tasks": true, "reminders": true,
}

// ListRows returns raw rows from an allowed table.
func ListRows(db *sql.DB, table string, limit int) ([]map[string]any, error) {
	if !AllowedTables[table] {
		return nil, fmt.Errorf("unknown table %q — allowed: %s", table, strings.Join(keys(), ", "))
	}
	q := fmt.Sprintf(`SELECT * FROM %s ORDER BY rowid DESC`, table)
	args := []any{}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
		row := map[string]any{}
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

// ShowRow returns one row by id prefix.
func ShowRow(db *sql.DB, table, idPrefix string) (map[string]any, error) {
	if !AllowedTables[table] {
		return nil, fmt.Errorf("unknown table %q", table)
	}
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM %s`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	prefix := strings.ToLower(idPrefix)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		var id string
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				row[c] = string(v)
			default:
				row[c] = v
			}
			if c == "id" {
				id = fmt.Sprint(row[c])
			}
		}
		if strings.HasPrefix(strings.ToLower(id), prefix) || strings.ToLower(id) == prefix {
			return row, nil
		}
	}
	return nil, fmt.Errorf("no row in %s matching %q", table, idPrefix)
}

func keys() []string {
	out := make([]string, 0, len(AllowedTables))
	for k := range AllowedTables {
		out = append(out, k)
	}
	return out
}
