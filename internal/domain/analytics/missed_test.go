package analytics

import (
	"fmt"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestRecordMissedCallNilDB(t *testing.T) {
	RecordMissedCall(nil, []string{"bad"}, "unknown_command", "err", "help", 2, "/tmp", "/repo", "test")
}

func TestMissedCallsAndSummary(t *testing.T) {
	db, _ := testutil.TempDB(t)
	RecordMissedCall(db, []string{"work-on"}, "unknown_command", "unknown", "devdb plan item start", 2, "/cwd", "/repo", "test")
	RecordMissedCall(db, []string{"work-on"}, "unknown_command", "unknown", "", 2, "/cwd", "/repo", "test")
	rows, err := ListMissedCalls(db, "", 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("list: %d err=%v", len(rows), err)
	}
	since := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	sum, err := MissedSummary(db, since, 7)
	if err != nil || sum.Total != 2 {
		t.Fatalf("summary: %+v err=%v", sum, err)
	}
	sum, err = MissedSummary(db, "", 0)
	if err != nil || sum.Total != 2 {
		t.Fatalf("default window: %+v", sum)
	}
}

func TestHygiene(t *testing.T) {
	db, _ := testutil.TempDB(t)
	for i := 0; i < 25; i++ {
		RecordMissedCall(db, []string{"x"}, "unknown_command", "e", "", 1, "", "", "test")
	}
	for i := 0; i < 6; i++ {
		id, err := storage.NewID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO architecture_notes
			(id, topic, body, source_paths, source_hashes, confidence, status, last_verified_at, created_at, updated_at, model_id)
			VALUES (?, ?, 'body', '[]', '{}', 'medium', 'active', datetime('now'), datetime('now'), datetime('now'), 'test')`,
			id, fmt.Sprintf("topic-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Hygiene(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Recommendations) < 2 {
		t.Fatalf("recommendations=%v", rep.Recommendations)
	}
}