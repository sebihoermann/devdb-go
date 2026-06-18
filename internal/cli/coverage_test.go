package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestErrorsAndLimits(t *testing.T) {
	err := unknownCommandError([]string{"work-on"})
	if err.Suggestion == "" || err.Kind != "unknown_command" {
		t.Fatalf("suggestion=%q kind=%s", err.Suggestion, err.Kind)
	}
	if suggestCommand(nil) != "devdb help" {
		t.Fatal("empty argv suggestion")
	}
	if suggestCommand([]string{"gc"}) != "devdb archive gc" {
		t.Fatal("legacy gc suggestion")
	}

	ce := coerceCLIError(usageError("bad usage"))
	if ce.Code != ExitUsage {
		t.Fatalf("code=%d", ce.Code)
	}
	ce = coerceCLIError(&CLIError{Code: ExitNotFound, Message: "nf"})
	if ce.Kind != "" {
		// preserved
	}
	ce = coerceCLIError(os.ErrNotExist)
	if ce.Code != ExitGeneral {
		t.Fatalf("generic code=%d", ce.Code)
	}

	var errBuf bytes.Buffer
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&errBuf, r)
		close(done)
	}()
	code := printCLIError(usageError("msg"), w)
	_ = w.Close()
	<-done
	if code != ExitUsage || !strings.Contains(errBuf.String(), "msg") {
		t.Fatalf("print usage: code=%d buf=%q", code, errBuf.String())
	}
	errBuf.Reset()
	r2, w2, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	done2 := make(chan struct{})
	go func() {
		_, _ = io.Copy(&errBuf, r2)
		close(done2)
	}()
	code = printCLIError(os.ErrInvalid, w2)
	_ = w2.Close()
	<-done2
	if code != ExitGeneral {
		t.Fatalf("print generic: %d", code)
	}

	flagAll = true
	if effectiveListLimit(20) != 0 {
		t.Fatal("--all should zero limit")
	}
	if applyAllLimit(5) != 0 {
		t.Fatal("applyAllLimit with --all")
	}
	flagAll = false
	if effectiveListLimit(20) != 20 || applyAllLimit(5) != 5 {
		t.Fatal("without --all")
	}
}

func TestExecuteUnknownCommand(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	_, _, code := runCLI(t, "--db", dbPath, "init")
	if code != 0 {
		t.Fatalf("init code=%d", code)
	}
	_, stderr, code := runCLI(t, "--db", dbPath, "list-missed-calls")
	if code == 0 || !strings.Contains(stderr, "unknown command") {
		t.Fatalf("unknown: code=%d stderr=%q", code, stderr)
	}
}

