package feedback_test

import (
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestListUnlimitedWhenLimitZero(t *testing.T) {
	db, _ := testutil.TempDB(t)
	for i := 0; i < 25; i++ {
		if _, err := feedback.Add(db, feedback.AddInput{Role: "user", Note: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	capped, err := feedback.List(db, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped) != 20 {
		t.Fatalf("limit 20: got %d", len(capped))
	}
	all, err := feedback.List(db, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 25 {
		t.Fatalf("limit 0: got %d want 25", len(all))
	}
}

func TestAddAndShow(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := feedback.Add(db, feedback.AddInput{
		Role: "model", Category: "ux", Severity: "medium", Note: "needs polish",
		Context: "sidebar", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	row, err := feedback.Show(db, id[:8])
	if err != nil {
		t.Fatal(err)
	}
	if row.Severity != "med" {
		t.Fatalf("severity=%q want med", row.Severity)
	}
	if row.Category != "ux" || row.Context != "sidebar" {
		t.Fatalf("row=%+v", row)
	}
}

func TestSeverityNormalization(t *testing.T) {
	db, _ := testutil.TempDB(t)
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"HIGH", "high"},
		{"medium", "med"},
		{"low", "low"},
	}
	for _, tc := range cases {
		id, err := feedback.Add(db, feedback.AddInput{
			Role: "codebase", Severity: tc.in, Note: "n", ModelID: "test",
		})
		if err != nil {
			t.Fatal(err)
		}
		row, err := feedback.Show(db, id)
		if err != nil {
			t.Fatal(err)
		}
		if row.Severity != tc.want {
			t.Fatalf("in=%q got severity=%q want %q", tc.in, row.Severity, tc.want)
		}
	}
}

func TestAnnotateAppendsContext(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := feedback.Add(db, feedback.AddInput{
		Role: "user", Note: "issue", Context: "first", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := feedback.Annotate(db, id[:8], "follow-up note", "test")
	if err != nil || resolved != id {
		t.Fatalf("annotate: id=%s err=%v", resolved, err)
	}
	row, err := feedback.Show(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(row.Context, "first") || !strings.Contains(row.Context, "follow-up note") {
		t.Fatalf("context=%q", row.Context)
	}
}

func TestCloseWithProposedFix(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := feedback.Add(db, feedback.AddInput{
		Role: "codebase", Note: "bug", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	closedID, err := feedback.Close(db, id[:8], "fixed in commit abc", "test")
	if err != nil || closedID != id {
		t.Fatalf("close: %s err=%v", closedID, err)
	}
	row, err := feedback.Show(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "closed" || row.ProposedFix != "fixed in commit abc" {
		t.Fatalf("row=%+v", row)
	}

	open, err := feedback.List(db, "open", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("expected no open feedback, got %d", len(open))
	}
}

func TestListNegativeLimitDefaults(t *testing.T) {
	db, _ := testutil.TempDB(t)
	for i := 0; i < 5; i++ {
		if _, err := feedback.Add(db, feedback.AddInput{Role: "user", Note: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := feedback.List(db, "", -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("got %d want 5 (no negative cap)", len(rows))
	}
}

func TestShowNotFound(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := feedback.Show(db, "missing"); err == nil {
		t.Fatal("expected error for missing feedback")
	}
}
