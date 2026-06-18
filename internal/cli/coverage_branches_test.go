package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageBranchesHumanPaths(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	// Human read paths (not --json)
	humanReads := [][]string{
		{"status"}, {"quality"}, {"report"}, {"resume"}, {"doctor"},
		{"feedback", "list"}, {"goal", "list"}, {"feature", "list"},
		{"task", "list"}, {"reminder", "list"}, {"plan", "list"},
		{"arch", "list"}, {"review", "list"}, {"inventory", "loc"},
		{"approval", "list"}, {"approval", "log"},
	}
	for _, args := range humanReads {
		out := e.runOut(args...)
		if strings.TrimSpace(out) == "" {
			t.Fatalf("empty human output for %v", args)
		}
	}

	planID := e.run("plan", "create", "Human", "--slug", "human-plan")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "In flight", "--plan", planID, "--milestone", msID)
	e.run("plan", "item", "start", itemID)
	e.runOut("plan", "item", "show", itemID)
	e.runOut("resume")

	taskID := e.run("task", "add", "approve me")
	e.run("approval", "request", taskID, "--note", "please")
	e.run("approval", "reject", taskID, "--note", "no")
	e.runOut("approval", "log")

	remID := e.run("reminder", "add", "human show", "--due", "2099-06-01T00:00:00Z")
	e.runOut("reminder", "show", remID)
	e.run("reminder", "dismiss", remID)

	noteID := e.run("arch", "add", "human-topic", "--body", "body text here.", "--source", "main.go")
	e.runOut("arch", "verify", noteID)
	e.runOut("arch", "verify", "all")
	runCLIOut(t, append(e.base, "--all", "arch", "verify")...)

	e.runOut("inventory", "scan", "--dry-run")
	e.runOut("inventory", "context", "--files", "main.go")
	e.runOut("doctor", "hygiene")
}

func TestCoverageBranchesHubProjectHuman(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	planID := e.run("plan", "create", "Hub", "--slug", "hub-attn")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Attention", "--plan", planID, "--milestone", msID)
	e.run("plan", "item", "start", itemID)
	e.run("feedback", "add", "--role", "model", "open feedback")

	dir := t.TempDir()
	metaDB := filepath.Join(dir, "meta.db")
	registry := filepath.Join(dir, "reg.json")
	hub := hubBase(t, metaDB, registry)
	alias := firstLine(runCLIOut(t, append(hub, "register", e.repo, "--alias", "humanhub")...))
	runCLIOut(t, append(hub, "sync")...)

	projHuman := runCLIOut(t, append(hub, "project", alias)...)
	if !strings.Contains(projHuman, alias) {
		t.Fatalf("project human=%q", projHuman)
	}
	acrossHuman := runCLIOut(t, append(hub, "across", "open-debt")...)
	if acrossHuman == "" {
		t.Fatal("across human empty")
	}
	dashHuman := runCLIOut(t, append(hub, "dashboard")...)
	if !strings.Contains(dashHuman, alias) {
		t.Fatalf("dashboard=%q", dashHuman)
	}
}

func TestCoverageBranchesErrorsAndEdges(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	_, stderr, code := runCLI(t, "--db", dbPath, "status")
	if code == 0 {
		t.Fatal("expected missing db error")
	}
	if !strings.Contains(stderr, "database") && !strings.Contains(stderr, "devdb") {
		t.Fatalf("stderr=%q", stderr)
	}

	e := newInProcessEnv(t)
	_, _, code = e.runAllow("arch", "verify", "00000000000000000000000000000000")
	if code == 0 {
		t.Fatal("expected not found for arch verify")
	}
	_, _, code = e.runAllow("verify", "show", "missing", "--view", "summary")
	if code == 0 {
		t.Fatal("expected verify show error")
	}
	_, _, code = e.runAllow("plan", "item", "show", "bad-id")
	if code == 0 {
		t.Fatal("expected plan item show error")
	}

	// init on fresh path
	fresh := filepath.Join(t.TempDir(), "repo")
	_ = os.MkdirAll(fresh, 0o755)
	freshDB := filepath.Join(t.TempDir(), "fresh.db")
	runCLIOut(t, "--repo", fresh, "--db", freshDB, "init")
}

func TestCoverageBranchesExtendedWritesHuman(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	fbID := e.run("feedback", "add", "--role", "model", "note")
	e.runOut("feedback", "annotate", fbID, "extra context")
	goalID := e.run("goal", "add", "G")
	e.runOut("goal", "set", goalID, "active")
	e.runOut("task", "done", e.run("task", "add", "done task"))

	planID := e.run("plan", "create", "Acc", "--slug", "acc-plan")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Item", "--plan", planID, "--milestone", msID)
	accID := e.run("plan", "acceptance", "add", "--plan-item", itemID, "criterion met")
	e.runOut("plan", "acceptance", "meet", accID, "--evidence", "tests pass")

	e.runOut("verify", "record", "go test ./...", "--scope", ".", "--status", "passed",
		"--finished", "--git-sha", "abc", "--exit-code", "0")
	e.runOut("verify", "query", "--command", "go test ./...", "--scope", ".")
}
