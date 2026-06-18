package storage

import (
	"database/sql"
	"fmt"
)

// SchemaKind distinguishes Go-native vs legacy Python databases.
type SchemaKind string

const (
	SchemaUnknown SchemaKind = "unknown"
	SchemaGo      SchemaKind = "go"
	SchemaPython  SchemaKind = "python"
)

// DetectSchema inspects migration metadata to classify the database.
func DetectSchema(db *sql.DB) (SchemaKind, int, error) {
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'`,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return SchemaUnknown, 0, nil
	}
	if err != nil {
		return SchemaUnknown, 0, err
	}

	var hasGoMarker int
	err = db.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE description LIKE 'go:%'`,
	).Scan(&hasGoMarker)
	if err != nil {
		return SchemaUnknown, 0, err
	}
	if hasGoMarker > 0 {
		var version int
		_ = db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
		return SchemaGo, version, nil
	}

	var version int
	err = db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return SchemaUnknown, 0, err
	}
	if version > 0 {
		return SchemaPython, version, nil
	}
	return SchemaUnknown, 0, nil
}

// TableExists reports whether a table is present.
func TableExists(db *sql.DB, table string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
		table,
	).Scan(&n)
	return n > 0, err
}

// AppendLimit adds " LIMIT ?" when limit > 0. Zero means no row cap (global --all).
func AppendLimit(query string, args []any, limit int) (string, []any) {
	if limit > 0 {
		return query + " LIMIT ?", append(args, limit)
	}
	return query, args
}

// ColumnNames returns column names for a table.
func ColumnNames(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}
