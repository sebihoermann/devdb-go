package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageBoost2HumanSweep(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	// Human-only variants for commands that default to JSON in other tests
	human := [][]string{
		{"plan", "show", e.run("plan", "create", "Show plan", "--slug", "show-plan")},
		{"plan", "tree", e.run("plan", "create", "Tree plan", "--slug", "tree-plan")},
		{"plan", "status", e.run("plan", "create", "Status plan", "--slug", "status-plan")},
		{"show", "tasks", e.run("task", "add", "show task")},
		{"show", "goals", e.run("goal", "add", "show goal")},
		{"show", "features", e.run("feature", "add", "show feature")},
		{"show", "feedback", e.run("feedback", "add", "--role", "model", "show fb")},
		{"arch", "show", e.run("arch", "add", "show-arch", "--body", "body for show.", "--source", "main.go")},
		{"verify", "show", e.run("verify", "record", "go test", "--scope", ".", "--status", "passed", "--finished", "--exit-code", "0"), "--view", "summary"},
		{"verify", "show", e.run("verify", "record", "go test2", "--scope", ".", "--status", "failed", "--finished", "--exit-code", "1"), "--view", "failures"},
		{"inventory", "diff", "HEAD"},
		{"inventory", "suggest-cuts", "--dry-run", "--principles", "dead"},
		{"review", "principles"},
		{"review", "principles", "--tier", "extended"},
	}
	for _, args := range human {
		out := e.runOut(args...)
		if strings.TrimSpace(out) == "" && args[0] == "inventory" && args[1] == "suggest-cuts" {
			continue
		}
		if strings.TrimSpace(out) == "" {
			t.Fatalf("empty output for %v", args)
		}
	}

	planID := e.run("plan", "create", "Mile", "--slug", "mile-plan")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	e.runOut("plan", "milestone", "list", "--plan", planID)
	e.runOut("plan", "milestone", "status", msID, "in_progress")
	itemID := e.run("plan", "item", "add", "Item", "--plan", planID, "--milestone", msID)
	e.runOut("plan", "item", "list", "--plan", planID)
	e.runOut("plan", "item", "status", itemID, "in_progress")
	e.runOut("plan", "file", "add", "--plan-item", itemID, "--path", "main.go", "--role", "modify")

	e.runOut("approval", "withdraw", e.run("approval", "request", e.run("task", "add", "ap"), "--note", "n"), "--note", "never mind")
	e.runOut("reminder", "snooze", e.run("reminder", "add", "later", "--due", "2099-01-01T00:00:00Z"), "--until", "2099-06-01T00:00:00Z")
	e.runOut("reminder", "unsnooze", e.run("reminder", "add", "wake", "--due", "2099-01-02T00:00:00Z"))

	e.runOut("analytics", "missed", "--since", "2000-01-01T00:00:00Z")
	e.runOut("analytics", "summary")
}

func TestCoverageBoost2ArchiveAndDoctor(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("feedback", "add", "--role", "model", "old")
	fbID := e.run("feedback", "add", "--role", "model", "closed")
	e.run("feedback", "close", fbID)
	e.runOut("archive", "run", "--dry-run", "--yes")
	e.runOut("archive", "gc", "--dry-run")
	e.runOut("doctor", "hygiene")
}

func TestCoverageBoost2HubHuman(t *testing.T) {
	e := newInProcessEnv(t)
	dir := t.TempDir()
	metaDB := filepath.Join(dir, "meta.db")
	registry := filepath.Join(dir, "reg.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", e.repo, "--alias", "boost2")...)
	runCLIOut(t, append(hub, "sync")...)
	runCLIOut(t, append(hub, "list")...)
	runCLIOut(t, append(hub, "list", "--refresh")...)
	runCLIOut(t, append(hub, "doctor", "--project", "boost2")...)
	runCLIOut(t, append(hub, "across", "code-hygiene-cross")...)
	runCLIOut(t, append(hub, "across", "similar-feedback", "--keyword", "model")...)
}

func TestCoverageBoost2ImportMarkdownMulti(t *testing.T) {
	e := newInProcessEnv(t)
	md := filepath.Join(e.repo, "multi.md")
	body := "## One\n- **Role**: user\n\nNote one.\n\n## Two\n- **Role**: model\n\nNote two.\n"
	if err := os.WriteFile(md, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, append(e.base, "feedback", "import", "markdown", md)...)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "imported") {
		t.Fatalf("stderr=%q", stderr)
	}
}
