package archive_test

import (
	"errors"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/archive"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// seedArchivedFeedback puts one closed row through a real sweep so tests read
// the same archive_entries shape production writes.
func seedArchivedFeedback(t *testing.T) (sourceID, entryID string) {
	t.Helper()
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`
		INSERT INTO feedback(id, role, severity, category, note, context, status, created_at)
		VALUES ('fb-archived-1', 'user', 'low', 'dx', 'archived note', '', 'closed', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Run(db, archive.RunOptions{Table: "feedback", Yes: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM archive_entries WHERE source_id='fb-archived-1'`).Scan(&id); err != nil {
		t.Fatalf("row was not archived: %v", err)
	}
	return "fb-archived-1", id
}

// TestShowReturnsPayloadByEntryID is acceptance 1: an archived row is readable
// by its archive id, labelled with its source.
func TestShowReturnsPayloadByEntryID(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`
		INSERT INTO feedback(id, role, severity, category, note, context, status, created_at)
		VALUES ('fb-archived-1', 'user', 'low', 'dx', 'archived note', '', 'closed', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Run(db, archive.RunOptions{Table: "feedback", Yes: true}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM archive_entries WHERE source_id='fb-archived-1'`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	res, err := archive.Show(db, id, "", "")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if res.Source != "archive" {
		t.Fatalf("Source = %q, want archive", res.Source)
	}
	if res.SourceTable != "feedback" {
		t.Fatalf("SourceTable = %q", res.SourceTable)
	}
	if res.SourceID != "fb-archived-1" {
		t.Fatalf("SourceID = %q", res.SourceID)
	}
	if got := res.Row["note"]; got != "archived note" {
		t.Fatalf("payload note = %v, want the original row", got)
	}
	if res.ArchivedAt == "" {
		t.Fatal("ArchivedAt empty; the caller cannot tell when it left")
	}
}

// TestShowRejectsMissingSelectors keeps the error honest instead of scanning
// the whole archive.
func TestShowRejectsMissingSelectors(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := archive.Show(db, "", "", ""); err == nil {
		t.Fatal("Show with no selector returned no error")
	}
	if _, err := archive.Show(db, "definitely-not-an-entry", "", ""); !errors.Is(err, archive.ErrEntryNotFound) {
		t.Fatalf("unknown id error = %v, want ErrEntryNotFound", err)
	}
}

// TestFindArchivedBySourceIDResolvesPrefix is acceptance 2's mechanism: a short
// id from a log line resolves to the archived row.
func TestFindArchivedBySourceIDResolvesPrefix(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`
		INSERT INTO feedback(id, role, severity, category, note, context, status, created_at)
		VALUES ('fb-prefix-target', 'user', 'low', 'dx', 'note', '', 'closed', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Run(db, archive.RunOptions{Table: "feedback", Yes: true}); err != nil {
		t.Fatal(err)
	}

	ids, err := archive.ArchivedIDsByPrefix(db, "feedback", "fb-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "fb-prefix-target" {
		t.Fatalf("prefix lookup = %v", ids)
	}

	res, ok, err := archive.FindArchivedBySourceID(db, "feedback", "fb-prefix-target")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("archived row reported as absent")
	}
	if res.Source != "archive" {
		t.Fatalf("Source = %q", res.Source)
	}

	// A live-but-unknown id must not be invented as archived.
	if _, ok, err := archive.FindArchivedBySourceID(db, "feedback", "fb-never-existed"); err != nil || ok {
		t.Fatalf("unknown id reported archived: ok=%v err=%v", ok, err)
	}
}

// TestFindArchivedBySourceIDIgnoresOtherTables stops a feedback id from
// resolving as a plan item just because the prefix matches.
func TestFindArchivedBySourceIDIgnoresOtherTables(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`
		INSERT INTO feedback(id, role, severity, category, note, context, status, created_at)
		VALUES ('shared-prefix-1', 'user', 'low', 'dx', 'note', '', 'closed', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Run(db, archive.RunOptions{Table: "feedback", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := archive.FindArchivedBySourceID(db, "plan_items", "shared-prefix-1"); err != nil || ok {
		t.Fatalf("feedback row resolved as a plan item: ok=%v err=%v", ok, err)
	}
}
