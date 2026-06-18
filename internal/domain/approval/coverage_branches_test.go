package approval

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestApprovalTaskRejectWithdraw(t *testing.T) {
	db, _ := testutil.TempDB(t)
	taskID, _ := tasks.Add(db, "task", "", "med", "", "test")
	if _, err := Request(db, "tasks", taskID, "please", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := Reject(db, "tasks", taskID, "no", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := Withdraw(db, "tasks", taskID, "clear", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := Request(db, "bad_table", taskID, "", "test"); err == nil {
		t.Fatal("bad table")
	}
	logs, err := Log(db, 0)
	if err != nil || len(logs) < 3 {
		t.Fatalf("logs=%d err=%v", len(logs), err)
	}
}
