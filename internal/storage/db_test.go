package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenAndWithTx(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := WithTx(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE tx_test (id INTEGER)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='tx_test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("commit failed: n=%d err=%v", n, err)
	}

	err = WithTx(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE tx_rollback (id INTEGER)`)
		if err != nil {
			return err
		}
		return errors.New("rollback me")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='tx_rollback'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rollback failed: n=%d err=%v", n, err)
	}
}

func TestNowUTC(t *testing.T) {
	ts := NowUTC()
	if ts == "" {
		t.Fatal("empty timestamp")
	}
}
