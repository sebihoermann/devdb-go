package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageBoost3TargetedHumanPaths(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	// cmdApprovalLog + cmdReminderShow human
	taskID := e.run("task", "add", "approval log target")
	e.run("approval", "request", taskID, "--note", "review")
	e.run("approval", "approve", taskID, "--note", "ok")
	logOut := e.runOut("approval", "log")
	if !strings.Contains(logOut, "approve") {
		t.Fatalf("log=%q", logOut)
	}
	remID := e.run("reminder", "add", "show target", "--due", "2099-08-01T00:00:00Z")
	showRem := e.runOut("reminder", "show", remID)
	if !strings.Contains(showRem, "show target") {
		t.Fatalf("reminder show=%q", showRem)
	}

	// cmdArchVerify + render + update human
	noteID := e.run("arch", "add", "verify3", "--body", "architecture body text.", "--source", "main.go")
	e.runOut("arch", "verify", noteID)
	e.runOut("arch", "render", noteID)
	e.runOut("arch", "update", noteID, "--body", "updated architecture body.")

	// cmdReviewReport human
	runID := e.run("review", "start", "--paths", ".")
	e.run("review", "finding", "--run", runID, "--file", "main.go", "--principle", "kiss",
		"--title", "report", "--recommendation", "doc", "--severity", "low")
	e.run("review", "finish", runID, "--summary", "done")
	reportPath := e.runOut("review", "report", runID)
	if reportPath == "" {
		t.Fatal("empty report")
	}

	// cmdVerifyRecord with inputs
	e.runOut("verify", "record", "go test ./internal/...", "--scope", ".", "--status", "passed",
		"--finished", "--git-sha", "deadbeef", "--exit-code", "0", "--notes", "ok",
		"--inputs", "main.go:scope:abc123")

	// cmdInventorySuggestCuts human persist path
	e.runOut("inventory", "suggest-cuts", "--principles", "dead", "--dry-run")
	e.run("inventory", "suggest-cuts", "--principles", "dead")

	// cmdPlanAcceptanceMeet + milestone list human
	planID := e.run("plan", "create", "Boost3", "--slug", "boost3-plan")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Item", "--plan", planID, "--milestone", msID)
	accID := e.run("plan", "acceptance", "add", "--plan-item", itemID, "done when tests pass")
	e.runOut("plan", "acceptance", "meet", accID, "--evidence", "green")
	e.runOut("plan", "milestone", "list", "--plan", planID)
	e.runOut("plan", "show", planID)

	// cmdInit idempotent
	e.runOut("init")
}

func TestCoverageBoost3HubAcrossAndSync(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", "main.go")
	e.run("review", "finding", "--run", runID, "--file", "main.go", "--principle", "dry",
		"--title", "dup", "--recommendation", "extract", "--severity", "high")
	e.run("review", "finish", runID, "--summary", "x")

	dir := t.TempDir()
	metaDB := filepath.Join(dir, "meta.db")
	registry := filepath.Join(dir, "reg.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", e.repo, "--alias", "boost3")...)
	syncOut := runCLIOut(t, append(hub, "sync")...)
	if syncOut == "" {
		t.Fatal("empty sync")
	}
	acrossOut := runCLIOut(t, append(hub, "across", "code-hygiene-cross")...)
	if acrossOut == "" {
		t.Fatal("empty across")
	}
	runCLIOut(t, append(hub, "across", "open-debt")...)
}

func TestCoverageBoost3FeedbackImportMarkdownMulti(t *testing.T) {
	e := newInProcessEnv(t)
	md := filepath.Join(e.repo, "multi2.md")
	body := "## A\n- **Role**: user\n\nNote A.\n\n## B\n- **Role**: model\n\nNote B.\n"
	if err := os.WriteFile(md, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(t, append(e.base, "feedback", "import", "markdown", md)...)
	if code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "imported") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestCoverageBoost3ReviewImportJSONL(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	runID := e.run("review", "start", "--paths", ".")
	jsonl := filepath.Join(e.repo, "findings.jsonl")
	line := `{"file":"main.go","principle":"kiss","title":"imported","recommendation":"fix","severity":"low","confidence":"medium","effort":"small"}`
	if err := os.WriteFile(jsonl, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	importID := e.run("review", "import", "--run", runID, "--file", jsonl)
	e.runOut("review", "resolve", importID, "--evidence", "fixed")
}
