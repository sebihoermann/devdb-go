package archive

import (
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestCoverageBoostArchivePlanItemChildren(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "arch-child", Title: "A", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Done", ModelID: "test",
	})
	_, _ = planning.AddAcceptance(db, itemID, "criterion", "test", 1)
	_, _ = planning.AddPlanFile(db, itemID, "main.go", "modify", "test")
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	_, _ = db.Exec(`UPDATE plan_items SET status='done', created_at=? WHERE id=?`, old, itemID)

	now := storage.NowUTC()
	n, err := archiveTable(db, archiveStep{
		table: "plan_items", where: "status='done' AND created_at < ?",
		params: []any{now}, reason: "retention",
	}, now, 3)
	if err != nil || n != 1 {
		t.Fatalf("archive plan_items=%d err=%v", n, err)
	}
	var childCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_item_acceptance WHERE plan_item_id=?`, itemID).Scan(&childCount); err != nil {
		t.Fatal(err)
	}
	if childCount != 0 {
		t.Fatalf("children should be archived: %d", childCount)
	}
}

func TestReadTableRowsAndArchiveRowPayload(t *testing.T) {
	db, _ := testutil.TempDB(t)
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO goals(id, kind, title, status, created_at, model_id) VALUES ('g-read','goal','G','active',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	rows, err := readTableRows(db, `SELECT * FROM goals WHERE id='g-read'`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	now := storage.NowUTC()
	if err := archiveRow(db, "goals", "g-read", rows[0].payload, now, "direct"); err != nil {
		t.Fatal(err)
	}
	var archived int
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_entries WHERE source_id='g-read'`).Scan(&archived); err != nil || archived != 1 {
		t.Fatalf("archived=%d err=%v", archived, err)
	}
}

func TestGCClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	_, err := GC(db, GCOptions{DryRun: true})
	if err == nil {
		t.Fatal("expected gc error on closed db")
	}
}


func TestCountLocSnapshotsBoost(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`CREATE TABLE loc_snapshots (
		id TEXT PRIMARY KEY, snapshot_at TEXT, file_path TEXT, lines INTEGER,
		created_at TEXT, model_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	for i, snap := range []string{"2020-01-01T00:00:00Z", "2020-02-01T00:00:00Z"} {
		_, err := db.Exec(
			`INSERT INTO loc_snapshots(id, snapshot_at, file_path, lines, created_at, model_id) VALUES (?, ?, ?, ?, ?, ?)`,
			"ls"+string(rune('a'+i)), snap, "main.go", 10+i, snap, "test",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	n, err := countLocSnapshots(db, 1)
	if err != nil || n != 1 {
		t.Fatalf("count loc=%d err=%v", n, err)
	}
}
