package testutil

import (
	"testing"
)

func TestClosedDBReturnsClosedConnection(t *testing.T) {
	db := ClosedDB(t)
	if db == nil {
		t.Fatal("nil db")
	}
	if err := db.Ping(); err == nil {
		t.Fatal("expected closed db ping error")
	}
}
