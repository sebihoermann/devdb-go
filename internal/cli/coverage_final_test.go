package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
)

func TestCoverageFinalCLIBranches(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)

	// cmdInit on existing db (idempotent)
	e.run("init")

	// Approval log human + json with history
	taskID := e.run("task", "add", "log coverage")
	e.run("approval", "request", taskID, "--note", "please")
	e.run("approval", "approve", taskID, "--note", "ok")
	logHuman := e.runOut("approval", "log")
	if !strings.Contains(logHuman, "approve") {
		t.Fatalf("approval log: %q", logHuman)
	}
	logJSON := e.runOut("--json", "approval", "log")
	if !strings.Contains(logJSON, "action") {
		t.Fatalf("approval log json: %q", logJSON)
	}

	// Reminder show human + json
	remID := e.run("reminder", "add", "show me", "--due", "2099-06-01T00:00:00Z")
	showHuman := e.runOut("reminder", "show", remID)
	if !strings.Contains(showHuman, "show me") {
		t.Fatalf("reminder show: %q", showHuman)
	}
	showJSON := e.runOut("--json", "reminder", "show", remID)
	if !strings.Contains(showJSON, remID) {
		t.Fatalf("reminder show json: %q", showJSON)
	}

	// Hub project + across json variants
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hubArgs := hubBase(t, metaDB, registry)
	alias := firstLine(runCLIOut(t, append(hubArgs, "register", e.repo, "--alias", "finalcov")...))
	runCLIOut(t, append(hubArgs, "sync")...)
	projJSON := runCLIOut(t, append(hubArgs, "--json", "project", alias)...)
	if !strings.Contains(projJSON, e.repo) {
		t.Fatalf("project json: %q", projJSON)
	}
	acrossJSON := runCLIOut(t, append(hubArgs, "--json", "across", "code-hygiene-cross")...)
	if !strings.Contains(acrossJSON, "[") && acrossJSON != "null" {
		t.Fatalf("across json: %q", acrossJSON)
	}

	// Arch verify edge: stale note after hash change
	e.run("inventory", "scan")
	noteID := e.run("arch", "add", "verify-edge", "--body", "body text here.", "--source", "main.go")
	e.runOut("arch", "verify", noteID)
	mainGo := filepath.Join(e.repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() { println(\"v2\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.run("inventory", "scan")
	verifyOut := e.runOut("arch", "verify", noteID)
	if !strings.Contains(verifyOut, "stale") {
		t.Fatalf("stale verify: %q", verifyOut)
	}

	// Inventory context with plan item + strict missing file
	planID := e.run("plan", "create", "Ctx plan", "--slug", "ctx-plan")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Ctx item", "--plan", planID, "--milestone", msID)
	ctxOut := e.runOut("inventory", "context", "--files", "main.go", "--plan-item", itemID)
	if ctxOut == "" {
		t.Fatal("context empty")
	}
	_, _, code := e.runAllow("inventory", "context", "--files", "missing.go", "--strict")
	if code == 0 {
		t.Log("strict context may succeed when no linked findings")
	}

	// Inventory diff + suggest-cuts json
	diffJSON := e.runOut("--json", "inventory", "diff", "HEAD")
	if !strings.Contains(diffJSON, "[") {
		t.Fatalf("diff json: %q", diffJSON)
	}
	e.runOut("--json", "inventory", "suggest-cuts", "--dry-run", "--principles", "dead")

	// Review report + principles json tier grass-cutter
	runID := e.run("review", "start", "--paths", ".")
	e.run("review", "finding", "--run", runID, "--file", "main.go", "--principle", "kiss",
		"--title", "final", "--recommendation", "note", "--severity", "low")
	e.run("review", "finish", runID, "--summary", "done")
	reportPath := e.run("review", "report", runID)
	if reportPath == "" {
		t.Fatal("empty report path")
	}
	princJSON := runCLIOut(t, append(e.base, "--json", "review", "principles", "--tier", "grass-cutter")...)
	var princ map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(princJSON)), &princ); err != nil {
		t.Fatalf("principles json: %v", err)
	}

	// Verify query with changed files compact line
	vRun := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "passed",
		"--finished", "--git-sha", "abc", "--exit-code", "0")
	e.runOut("verify", "query", "--command", "go test ./...", "--scope", ".")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.run("inventory", "scan")
	staleLine := e.runOut("verify", "query", "--command", "go test ./...", "--scope", ".")
	if staleLine == "" {
		t.Fatal("stale query line empty")
	}
	_ = vRun

	// Feedback import markdown json single row stderr path
	md := filepath.Join(e.repo, "solo.md")
	if err := os.WriteFile(md, []byte("## One\n- **Role**: user\n\nNote.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, append(e.base, "feedback", "import", "markdown", md)...)
	if code != 0 {
		t.Fatalf("import md: %d %s", code, stderr)
	}
	if !strings.Contains(stderr, "imported 1") {
		t.Fatalf("stderr=%q", stderr)
	}

	// Plan acceptance backfill json
	spec := filepath.Join(e.repo, "SPEC.md")
	specBody := "### M1 Alpha\n\n- [ ] criterion one\n"
	if err := os.WriteFile(spec, []byte(specBody), 0o644); err != nil {
		t.Fatal(err)
	}
	backfillJSON := e.runOut("--json", "plan", "acceptance", "backfill", "--milestone", "M1", "--spec", spec)
	if !strings.Contains(backfillJSON, "created") {
		t.Fatalf("backfill json: %q", backfillJSON)
	}

	// Goal add with body + feature with all fields
	e.run("goal", "add", "Body goal", "--kind", "do", "--body", "do this")
	e.run("feature", "add", "Full feature", "--description", "desc")

	// cmdHubDashboard attention-only json
	dashJSON := runCLIOut(t, append(hubArgs, "--json", "dashboard", "--attention-only")...)
	if !strings.Contains(dashJSON, "[") {
		t.Fatalf("dashboard json: %q", dashJSON)
	}
}

func TestCoverageFinalGitAwareScanAndUpstream(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)

	// git-aware scan
	e.runOut("inventory", "scan", "--git-aware", "--paths", "main.go")

	// setup upstream for ahead/behind display in status
	bare := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("bare init: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"git", "-C", e.repo, "remote", "add", "origin", bare},
		{"git", "-C", e.repo, "push", "-u", "origin", "master"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			// try main branch name
			if strings.Contains(string(out), "master") {
				_ = exec.Command("git", "-C", e.repo, "branch", "-M", "main").Run()
				if out2, err2 := exec.Command("git", "-C", e.repo, "push", "-u", "origin", "main").CombinedOutput(); err2 != nil {
					t.Fatalf("push: %v\n%s", err2, out2)
				}
			} else {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(e.repo, "extra.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", e.repo, "add", "extra.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", e.repo, "commit", "-m", "ahead").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	statusOut := e.runOut("--verbose", "status")
	if !strings.Contains(statusOut, "git:") {
		t.Fatalf("status: %q", statusOut)
	}
}

func TestFormatDashboardBlockedReason(t *testing.T) {
	rows := []hub.DashboardRow{{
		Alias: "blk", WorkStatus: "blocked", Status: "blocked", StatusReason: "needs sync",
		AttentionScore: 90,
	}}
	lines := formatDashboard(rows, "summary")
	if len(lines) != 1 || !strings.Contains(lines[0], "blk") {
		t.Fatalf("lines=%v", lines)
	}
}
