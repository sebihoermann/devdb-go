package importer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ArchiveSpec describes one archived python-only table.
type ArchiveSpec struct {
	Table string `json:"table"`
	Path  string `json:"path"`
	Rows  int    `json:"rows"`
}

// ArchivePythonOnly copies rows from every populated python-only table in srcDB
// to <archiveDir>/<table>.jsonl. Tables that do not exist in srcDB or that have
// zero rows are skipped silently. Returns the list of archived tables.
func ArchivePythonOnly(srcDB *sql.DB, archiveDir string) ([]ArchiveSpec, error) {
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive dir: %w", err)
	}
	var out []ArchiveSpec
	for _, table := range pythonOnlyTables {
		exists, err := tableExists(srcDB, table)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", table, err)
		}
		if !exists {
			continue
		}
		count, err := rowCount(srcDB, table)
		if err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		if count == 0 {
			continue
		}
		path, err := writeTableJSONL(srcDB, table, filepath.Join(archiveDir, table+".jsonl"))
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", table, err)
		}
		out = append(out, ArchiveSpec{Table: table, Path: path, Rows: count})
	}
	return out, nil
}

func tableExists(db *sql.DB, table string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
	return n == 1, err
}

func rowCount(db *sql.DB, table string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM "` + table + `"`).Scan(&n)
	return n, err
}

func writeTableJSONL(db *sql.DB, table, path string) (string, error) {
	rows, err := db.Query(`SELECT * FROM "` + table + `"`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	written := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		rec := map[string]any{}
		for i, c := range cols {
			rec[c] = vals[i]
		}
		if err := enc.Encode(rec); err != nil {
			return "", err
		}
		written++
	}
	return path, rows.Err()
}
