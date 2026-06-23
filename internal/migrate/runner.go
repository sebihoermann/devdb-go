package migrate

import (
	"database/sql"
	"fmt"
	"strings"
)

// Migration is a versioned schema change.
type Migration struct {
	Version     int
	Description string
	Apply       func(*sql.Tx) error
}

// RunAll applies pending migrations.
func RunAll(db *sql.DB) error {
	if err := validateMigrations("source", SourceMigrations); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL,
			description TEXT NOT NULL
		)`); err != nil {
		return err
	}

	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, m := range SourceMigrations {
		if applied[m.Version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := m.Apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at, description) VALUES (?, datetime('now'), ?)`,
			m.Version, m.Description,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// RunHub applies pending hub metadata migrations.
func RunHub(db *sql.DB) error {
	if err := validateMigrations("hub", HubMigrations); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL,
			description TEXT NOT NULL
		)`); err != nil {
		return err
	}

	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, m := range HubMigrations {
		if applied[m.Version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := m.Apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("hub migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at, description) VALUES (?, datetime('now'), ?)`,
			m.Version, m.Description,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func validateMigrations(name string, migrations []Migration) error {
	previous := 0
	for i, migration := range migrations {
		if migration.Version <= 0 {
			return fmt.Errorf("%s migration at index %d has non-positive version %d", name, i, migration.Version)
		}
		if migration.Version <= previous {
			return fmt.Errorf("%s migration version %d at index %d must be greater than previous version %d", name, migration.Version, i, previous)
		}
		previous = migration.Version
	}
	return nil
}

func execStatements(tx *sql.Tx, script string) error {
	stmt := ""
	for _, part := range splitStatements(script) {
		stmt = part
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// splitStatements breaks a SQL script into individual statements on `;` while
// honoring SQL lexical rules: single-quoted string literals (with `''` as the
// SQL-standard escape), double-quoted identifiers (with `""` as the escape),
// and `--` line comments. Tokens inside any of those constructs never split.
// Block comments /* ... */ are tolerated inside one statement (their `;` and
// `'` are not interpreted) but are otherwise inert.
func splitStatements(script string) []string {
	var out []string
	var b strings.Builder
	state := sqlStateCode
	for i := 0; i < len(script); i++ {
		ch := script[i]
		switch state {
		case sqlStateCode:
			switch ch {
			case '\'':
				state = sqlStateSingleQuote
				b.WriteByte(ch)
			case '"':
				state = sqlStateDoubleQuote
				b.WriteByte(ch)
			case '-':
				if i+1 < len(script) && script[i+1] == '-' {
					state = sqlStateLineComment
					b.WriteByte(ch)
					continue
				}
				b.WriteByte(ch)
			case '/':
				if i+1 < len(script) && script[i+1] == '*' {
					state = sqlStateBlockComment
					b.WriteByte(ch)
					b.WriteByte(script[i+1])
					i++
					continue
				}
				b.WriteByte(ch)
			case ';':
				if stmt := strings.TrimSpace(b.String()); stmt != "" {
					out = append(out, stmt)
				}
				b.Reset()
			default:
				b.WriteByte(ch)
			}
		case sqlStateSingleQuote:
			if ch == '\'' {
				if i+1 < len(script) && script[i+1] == '\'' {
					b.WriteByte(ch)
					b.WriteByte(script[i+1])
					i++
					continue
				}
				b.WriteByte(ch)
				state = sqlStateCode
				continue
			}
			b.WriteByte(ch)
		case sqlStateDoubleQuote:
			if ch == '"' {
				if i+1 < len(script) && script[i+1] == '"' {
					b.WriteByte(ch)
					b.WriteByte(script[i+1])
					i++
					continue
				}
				b.WriteByte(ch)
				state = sqlStateCode
				continue
			}
			b.WriteByte(ch)
		case sqlStateLineComment:
			if ch == '\n' {
				b.WriteByte(ch)
				state = sqlStateCode
				continue
			}
			b.WriteByte(ch)
		case sqlStateBlockComment:
			b.WriteByte(ch)
			if ch == '*' && i+1 < len(script) && script[i+1] == '/' {
				b.WriteByte('/')
				i++
				state = sqlStateCode
			}
		}
	}
	if stmt := strings.TrimSpace(b.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}

type sqlLexState int

const (
	sqlStateCode sqlLexState = iota
	sqlStateSingleQuote
	sqlStateDoubleQuote
	sqlStateLineComment
	sqlStateBlockComment
)
