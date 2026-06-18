package feedback_test

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestAnnotateEmptyContext(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := feedback.Add(db, feedback.AddInput{Role: "user", Note: "solo", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := feedback.Annotate(db, id, "first note", "test"); err != nil {
		t.Fatal(err)
	}
	row, err := feedback.Show(db, id)
	if err != nil || row.Context == "" {
		t.Fatalf("context=%q err=%v", row.Context, err)
	}
}

func TestCloseEmptyProposedFix(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := feedback.Add(db, feedback.AddInput{Role: "codebase", Note: "n", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := feedback.Close(db, id, "", "test"); err != nil {
		t.Fatal(err)
	}
}

func TestListByStatusClosed(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, _ := feedback.Add(db, feedback.AddInput{Role: "user", Note: "x", ModelID: "test"})
	_, _ = feedback.Close(db, id, "", "test")
	open, err := feedback.List(db, "open", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("open=%d", len(open))
	}
	closed, err := feedback.List(db, "closed", 0)
	if err != nil || len(closed) != 1 {
		t.Fatalf("closed=%d err=%v", len(closed), err)
	}
}

func TestImportMarkdownTextSuccess(t *testing.T) {
	db, _ := testutil.TempDB(t)
	text := "## Imported\n- **Role**: model\n- **Severity**: low\n\nBody text\n"
	result, err := feedback.ImportMarkdownText(db, text, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("imported=%d", result.Imported)
	}
}
