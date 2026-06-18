package approval

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestApprovalFlow(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := Request(db, "bad_table", "x", "", "test"); err == nil {
		t.Fatal("expected invalid table")
	}
	taskID, err := tasks.Add(db, "Needs approval", "", "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Request(db, "tasks", taskID, "please review", "test"); err != nil {
		t.Fatal(err)
	}
	pending, err := ListPending(db)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %d err=%v", len(pending), err)
	}
	if _, err := Approve(db, "tasks", taskID, "lgtm", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := Reject(db, "tasks", taskID, "nope", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := Withdraw(db, "tasks", taskID, "", "test"); err != nil {
		t.Fatal(err)
	}
	logs, err := Log(db, 10)
	if err != nil || len(logs) < 4 {
		t.Fatalf("log: %d err=%v", len(logs), err)
	}
	logs, err = Log(db, -1)
	if err != nil || len(logs) < 4 {
		t.Fatalf("default log limit: %d", len(logs))
	}
}
