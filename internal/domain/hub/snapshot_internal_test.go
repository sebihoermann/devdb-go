package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestCollectSnapshotMissingDatabase(t *testing.T) {
	root := t.TempDir()
	snap := CollectSnapshot(root, filepath.Join(root, ".devdb", "development.db"))
	if snap.SyncStatus != "missing" || snap.AttentionScore != 100 {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestCollectSnapshotBlockedAndAttention(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcDB, srcPath := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(srcDB, planning.CreatePlanInput{Slug: "blk", Title: "Blk", ModelID: "test"})
	msID, _ := planning.AddMilestone(srcDB, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(srcDB, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Blocked work", ModelID: "test",
	})
	_, _ = planning.StartItem(srcDB, itemID, "test")
	_, _ = planning.PauseItem(srcDB, itemID, "blocked on upstream API", "test")
	_, _ = feedback.Add(srcDB, feedback.AddInput{Role: "model", Severity: "high", Note: "urgent", ModelID: "test"})
	exit := 0
	runID, err := verification.RecordRun(srcDB, "go test ./...", ".", "abc", "passed", &exit, "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := verification.FinishRun(srcDB, runID, "passed", &exit, ""); err != nil {
		t.Fatal(err)
	}
	_ = srcDB.Close()
	data, _ := os.ReadFile(srcPath)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	snap := CollectSnapshot(repo, dbPath)
	if snap.OpenHighFeedback < 1 {
		t.Fatalf("quality counts: %+v", snap)
	}
	if len(snap.AttentionItems) == 0 || snap.AttentionScore < 15 {
		t.Fatalf("attention: score=%d items=%d", snap.AttentionScore, len(snap.AttentionItems))
	}
}

func TestBlockedReasonPatterns(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "br", Title: "BR", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "W", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")
	cases := []struct {
		note string
		want bool
	}{
		{"waiting for review", true},
		{"stuck on tests", true},
		{"", false},
		{"progressing fine", false},
	}
	for _, tc := range cases {
		_, _ = db.Exec(`DELETE FROM status_log`)
		if tc.note != "" {
			_, _ = db.Exec(`INSERT INTO status_log(id, plan_item_id, status, note, created_at, model_id)
				VALUES ('s1', ?, 'in_progress', ?, datetime('now'), 'test')`, itemID, tc.note)
		}
		got := blockedReason(db)
		if (got != "") != tc.want {
			t.Fatalf("note=%q got=%q want blocked=%v", tc.note, got, tc.want)
		}
	}
}

func TestAttentionScorePreservesErrorScore(t *testing.T) {
	s := Snapshot{SyncStatus: "error", AttentionScore: 90}
	if attentionScore(s) != 90 {
		t.Fatalf("score=%d", attentionScore(s))
	}
}

func TestBuildAttentionReturnsPresetOnError(t *testing.T) {
	items := []AttentionItem{{Kind: "missing_db"}}
	s := Snapshot{Error: "boom", AttentionItems: items}
	if got := buildAttention(s); len(got) != 1 || got[0].Kind != "missing_db" {
		t.Fatalf("got=%v", got)
	}
}

func TestLatestVerificationFailedStatus(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	runID, err := verification.RecordRun(db, "go test", ".", "", "failed", &exit, "fail", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := verification.FinishRun(db, runID, "failed", &exit, "fail"); err != nil {
		t.Fatal(err)
	}
	st, fresh, cmd, scope, at := latestVerification(db)
	if st != "failed" || fresh != "failed" || cmd == "" || scope == "" || at == "" {
		t.Fatalf("st=%s fresh=%s cmd=%s scope=%s at=%s", st, fresh, cmd, scope, at)
	}
}

func TestExpandHome(t *testing.T) {
	home := expandHome("~/devdb-test-path")
	if !strings.HasPrefix(home, "/") || strings.Contains(home, "~") {
		t.Fatalf("home=%q", home)
	}
	if got := defaultAlias("my-repo"); got != "my-repo" {
		t.Fatalf("alias=%q", got)
	}
}

func TestReadRegistryMissingFile(t *testing.T) {
	rows, err := ReadRegistry(filepath.Join(t.TempDir(), "missing-registry"))
	if err != nil || rows != nil {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}
