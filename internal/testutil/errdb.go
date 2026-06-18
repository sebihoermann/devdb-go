package testutil

import (
	"database/sql"
	"testing"
)

// ClosedDB returns a migrated database that has been closed (for error-path tests).
func ClosedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := TempDB(t)
	_ = db.Close()
	return db
}
