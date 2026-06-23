package openclaw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestSyncIsIdempotentAcrossIndependentWorkspaceAndLedger(t *testing.T) {
	db, _ := testutil.TempDB(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("# Memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "2026-06-22.md"), []byte("# Today\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := Sync(db, workspace, "test")
	if err != nil {
		t.Fatal(err)
	}
	if first.Files != 2 || first.NotesCreated != 2 || first.NotesUpdated != 0 {
		t.Fatalf("first=%+v", first)
	}
	second, err := Sync(db, workspace, "test")
	if err != nil {
		t.Fatal(err)
	}
	if second.NotesCreated != 0 || second.NotesUpdated != 2 {
		t.Fatalf("second=%+v", second)
	}
	notes, err := architecture.List(db, architecture.ListFilter{})
	if err != nil || len(notes) != 2 {
		t.Fatalf("notes=%d err=%v", len(notes), err)
	}
}

func TestExternalHashChangeMakesMemoryNoteStale(t *testing.T) {
	db, _ := testutil.TempDB(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("# Memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Sync(db, workspace, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.SyncExternalSources(db, result.Namespace, []inventory.ExternalSource{{Path: "MEMORY.md", ContentHash: "changed"}}, "test"); err != nil {
		t.Fatal(err)
	}
	notes, err := architecture.List(db, architecture.ListFilter{Stale: true})
	if err != nil || len(notes) != 1 || !notes[0].Stale {
		t.Fatalf("notes=%+v err=%v", notes, err)
	}
}

func TestSyncPreservesEditedNoteBody(t *testing.T) {
	db, _ := testutil.TempDB(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("# Memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(db, workspace, "test"); err != nil {
		t.Fatal(err)
	}
	notes, err := architecture.List(db, architecture.ListFilter{})
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes=%d err=%v", len(notes), err)
	}
	customBody := "manually curated body"
	if _, found, err := architecture.Update(db, notes[0].ID, &customBody, notes[0].SourcePaths, nil); err != nil || !found {
		t.Fatalf("update found=%v err=%v", found, err)
	}
	if _, err := Sync(db, workspace, "test"); err != nil {
		t.Fatal(err)
	}
	note, err := architecture.Get(db, notes[0].ID)
	if err != nil || note == nil {
		t.Fatalf("note=%v err=%v", note, err)
	}
	if note.Body != customBody {
		t.Fatalf("body=%q", note.Body)
	}
}

func TestMemoryRef(t *testing.T) {
	if got := MemoryRef("memory/2026-06-22.md", "decision"); got != "openclaw:memory/2026-06-22.md#decision" {
		t.Fatalf("ref=%q", got)
	}
}
