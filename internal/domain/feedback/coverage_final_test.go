package feedback

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestFeedbackAnnotateCloseShow(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := Add(db, AddInput{Role: "user", Category: "ux", Severity: "medium", Note: "note", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	annotated, err := Annotate(db, id[:8], "more context", "test")
	if err != nil || annotated != id {
		t.Fatalf("annotate=%q err=%v", annotated, err)
	}
	row, err := Show(db, id)
	if err != nil || row.Context == "" {
		t.Fatalf("show=%+v err=%v", row, err)
	}
	closed, err := Close(db, id, "fixed it", "test")
	if err != nil || closed != id {
		t.Fatalf("close=%q err=%v", closed, err)
	}
	all, err := List(db, "closed", 0)
	if err != nil || len(all) != 1 {
		t.Fatalf("closed list=%d err=%v", len(all), err)
	}
	open, err := List(db, "open", -1)
	if err != nil || len(open) != 0 {
		t.Fatalf("open=%d", len(open))
	}
}

func TestNormalizeSeverityBranches(t *testing.T) {
	if normalizeSeverity("") != nil {
		t.Fatal("empty severity nil")
	}
	if v, ok := normalizeSeverity("high").(string); !ok || v != "high" {
		t.Fatalf("high=%v", normalizeSeverity("high"))
	}
	if v, ok := normalizeSeverity("medium").(string); !ok || v != "med" {
		t.Fatalf("medium=%v", normalizeSeverity("medium"))
	}
}

func TestNullIfEmptyFeedback(t *testing.T) {
	if nullIfEmpty("  ") != nil {
		t.Fatal("whitespace nil")
	}
	if nullIfEmpty("x") != "x" {
		t.Fatal("value kept")
	}
}