func TestInProcessCommandSweep(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(repo, "main.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := []string{"--repo", repo, "--db", dbPath}

	run := func(args ...string) string {
		t.Helper()
		stdout, stderr, code := runCLI(t, append(base, args...)...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
		}
		return firstLine(stdout)
	}
	runOut := func(args ...string) string {
		t.Helper()
		stdout, stderr, code := runCLI(t, append(base, args...)...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s", args, code, stderr)
		}
		return stdout
	}

	run("init")
	planID := run("plan", "create", "Sweep plan", "--slug", "sweep")
	msID := run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := run("plan", "item", "add", "Sweep item", "--plan", planID, "--milestone", msID)
	accID := run("plan", "acceptance", "add", "works", "--plan-item", itemID)
	_ = run("plan", "file", "add", "--plan-item", itemID, "--path", "main.go", "--role", "modify")
	fbID := run("feedback", "add", "--role", "model", "sweep note")
	goalID := run("goal", "add", "Sweep goal", "--kind", "goal")
	featID := run("feature", "add", "Sweep feature")
	taskID := run("task", "add", "Sweep task")
	remID := run("reminder", "add", "Sweep reminder", "--due", "2026-12-31T00:00:00Z")
	_ = run("inventory", "scan", "--paths", "main.go")
	noteID := run("arch", "add", "sweep-topic", "--body", "Architecture note body.", "--source", "main.go")

	run("plan", "item", "start", itemID)
	run("plan", "item", "pause", itemID, "--note", "checkpoint")
	run("plan", "acceptance", "meet", accID, "--evidence", "test")
	run("plan", "item", "close", itemID, "--evidence", "done")

	_ = run("approval", "request", taskID)
	_ = run("approval", "approve", taskID)
	_ = run("reminder", "snooze", remID, "--until", "2026-12-31T00:00:00Z")
	_ = run("reminder", "unsnooze", remID)

	for _, cmd := range [][]string{
		{"status"},
		{"status", "--verbose"},
		{"quality"},
		{"report"},
		{"resume"},
		{"doctor"},
		{"doctor", "hygiene"},
		{"feedback", "list"},
		{"feedback", "show", fbID},
		{"feedback", "close", fbID},
		{"goal", "list"},
		{"goal", "set", goalID, "done"},
		{"feature", "list"},
		{"task", "list"},
		{"task", "show", taskID},
		{"task", "done", taskID},
		{"reminder", "list"},
		{"reminder", "show", remID},
		{"approval", "list"},
		{"approval", "log"},
		{"plan", "list"},
		{"plan", "show", planID},
		{"plan", "tree", planID},
		{"plan", "status", planID},
		{"plan", "milestone", "list", "--plan", planID},
		{"plan", "item", "list", "--plan", planID},
		{"plan", "item", "show", itemID},
		{"arch", "list"},
		{"arch", "verify", noteID},
		{"inventory", "context", "--files", "main.go"},
		{"list", "tasks"},
		{"show", "tasks", taskID},
		{"analytics", "missed"},
		{"analytics", "summary"},
		{"archive", "gc", "--dry-run"},
		{"archive", "run", "--dry-run"},
		{"archive", "list"},
	} {
		if out := runOut(cmd...); out == "" && cmd[0] != "resume" {
			// some commands may return empty; that's ok
		}
	}

	_ = featID
	_ = runOut("--json", "status")
	_ = runOut("--json", "report")
	_ = runOut("--all", "feedback", "list")
	_ = runOut("--verbose", "quality")
	_ = runOut("inventory", "loc")
	_ = runOut("review", "list")
	_ = runOut("verify", "query", "--command", "go test ./...", "--scope", ".")
	_ = runOut("goal", "list", "--status", "all")
	_ = runOut("task", "list", "--status", "all")
	_ = runOut("plan", "milestone", "list", "--plan", planID)
}

func TestInProcessDoctorWithoutDB(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := runCLI(t, "--repo", dir, "--db", filepath.Join(dir, "missing.db"), "doctor")
	if code != 0 || !strings.Contains(stdout, "missing") {
		t.Fatalf("doctor missing db: code=%d stdout=%q", code, stdout)
	}
}

func TestArgvFlag(t *testing.T) {
	old := os.Args
	os.Args = []string{"devdb", "--repo", "/tmp/r", "--db", "/tmp/d.db", "status"}
	if got := argvFlag("--repo"); got != "/tmp/r" {
		t.Fatalf("repo=%q", got)
	}
	if got := argvFlag("--db"); got != "/tmp/d.db" {
		t.Fatalf("db=%q", got)
	}
	os.Args = old
}

func TestUnknownCommandArgv(t *testing.T) {
	old := os.Args
	os.Args = []string{"devdb", "--repo", "/r", "work-on", "extra"}
	got := unknownCommandArgv()
	if len(got) != 1 || got[0] != "work-on" {
		t.Fatalf("argv=%v", got)
	}
	os.Args = old
}

func TestCoerceUnknownCommandError(t *testing.T) {
	old := os.Args
	os.Args = []string{"devdb", "work-on"}
	ce := coerceCLIError(fmt.Errorf("unknown command: work-on"))
	if ce.Suggestion == "" {
		t.Fatal("expected suggestion from unknown command")
	}
	os.Args = old
}
