package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/archive"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// fbStatus reads a feedback row's status directly from the ledger.
func fbStatus(t *testing.T, dbPath, id string) string {
	t.Helper()
	con, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	var status string
	if err := con.QueryRow(`SELECT status FROM feedback WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// ageFeedback back-dates a feedback row's created_at so gc sees it as stale.
func ageFeedback(t *testing.T, dbPath, id string, days int) {
	t.Helper()
	con, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer con.Close()
	if _, err := con.Exec(
		`UPDATE feedback SET created_at = datetime('now', ?) WHERE id = ?`,
		"-"+itoa(days)+" days", id); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestArchiveGCSeverityFloor covers feedback 20b34ea5: gc must not close
// high/critical rows by default (they are live debt, not silt), must
// respect --keep-severity overrides, and --keep-severity "" restores the
// legacy close-everything behavior.
func TestArchiveGCSeverityFloor(t *testing.T) {
	e := newInProcessEnv(t)

	hi := e.run("feedback", "add", "high severity stale row", "--role", "model", "--severity", "high")
	crit := e.run("feedback", "add", "critical stale row", "--role", "model", "--severity", "critical")
	lo := e.run("feedback", "add", "low severity stale row", "--role", "model", "--severity", "low")
	ageFeedback(t, e.dbPath, hi, 60)
	ageFeedback(t, e.dbPath, crit, 60)
	ageFeedback(t, e.dbPath, lo, 60)

	type gcJSON struct {
		FeedbackToClose   int      `json:"feedback_to_close"`
		FeedbackProtected int      `json:"feedback_protected"`
		FeedbackIDs       []string `json:"feedback_ids"`
		FeedbackClosed    int      `json:"feedback_closed"`
	}

	// Default: high+critical protected, only the low row closes.
	var d gcJSON
	out := e.runOut("--json", "archive", "gc", "--dry-run")
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("gc dry-run json: %v (%s)", err, out)
	}
	if d.FeedbackToClose != 1 || d.FeedbackProtected != 2 {
		t.Fatalf("default floor: want close=1 protected=2, got %+v", d)
	}
	if len(d.FeedbackIDs) != 1 || d.FeedbackIDs[0] != lo {
		t.Fatalf("dry-run should list the low row id, got %v", d.FeedbackIDs)
	}
	// Dry-run must not write.
	e.runOut("archive", "gc", "--dry-run")
	if s := fbStatus(t, e.dbPath, hi); s != "open" {
		t.Fatalf("dry-run closed a protected row: %s", s)
	}

	// Apply: low closes, high+critical survive.
	var a gcJSON
	if err := json.Unmarshal([]byte(e.runOut("--json", "archive", "gc")), &a); err != nil {
		t.Fatal(err)
	}
	if a.FeedbackClosed != 1 || a.FeedbackProtected != 2 {
		t.Fatalf("apply: want closed=1 protected=2, got %+v", a)
	}
	if s := fbStatus(t, e.dbPath, lo); s != "closed" {
		t.Fatalf("low row should be closed, got %s", s)
	}
	if s := fbStatus(t, e.dbPath, hi); s != "open" {
		t.Fatalf("high row must survive default floor, got %s", s)
	}

	// Explicit narrower floor: protect only critical -> high now closes.
	var n gcJSON
	if err := json.Unmarshal([]byte(e.runOut("--json", "archive", "gc", "--keep-severity", "critical")), &n); err != nil {
		t.Fatal(err)
	}
	if n.FeedbackClosed != 1 {
		t.Fatalf("keep-severity=critical: want closed=1 (the high row), got %+v", n)
	}
	if s := fbStatus(t, e.dbPath, hi); s != "closed" {
		t.Fatalf("high row should close under critical-only floor, got %s", s)
	}
	if s := fbStatus(t, e.dbPath, crit); s != "open" {
		t.Fatalf("critical row must survive, got %s", s)
	}

	// Empty floor = legacy behavior: everything stale closes.
	var l gcJSON
	if err := json.Unmarshal([]byte(e.runOut("--json", "archive", "gc", "--keep-severity", "")), &l); err != nil {
		t.Fatal(err)
	}
	if l.FeedbackClosed != 1 || l.FeedbackProtected != 0 {
		t.Fatalf("empty floor: want closed=1 protected=0, got %+v", l)
	}
	if s := fbStatus(t, e.dbPath, crit); s != "closed" {
		t.Fatalf("critical row should close under empty floor, got %s", s)
	}

	// Invalid severity token is rejected.
	if _, stderr, code := runCLI(t, append(e.base, "archive", "gc", "--dry-run", "--keep-severity", "bogus")...); code == 0 {
		t.Fatal("invalid keep-severity should exit non-zero")
	} else if !strings.Contains(stderr, "keep-severity") && !strings.Contains(stderr, "bogus") {
		t.Fatalf("error should name the bad value: %s", stderr)
	}
}

// TestGCServiceKeepNilDefaultsToHighCritical pins the library-level default:
// a nil KeepSeverities protects high+critical; an empty non-nil slice
// protects nothing. Guards callers that bypass the CLI.
func TestGCServiceKeepNilDefaultsToHighCritical(t *testing.T) {
	e := newInProcessEnv(t)
	hi := e.run("feedback", "add", "svc high row", "--role", "model", "--severity", "high")
	ageFeedback(t, e.dbPath, hi, 45)

	db, err := storage.Open(e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := archive.GC(db, archive.GCOptions{OlderThanDays: 30, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.FeedbackProtected != 1 || res.FeedbackToClose != 0 {
		t.Fatalf("nil KeepSeverities should protect high: %+v", res)
	}
	res2, err := archive.GC(db, archive.GCOptions{OlderThanDays: 30, DryRun: true, KeepSeverities: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if res2.FeedbackToClose != 1 || res2.FeedbackProtected != 0 {
		t.Fatalf("empty KeepSeverities should protect nothing: %+v", res2)
	}
	db.Close()
}
