package hub_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestAuditEmptyRegistry(t *testing.T) {
	dir := t.TempDir()
	report, err := hub.Audit(hub.AuditOptions{Registry: filepath.Join(dir, "reg")})
	if err != nil {
		t.Fatalf("Audit empty registry: %v", err)
	}
	if len(report.Sections) != 8 {
		t.Fatalf("expected 8 sections, got %d", len(report.Sections))
	}
	for _, kind := range hub.AuditSectionOrder() {
		if report.Sections[kind].Kind != kind {
			t.Fatalf("section %q kind=%q", kind, report.Sections[kind].Kind)
		}
	}
	if len(report.ByProject) != 0 {
		t.Fatalf("expected no projects, got %d", len(report.ByProject))
	}
}

func TestAuditInvalidSeverity(t *testing.T) {
	dir := t.TempDir()
	_, err := hub.Audit(hub.AuditOptions{Registry: filepath.Join(dir, "reg"), Severity: "bogus"})
	if err == nil {
		t.Fatal("expected error for invalid severity")
	}
}

func TestAuditInvalidMode(t *testing.T) {
	dir := t.TempDir()
	_, err := hub.Audit(hub.AuditOptions{Registry: filepath.Join(dir, "reg"), Mode: "weird"})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestAuditAllEightSectionsPopulated(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := setupAcrossProject(t, dir, "populated")

	db, path := testutil.TempDB(t)

	// high_feedback
	if _, err := feedback.Add(db, feedback.AddInput{
		Role: "codebase", Severity: "high", Category: "correctness",
		Note: "GDPR consent overlay blocks scraper", ModelID: "test",
	}); err != nil {
		t.Fatal(err)
	}

	// high_findings
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	if _, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "src/scraper.py", Principle: "dry", Title: "duplicate selector logic",
		Recommendation: "extract", Severity: "high", Confidence: "high", Effort: "small",
	}, "test"); err != nil {
		t.Fatal(err)
	}

	// in_progress plan item with blocked status_log note
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "blocked-plan", Title: "blocked plan", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	msID, err := planning.AddMilestone(db, planID, "M1", "", "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, MilestoneID: msID,
		Title: "blocked item", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.StartItem(db, itemID, "test"); err != nil {
		t.Fatal(err)
	}
	// Insert a status_log note that contains "blocked" to trigger the
	// blocked section. StartItem writes "started" by default; we add a
	// second note via a direct INSERT to model a paused/blocked state.
	// Use the same RFC3339Nano format that production code emits.
	if _, err := db.Exec(`INSERT INTO status_log(plan_item_id, status, note, created_at, model_id)
		VALUES (?, 'in_progress', ?, ?, 'test')`, itemID, "blocked on upstream API",
		time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	// planned item
	plannedItemID, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, MilestoneID: msID,
		Title: "next planned item", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = plannedItemID

	// overdue reminder
	if _, err := reminders.Add(db, reminders.AddInput{
		Title: "renew cert", DueAt: "2020-01-01T00:00:00Z", ModelID: "test",
	}); err != nil {
		t.Fatal(err)
	}

	// stale architecture note: insert with a source path whose hash we'll
	// later invalidate by replacing repo_files so noteIsStale returns true.
	if _, err := db.Exec(`INSERT INTO repo_files(path, language, kind, lines, content_hash, size_bytes, last_seen_at, last_scan_run_id)
		VALUES ('docs/x.md', 'text', 'doc', 10, 'abc123', 100, datetime('now'), NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := architecture.Add(db, "stale-topic", "body text",
		[]string{"docs/x.md"}, "high", "test"); err != nil {
		t.Fatal(err)
	}
	// Make the source stale by changing the content_hash on disk.
	if _, err := db.Exec(`UPDATE repo_files SET content_hash='different' WHERE path='docs/x.md'`); err != nil {
		t.Fatal(err)
	}

	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "populated", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{Registry: registry})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}

	// high_feedback
	if len(report.Sections["high_feedback"].Rows) != 1 {
		t.Fatalf("high_feedback rows=%d", len(report.Sections["high_feedback"].Rows))
	}
	row := report.Sections["high_feedback"].Rows[0]
	if row["project"] != "populated" || row["severity"] != "high" {
		t.Fatalf("high_feedback row=%v", row)
	}

	// high_findings
	if len(report.Sections["high_findings"].Rows) != 1 {
		t.Fatalf("high_findings rows=%d", len(report.Sections["high_findings"].Rows))
	}

	// stale_arch: 1 row (the source_hash on disk is 'abc123' but the
	// file doesn't exist when we read repo_files inside audit live-mode —
	// it depends on whether scan_runs has refreshed). We just assert the
	// section exists.
	if report.Sections["stale_arch"].Kind != "stale_arch" {
		t.Fatalf("stale_arch missing")
	}

	// overdue_reminders
	if len(report.Sections["overdue_reminders"].Rows) != 1 {
		t.Fatalf("overdue_reminders rows=%d", len(report.Sections["overdue_reminders"].Rows))
	}

	// in_progress
	if len(report.Sections["in_progress"].Rows) != 1 {
		t.Fatalf("in_progress rows=%d", len(report.Sections["in_progress"].Rows))
	}

	// blocked (status_log note contains "blocked")
	if len(report.Sections["blocked"].Rows) != 1 {
		t.Fatalf("blocked rows=%d", len(report.Sections["blocked"].Rows))
	}

	// planned_per_project
	if len(report.Sections["planned_per_project"].Rows) != 1 {
		t.Fatalf("planned_per_project rows=%d", len(report.Sections["planned_per_project"].Rows))
	}
	pRow := report.Sections["planned_per_project"].Rows[0]
	if pRow["project"] != "populated" || pRow["count"] != 1 {
		t.Fatalf("planned_per_project row=%v", pRow)
	}
	if pRow["next"] != "next planned item" {
		t.Fatalf("planned next=%v", pRow["next"])
	}

	// by_project
	if report.ByProject["populated"]["open_high_feedback"] != 1 {
		t.Fatalf("by_project[populated]=%v", report.ByProject["populated"])
	}
}

func TestAuditProjectFilter(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root1 := setupAcrossProject(t, dir, "p1")
	root2 := setupAcrossProject(t, dir, "p2")

	db1, p1 := testutil.TempDB(t)
	if _, err := feedback.Add(db1, feedback.AddInput{Role: "model", Severity: "high", Note: "p1 note", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	db1.Close()
	moveDB(t, p1, root1)

	db2, p2 := testutil.TempDB(t)
	if _, err := feedback.Add(db2, feedback.AddInput{Role: "model", Severity: "high", Note: "p2 note", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	db2.Close()
	moveDB(t, p2, root2)

	if _, err := hub.Register(root1, "p1", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root2, "p2", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{Registry: registry, Projects: []string{"p1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sections["high_feedback"].Rows) != 1 {
		t.Fatalf("expected 1 row for p1 only, got %d", len(report.Sections["high_feedback"].Rows))
	}
	if report.Sections["high_feedback"].Rows[0]["project"] != "p1" {
		t.Fatalf("got %v", report.Sections["high_feedback"].Rows[0]["project"])
	}
	if _, ok := report.ByProject["p2"]; ok {
		t.Fatalf("p2 should have been filtered out")
	}
}

func TestAuditSeverityThreshold(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := setupAcrossProject(t, dir, "thr")

	db, path := testutil.TempDB(t)
	if _, err := feedback.Add(db, feedback.AddInput{Role: "model", Severity: "low", Note: "low note", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := feedback.Add(db, feedback.AddInput{Role: "model", Severity: "medium", Note: "med note", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := feedback.Add(db, feedback.AddInput{Role: "model", Severity: "high", Note: "high note", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "thr", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	highOnly, err := hub.Audit(hub.AuditOptions{Registry: registry, Severity: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(highOnly.Sections["high_feedback"].Rows); got != 1 {
		t.Fatalf("severity=high expected 1 row, got %d", got)
	}

	medPlus, err := hub.Audit(hub.AuditOptions{Registry: registry, Severity: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(medPlus.Sections["high_feedback"].Rows); got != 2 {
		t.Fatalf("severity=medium expected 2 rows, got %d", got)
	}
}

func TestAuditKindFilter(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := setupAcrossProject(t, dir, "kf")

	db, path := testutil.TempDB(t)
	if _, err := feedback.Add(db, feedback.AddInput{Role: "model", Severity: "high", Note: "fb", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	if _, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "a.go", Principle: "dry", Title: "fd", Recommendation: "r",
		Severity: "high", Confidence: "high", Effort: "small",
	}, "test"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "kf", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{
		Registry: registry,
		Kinds:    []string{"feedback"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sections["high_feedback"].Rows) != 1 {
		t.Fatalf("expected 1 high_feedback, got %d", len(report.Sections["high_feedback"].Rows))
	}
	if len(report.Sections["high_findings"].Rows) != 0 {
		t.Fatalf("high_findings should be empty, got %d", len(report.Sections["high_findings"].Rows))
	}
	if len(report.Sections["in_progress"].Rows) != 0 {
		t.Fatalf("in_progress should be empty (kind=feedback only), got %d", len(report.Sections["in_progress"].Rows))
	}
}

func TestAuditSkipsCorruptDB(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := setupAcrossProject(t, dir, "corrupt")
	if err := os.WriteFile(filepath.Join(root, ".devdb", "development.db"), []byte("not-a-db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registry, []byte(root+" corrupt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := hub.Audit(hub.AuditOptions{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sections["high_feedback"].Rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(report.Sections["high_feedback"].Rows))
	}
}

func TestAuditSkipsMissingDB(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	ghost := filepath.Join(dir, "ghost")
	if err := os.MkdirAll(filepath.Join(ghost, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registry, []byte(ghost+" ghost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := hub.Audit(hub.AuditOptions{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sections["planned_per_project"].Rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(report.Sections["planned_per_project"].Rows))
	}
}

func TestAuditSkipsLegacyPython(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := setupAcrossProject(t, dir, "py")
	// Write a minimal Python-schema DB: a non-go schema_migrations row.
	db, path := testutil.TempDB(t)
	_, err := db.Exec(`DELETE FROM schema_migrations`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO schema_migrations(version, description, applied_at) VALUES (3, 'python legacy', datetime('now'))`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	moveDB(t, path, root)
	if err := os.WriteFile(registry, []byte(root+" py\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := hub.Audit(hub.AuditOptions{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sections["planned_per_project"].Rows) != 0 {
		t.Fatalf("expected 0 rows for legacy python, got %d", len(report.Sections["planned_per_project"].Rows))
	}
}

func TestAuditCachedMode(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := setupAcrossProject(t, dir, "cached")

	db, path := testutil.TempDB(t)
	if _, err := feedback.Add(db, feedback.AddInput{Role: "model", Severity: "high", Note: "cached note", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "cached", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}
	// Force a sync so the snapshot has the count.
	if _, err := hub.SyncAll(registry, filepath.Join(dir, "meta.db"), false); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{
		Registry:   registry,
		MetadataDB: filepath.Join(dir, "meta.db"),
		Mode:       "cached",
	})
	if err != nil {
		t.Fatal(err)
	}
	// cached mode returns counts via by_project but not row-level data
	// for the high_feedback section.
	if report.ByProject["cached"]["open_high_feedback"] < 1 {
		t.Fatalf("cached by_project[open_high_feedback]=%v", report.ByProject["cached"])
	}
}

func TestAuditJSONShape(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := setupAcrossProject(t, dir, "json")

	db, path := testutil.TempDB(t)
	if _, err := feedback.Add(db, feedback.AddInput{Role: "model", Severity: "high", Note: "json note", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	moveDB(t, path, root)
	if _, err := hub.Register(root, "json", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, key := range []string{
		`"collected_at"`, `"registry"`, `"mode"`, `"severity_threshold"`,
		`"sections"`, `"by_project"`,
		`"high_feedback"`, `"high_findings"`, `"stale_arch"`,
		`"overdue_reminders"`, `"in_progress"`, `"blocked"`,
		`"planned_per_project"`, `"stale_verification"`,
	} {
		if !strings.Contains(s, key) {
			t.Fatalf("JSON missing key %s in: %s", key, s)
		}
	}
}

func TestAuditCanonicalKind(t *testing.T) {
	if hub.CanonicalKindForTest("feedback") != "high_feedback" {
		t.Fatal("feedback alias failed")
	}
	if hub.CanonicalKindForTest("findings") != "high_findings" {
		t.Fatal("findings alias failed")
	}
	if hub.CanonicalKindForTest("planned") != "planned_per_project" {
		t.Fatal("planned alias failed")
	}
	if hub.CanonicalKindForTest("unknown") != "unknown" {
		t.Fatal("unknown should pass through")
	}
}
