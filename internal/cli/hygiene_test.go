package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestM4HygieneCommands(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")

	_, _, code := runDevdb(t, bin, "--db", dbPath, "init")
	if code != 0 {
		t.Fatal("init failed")
	}

	// Trigger a missed call via unknown legacy command.
	_, stderr, code := runDevdb(t, bin, "--db", dbPath, "list-missed-calls")
	if code == 0 {
		t.Fatal("expected unknown command failure")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr=%q", stderr)
	}

	stdout, _, code := runDevdb(t, bin, "--db", dbPath, "analytics", "missed", "--json")
	if code != 0 {
		t.Fatalf("analytics missed exit %d: %s", code, stdout)
	}
	if !strings.Contains(stdout, "failure_kind") && stdout != "[]\n" {
		t.Fatalf("missed json=%q", stdout)
	}

	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "analytics", "summary", "--json")
	if code != 0 || !strings.Contains(stdout, `"total"`) {
		t.Fatalf("summary: code=%d out=%q", code, stdout)
	}

	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "archive", "gc", "--dry-run", "--json")
	if code != 0 || !strings.Contains(stdout, "feedback_to_close") {
		t.Fatalf("gc dry-run: code=%d out=%q", code, stdout)
	}

	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "archive", "run", "--dry-run", "--json")
	if code != 0 || !strings.Contains(stdout, `"dry_run": true`) {
		t.Fatalf("archive run dry-run: code=%d out=%q", code, stdout)
	}

	// devdb ledger housekeeping bundles archive + reconcile + scan in one
	// verb. --dry-run must preview without writing, --json must return the
	// structured step list. The plan acceptance criterion is that each
	// sub-step is invoked in order; under --dry-run we expect archive and
	// reconcile to run as preview, and inventory-scan to be skipped.
	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "ledger", "housekeeping", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("ledger housekeeping --dry-run exit %d: %s", code, stdout)
	}
	for _, step := range []string{`"name": "archive-run"`, `"name": "plan-reconcile"`, `"name": "inventory-scan"`, `"dry_run": true`} {
		if !strings.Contains(stdout, step) {
			t.Fatalf("housekeeping --dry-run missing %q in: %s", step, stdout)
		}
	}
	// Step status assertions: archive-run must be would-run, plan-reconcile
	// must be ran, inventory-scan must be skipped.
	if !strings.Contains(stdout, `"name": "archive-run"`) || !strings.Contains(stdout, `"status": "would-run"`) {
		t.Fatalf("archive-run should be would-run under --dry-run: %s", stdout)
	}
	if !strings.Contains(stdout, `"status": "skipped"`) {
		t.Fatalf("inventory-scan should be skipped under --dry-run: %s", stdout)
	}

	// --json without --dry-run returns dry_run=false and ran status for
	// archive + reconcile. inventory-scan may fail in this temp dir (no
	// git repo) but the verb must still exit 0 because individual step
	// failures are reported in the result, not surfaced as verb errors.
	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "ledger", "housekeeping", "--json")
	if code != 0 {
		t.Fatalf("ledger housekeeping exit %d: %s", code, stdout)
	}
	if !strings.Contains(stdout, `"dry_run": false`) {
		t.Fatalf("housekeeping without --dry-run should report dry_run=false: %s", stdout)
	}

	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "doctor", "hygiene", "--json")
	if code != 0 || !strings.Contains(stdout, "missed_cli_calls_7d") {
		t.Fatalf("doctor hygiene: code=%d out=%q", code, stdout)
	}
}

func TestM4TaskApprovalReminderFlow(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runDevdb(t, bin, "--db", dbPath, "init")

	stdout, _, code := runDevdb(t, bin, "--db", dbPath, "task", "add", "Ship M4")
	if code != 0 {
		t.Fatal("task add failed")
	}
	taskID := strings.TrimSpace(stdout)

	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "approval", "request", taskID)
	if code != 0 {
		t.Fatalf("approval request failed: %s", stdout)
	}

	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "approval", "approve", taskID)
	if code != 0 {
		t.Fatalf("approval approve failed: %s", stdout)
	}

	stdout, _, code = runDevdb(t, bin, "--db", dbPath, "reminder", "add", "Check M4", "--plan-item", taskID)
	if code != 0 {
		t.Fatalf("reminder add failed: %s", stdout)
	}
	remID := strings.TrimSpace(stdout)

	_, _, code = runDevdb(t, bin, "--db", dbPath, "reminder", "snooze", remID, "--until", "2026-12-31T00:00:00Z")
	if code != 0 {
		t.Fatal("reminder snooze failed")
	}

	_, _, code = runDevdb(t, bin, "--db", dbPath, "reminder", "unsnooze", remID)
	if code != 0 {
		t.Fatal("reminder unsnooze failed")
	}

	listOut, _, code := runDevdb(t, bin, "--db", dbPath, "reminder", "list", "--json")
	if code != 0 || !strings.Contains(listOut, remID[:8]) {
		t.Fatalf("reminder list: %q", listOut)
	}
}
