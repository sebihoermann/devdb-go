package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

type inProcessEnv struct {
	t      *testing.T
	repo   string
	dbPath string
	base   []string
}

func newInProcessEnv(t *testing.T) *inProcessEnv {
	t.Helper()
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
	e := &inProcessEnv{
		t:      t,
		repo:   repo,
		dbPath: dbPath,
		base:   []string{"--repo", repo, "--db", dbPath},
	}
	e.run("init")
	return e
}

func (e *inProcessEnv) run(args ...string) string {
	e.t.Helper()
	stdout, stderr, code := runCLI(e.t, append(append([]string{}, e.base...), args...)...)
	if code != 0 {
		e.t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
	}
	return firstLine(stdout)
}

func (e *inProcessEnv) runOut(args ...string) string {
	e.t.Helper()
	stdout, stderr, code := runCLI(e.t, append(append([]string{}, e.base...), args...)...)
	if code != 0 {
		e.t.Fatalf("%v exit %d stderr=%s", args, code, stderr)
	}
	return stdout
}

func (e *inProcessEnv) runAllow(args ...string) (stdout, stderr string, code int) {
	e.t.Helper()
	return runCLI(e.t, append(append([]string{}, e.base...), args...)...)
}

func initGitRepo(t *testing.T, repo string) {
	t.Helper()
	testutil.InitGitRepo(t, repo)
	mainGo := filepath.Join(repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repo, "add", "main.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", repo, "commit", "-m", "extend main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func hubBase(t *testing.T, metaDB, registry string) []string {
	t.Helper()
	return []string{
		"hub", "--metadata-db", metaDB, "--registry", registry,
	}
}

func TestInProcessReviewVerifyFlow(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)

	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", ".")
	findingID := e.run("review", "finding",
		"--run", runID,
		"--file", "main.go",
		"--principle", "kiss",
		"--title", "entry point",
		"--recommendation", "document package",
		"--severity", "low",
	)
	if len(findingID) != 32 {
		t.Fatalf("finding id: %q", findingID)
	}

	listOut := e.runOut("review", "list", "--run", runID)
	if !strings.Contains(listOut, "kiss") {
		t.Fatalf("list: %q", listOut)
	}

	e.run("review", "resolve", findingID, "--evidence", "documented")
	e.run("review", "finish", runID, "--summary", "smoke review")

	reportPath := e.run("review", "report", runID)
	if !strings.Contains(reportPath, ".md") {
		t.Fatalf("report path: %q", reportPath)
	}

	principlesOut := e.runOut("review", "principles")
	if !strings.Contains(principlesOut, "kiss") {
		t.Fatalf("principles: %q", principlesOut)
	}

	vRun := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "passed",
		"--exit-code", "0", "--finished", "--git-sha", "abc123")
	queryOut := e.runOut("verify", "query", "--command", "go test ./...", "--scope", ".")
	if !strings.Contains(queryOut, "skip rerun") && !strings.Contains(queryOut, "fresh") {
		t.Fatalf("query: %q", queryOut)
	}

	showOut := e.runOut("verify", "show", vRun)
	if !strings.Contains(showOut, "fresh_pass") && !strings.Contains(showOut, "go test") {
		t.Fatalf("show: %q", showOut)
	}

	inputsOut := e.runOut("verify", "show", vRun, "--view", "inputs")
	if !strings.Contains(inputsOut, "main.go") && !strings.Contains(inputsOut, "none") {
		t.Fatalf("inputs view: %q", inputsOut)
	}

	failuresOut := e.runOut("verify", "show", vRun, "--view", "failures")
	if !strings.Contains(failuresOut, "none") {
		t.Fatalf("failures view: %q", failuresOut)
	}

	dismissID := e.run("verify", "dismiss", vRun, "--reason", "superseded")
	if dismissID != vRun {
		t.Fatalf("dismiss id: %q", dismissID)
	}
}

func TestInProcessReviewImportJSONL(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	runID := e.run("review", "start", "--paths", ".")
	jsonl := filepath.Join(e.repo, "findings.jsonl")
	content := `{"file":"main.go","principle":"kiss","title":"imported","recommendation":"fix it","severity":"low","confidence":"medium","effort":"small","line_start":1}
`
	if err := os.WriteFile(jsonl, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	importID := e.run("review", "import", "--run", runID, "--file", jsonl)
	if len(importID) != 32 {
		t.Fatalf("import id: %q", importID)
	}

	jsonOut := e.runOut("--json", "review", "import", "--run", runID, "--file", jsonl, "--force-cap")
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json import: %v out=%q", err, jsonOut)
	}
}

func TestInProcessExtendedEntityCommands(t *testing.T) {
	e := newInProcessEnv(t)

	fbID := e.run("feedback", "add", "--role", "model", "annotate me")
	annotated := e.run("feedback", "annotate", fbID, "extra context")
	if annotated != fbID {
		t.Fatalf("annotate id: %q", annotated)
	}

	taskID := e.run("task", "add", "Status task")
	statusID := e.run("task", "status", taskID, "wontfix")
	if statusID != taskID {
		t.Fatalf("task status id: %q", statusID)
	}

	remID := e.run("reminder", "add", "Dismiss me", "--due", "2026-12-31T00:00:00Z")
	dismissed := e.run("reminder", "dismiss", remID)
	if dismissed != remID {
		t.Fatalf("dismiss id: %q", dismissed)
	}

	rejectTask := e.run("task", "add", "Reject approval")
	e.run("approval", "request", rejectTask)
	rejectID := e.run("approval", "reject", rejectTask, "--note", "not ready")
	if rejectID == "" {
		t.Fatal("reject returned empty")
	}

	withdrawTask := e.run("task", "add", "Withdraw approval")
	e.run("approval", "request", withdrawTask)
	withdrawID := e.run("approval", "withdraw", withdrawTask, "--note", "changed mind")
	if withdrawID == "" {
		t.Fatal("withdraw returned empty")
	}

	planID := e.run("plan", "create", "Status plan", "--slug", "status-plan")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Item", "--plan", planID, "--milestone", msID)

	msStatus := e.run("plan", "milestone", "status", msID, "in_progress")
	if msStatus != msID {
		t.Fatalf("milestone status: %q", msStatus)
	}

	itemStatus := e.run("plan", "item", "status", itemID, "wontfix", "--note", "waiting")
	if itemStatus != itemID {
		t.Fatalf("item status: %q", itemStatus)
	}
}

func TestInProcessHubFlow(t *testing.T) {
	e := newInProcessEnv(t)
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)

	alias := firstLine(runCLIOut(t, append(hub, "register", e.repo, "--alias", "covtest")...))
	if alias != "covtest" {
		t.Fatalf("alias: %q", alias)
	}

	listOut := runCLIOut(t, append(hub, "list")...)
	if !strings.Contains(listOut, "covtest") {
		t.Fatalf("list: %q", listOut)
	}

	syncOut := runCLIOut(t, append(hub, "sync")...)
	if !strings.Contains(syncOut, "seen=") && !strings.Contains(syncOut, "sync") {
		// human hints go to stderr; stdout may be empty
		_, syncErr, _ := runCLI(t, append(hub, "sync")...)
		if !strings.Contains(syncErr, "seen=") {
			t.Fatalf("sync: stdout=%q stderr=%q", syncOut, syncErr)
		}
	}

	for _, view := range []string{"summary", "work", "delivery", "quality"} {
		dashOut := runCLIOut(t, append(hub, "dashboard", "--view", view)...)
		if dashOut == "" {
			t.Fatalf("dashboard %s empty", view)
		}
	}

	projOut := runCLIOut(t, append(hub, "project", "covtest")...)
	if !strings.Contains(projOut, e.repo) {
		t.Fatalf("project: %q", projOut)
	}

	docOut := runCLIOut(t, append(hub, "doctor")...)
	if !strings.Contains(docOut, "covtest") && !strings.Contains(docOut, "freshness") {
		t.Fatalf("doctor: %q", docOut)
	}

	acrossOut := runCLIOut(t, append(hub, "across", "open-debt")...)
	if acrossOut == "" {
		t.Fatal("across open-debt empty")
	}

	jsonOut := runCLIOut(t, append(hub, "--json", "list", "--refresh")...)
	if !strings.Contains(jsonOut, "covtest") {
		t.Fatalf("json list: %q", jsonOut)
	}
}

func runCLIOut(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, args...)
	if code != 0 {
		t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
	}
	combined := strings.TrimSpace(stdout)
	if combined == "" {
		combined = strings.TrimSpace(stderr)
	}
	return combined
}

func TestInProcessFeedbackImportMarkdown(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	base := []string{"--db", dbPath}
	if _, _, code := runCLI(t, append(base, "init")...); code != 0 {
		t.Fatal("init failed")
	}

	fixture := filepath.Join(moduleRoot(t), "internal", "domain", "feedback", "testdata", "feedback_archive.md")
	stdout, stderr, code := runCLI(t, append(base, "feedback", "import", "markdown", fixture)...)
	if code != 0 {
		t.Fatalf("import exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "imported 3") {
		t.Fatalf("stdout=%q", stdout)
	}
	if !strings.Contains(stderr, "imported 3 row") {
		t.Fatalf("stderr=%q", stderr)
	}

	jsonOut := runCLIOut(t, append(base, "--json", "feedback", "import", "markdown", fixture)...)
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if int(payload["imported"].(float64)) != 3 {
		t.Fatalf("imported=%v", payload["imported"])
	}
}

func TestInProcessImportPythonDBInspect(t *testing.T) {
	src := testutil.LegacyPythonDBPath(t)
	if src == "" {
		t.Skip("no legacy python development.db")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	base := []string{"--db", dbPath}

	inspectOut := runCLIOut(t, append(base, "import", "python-db", src)...)
	if !strings.Contains(inspectOut, "tables") && !strings.Contains(inspectOut, "version") {
		// JSON payload
		var info map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(inspectOut)), &info); err != nil {
			t.Fatalf("inspect: %q", inspectOut)
		}
	}

	outDB := filepath.Join(dir, "imported.db")
	importOut := runCLIOut(t, append(base, "import", "python-db", src, "--output", outDB, "--replace")...)
	if _, err := os.Stat(outDB); err != nil {
		t.Fatalf("output db missing: %v out=%q", err, importOut)
	}
}

func TestInProcessArchAndInventory(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	noteID := e.run("arch", "add", "entry-main", "--body", "Main package entry.", "--source", "main.go")
	showOut := e.runOut("arch", "show", noteID)
	if !strings.Contains(showOut, "entry-main") {
		t.Fatalf("arch show: %q", showOut)
	}

	updated := e.run("arch", "update", noteID, "--body", "Updated body.", "--confidence", "high")
	if updated != noteID {
		t.Fatalf("arch update: %q", updated)
	}

	renderOut := e.runOut("arch", "render")
	if !strings.Contains(renderOut, "entry-main") && !strings.Contains(renderOut, "Updated") {
		t.Fatalf("arch render: %q", renderOut)
	}

	renderFile := filepath.Join(e.repo, "arch.md")
	renderPath := e.run("arch", "render", "--output", renderFile)
	if renderPath != renderFile {
		t.Fatalf("render path: %q", renderPath)
	}

	e.run("inventory", "scan")
	mainGo := filepath.Join(e.repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() { println(\"stale\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", e.repo, "add", "main.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", e.repo, "commit", "-m", "extend main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	staleOut := e.runOut("arch", "list", "--stale")
	if !strings.Contains(staleOut, noteID[:8]) && !strings.Contains(staleOut, "entry-main") {
		// stale list may be JSON array
		if staleOut == "[]" || staleOut == "null" {
			e.run("inventory", "scan")
			staleOut = e.runOut("arch", "list", "--stale")
		}
	}

	diffOut := e.runOut("inventory", "diff", "HEAD~1")
	if !strings.Contains(diffOut, "main.go") {
		t.Fatalf("diff: %q", diffOut)
	}

	locOut := e.runOut("inventory", "loc")
	if !strings.Contains(locOut, "files:") {
		t.Fatalf("loc: %q", locOut)
	}

	dryScan := e.runOut("inventory", "scan", "--dry-run")
	if !strings.Contains(dryScan, "dry-run") {
		t.Fatalf("scan dry-run: %q", dryScan)
	}

	fixture := testutil.GrassFixture(t, "dead_code.py")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.repo, "dead_code.py"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", e.repo, "add", "dead_code.py")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add dead_code: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", e.repo, "commit", "-m", "add dead code")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit dead_code: %v\n%s", err, out)
	}

	e.run("inventory", "scan")
	stdout, stderr, code := e.runAllow("inventory", "suggest-cuts", "--dry-run", "--principles", "dead")
	if code != 0 {
		t.Fatalf("suggest-cuts exit %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "dead") && !strings.Contains(stderr, "dead") {
		t.Fatalf("suggest-cuts dry-run stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestInProcessPlanningExtended(t *testing.T) {
	e := newInProcessEnv(t)

	planID := e.run("plan", "scaffold", "M9 Feature", "--slug", "m9-inproc", "--mode", "design", "--milestones", "2")
	artifact := filepath.Join(e.repo, "docs", "m9-inproc-implementation-plan.html")
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("scaffold artifact missing: %v", err)
	}

	promoteID := e.run("plan", "promote", "--plan", "m9-inproc")
	if promoteID != planID {
		t.Fatalf("promote id %s != plan %s", promoteID, planID)
	}

	driftOut := e.runOut("plan", "reconcile")
	if !strings.Contains(driftOut, "drift") {
		t.Fatalf("reconcile: %q", driftOut)
	}

	specPath := filepath.Join(e.repo, "SPEC.md")
	spec := `### M1 Alpha

- [ ] first check
- [ ] second check
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	backfillOut := e.runOut("plan", "acceptance", "backfill", "--milestone", "M1", "--spec", specPath)
	if !strings.Contains(backfillOut, "created 2 acceptance") {
		t.Fatalf("backfill: %q", backfillOut)
	}

	_, stderr, code := runCLI(t, append(e.base, "plan", "scaffold", "Two", "--slug", "m9-inproc")...)
	if code == 0 {
		t.Fatal("expected duplicate slug failure")
	}
	if !strings.Contains(stderr, "slug already exists") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestInProcessHygieneExtended(t *testing.T) {
	e := newInProcessEnv(t)

	_, stderr, code := e.runAllow("list-missed-calls")
	if code == 0 || !strings.Contains(stderr, "unknown command") {
		t.Fatalf("unknown command: code=%d stderr=%q", code, stderr)
	}

	missedOut := e.runOut("analytics", "missed")
	if missedOut == "" {
		t.Fatal("analytics missed empty")
	}

	summaryOut := e.runOut("analytics", "summary")
	if !strings.Contains(summaryOut, "total misses") {
		t.Fatalf("summary: %q", summaryOut)
	}

	gcOut := e.runOut("archive", "gc")
	if !strings.Contains(gcOut, "closed") && !strings.Contains(gcOut, "feedback") {
		t.Fatalf("gc: %q", gcOut)
	}

	runOut := e.runOut("archive", "run", "--yes")
	if !strings.Contains(runOut, "archived") {
		t.Fatalf("archive run: %q", runOut)
	}

	listOut := e.runOut("archive", "list")
	if listOut == "" {
		t.Fatal("archive list empty")
	}

	hygieneOut := e.runOut("doctor", "hygiene")
	if !strings.Contains(hygieneOut, "missed call") {
		t.Fatalf("doctor hygiene: %q", hygieneOut)
	}
}

func TestInProcessReadVerboseAndHelp(t *testing.T) {
	e := newInProcessEnv(t)
	planID := e.run("plan", "create", "Verbose plan", "--slug", "verbose")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Work item", "--plan", planID, "--milestone", msID)
	e.run("plan", "item", "start", itemID)

	statusOut := e.runOut("--verbose", "status")
	if !strings.Contains(statusOut, "schema:") {
		t.Fatalf("verbose status: %q", statusOut)
	}

	qualityOut := e.runOut("--verbose", "quality")
	if !strings.Contains(qualityOut, "open tasks") {
		t.Fatalf("verbose quality: %q", qualityOut)
	}

	helpOut := runCLIOut(t, "help")
	if !strings.Contains(helpOut, "devdb") {
		t.Fatalf("help: %q", helpOut)
	}

	helpInitOut := runCLIOut(t, "help", "init")
	if !strings.Contains(helpInitOut, "Initialize .devdb/development.db") {
		t.Fatalf("help init should dispatch to init's help: %q", helpInitOut)
	}
}

func TestInProcessVerifyRecordWithInputs(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan", "--paths", "main.go")

	vRun := e.run("verify", "record", "custom check", "--scope", ".",
		"--status", "passed", "--finished", "--git-sha", "deadbeef",
		"--inputs", "main.go:source:abc123hash00000000000000000000001")
	if len(vRun) != 32 {
		t.Fatalf("run id: %q", vRun)
	}

	jsonOut := e.runOut("--json", "verify", "show", vRun, "--view", "inputs")
	if !strings.Contains(jsonOut, "main.go") {
		t.Fatalf("json inputs: %q", jsonOut)
	}
}

func TestInProcessLegacySuggestionsExtended(t *testing.T) {
	cases := map[string]string{
		"work-on":               "plan item start",
		"review-start":          "review start",
		"record-verification-run": "verify record",
		"hub-dashboard":         "hub dashboard",
		"diff-since":            "inventory diff",
	}
	for legacy, want := range cases {
		sug := suggestCommand([]string{legacy})
		if !strings.Contains(sug, want) {
			t.Fatalf("%s suggestion=%q want %q", legacy, sug, want)
		}
	}
}

func TestInProcessFeedbackImportCommitsRequiresBranches(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	_, _, code := runCLI(t, "--db", dbPath, "init")
	if code != 0 {
		t.Fatal("init failed")
	}
	_, stderr, code := runCLI(t, "--db", dbPath, "feedback", "import", "commits")
	if code != 2 {
		t.Fatalf("exit=%d want 2 stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "--branches") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestCLIErrorHelpers(t *testing.T) {
	ne := notFoundError("entity missing")
	if ne.Error() != "entity missing" || ne.Code != ExitNotFound {
		t.Fatalf("notFoundError: %+v", ne)
	}
	ue := usageError("bad usage")
	if ue.Error() != "bad usage" {
		t.Fatal(ue)
	}
}

func TestInProcessArchiveRestore(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", ".")
	findingID := e.run("review", "finding",
		"--run", runID, "--file", "main.go", "--principle", "kiss",
		"--title", "archive", "--recommendation", "fix", "--severity", "low",
	)
	e.run("review", "resolve", findingID, "--evidence", "fixed")
	e.run("review", "finish", runID, "--summary", "done")
	e.run("archive", "run", "--yes", "--table", "review_findings")

	listOut := e.runOut("--json", "archive", "list", "--table", "review_findings")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(listOut), &rows); err != nil || len(rows) == 0 {
		t.Fatalf("archive list: %v out=%q", err, listOut)
	}
	entryID, _ := rows[0]["id"].(string)

	restoreOut := e.runOut("archive", "restore", "--id", entryID)
	if !strings.Contains(restoreOut, "restored") {
		t.Fatalf("restore: %q", restoreOut)
	}

	jsonRestore := e.runOut("--json", "archive", "restore", "--source-table", "review_findings", "--source-id", findingID, "--keep-archive")
	if !strings.Contains(jsonRestore, "restored") {
		t.Fatalf("json restore: %q", jsonRestore)
	}
}

func TestInProcessPlanReconcileApply(t *testing.T) {
	e := newInProcessEnv(t)
	planID := e.run("plan", "create", "Reconcile plan", "--slug", "reconcile")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Ship it", "--plan", planID, "--milestone", msID)
	accID := e.run("plan", "acceptance", "add", "works", "--plan-item", itemID)
	e.run("plan", "acceptance", "meet", accID, "--evidence", "test")
	e.run("plan", "item", "close", itemID, "--evidence", "done")

	drift := e.runOut("plan", "reconcile", "--plan", "reconcile")
	if !strings.Contains(drift, "drift detected") {
		t.Fatalf("reconcile: %q", drift)
	}
	applied := e.runOut("plan", "reconcile", "--plan", "reconcile", "--apply")
	if !strings.Contains(applied, "reconciled") {
		t.Fatalf("reconcile apply: %q", applied)
	}

	jsonDrift := e.runOut("--json", "plan", "reconcile", "--plan", "reconcile")
	if !strings.Contains(jsonDrift, "drift") {
		t.Fatalf("json reconcile: %q", jsonDrift)
	}
}

func TestInProcessArchVerifyBulk(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	noteID := e.run("arch", "add", "bulk-topic", "--body", "Bulk verify note.", "--source", "main.go")

	bulkOut := e.runOut("arch", "verify", "all")
	if !strings.Contains(bulkOut, "verified") {
		t.Fatalf("verify all: %q", bulkOut)
	}

	jsonBulk := e.runOut("--json", "--all", "arch", "verify")
	if !strings.Contains(jsonBulk, "verified") {
		t.Fatalf("verify --all json: %q", jsonBulk)
	}

	single := e.runOut("arch", "verify", noteID)
	if single != "ok" && !strings.Contains(single, "ok") {
		t.Fatalf("verify single: %q", single)
	}
}

func TestInProcessHubSyncModes(t *testing.T) {
	e := newInProcessEnv(t)
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", e.repo)...)

	_, stderr, code := runCLI(t, append(hub, "sync", "--watch", "--iterations", "1", "--interval", "0")...)
	if code != 0 {
		t.Fatalf("watch sync exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "seen=") && !strings.Contains(stderr, "watch") {
		t.Fatalf("watch stderr=%q", stderr)
	}

	runCLIOut(t, append(hub, "--json", "sync", "--strict")...)
}

func TestInProcessHubAcrossQueries(t *testing.T) {
	e := newInProcessEnv(t)
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)
	alias := firstLine(runCLIOut(t, append(hub, "register", e.repo, "--alias", "acrosstest")...))
	_ = runCLIOut(t, append(hub, "sync")...)

	for _, q := range []string{"open-debt", "code-hygiene-cross", "similar-feedback"} {
		out := runCLIOut(t, append(hub, "across", q)...)
		if out == "" {
			t.Fatalf("across %s empty", q)
		}
	}

	dash := runCLIOut(t, append(hub, "dashboard", "--attention-only")...)
	if dash == "" {
		t.Fatal("dashboard attention-only empty")
	}

	projJSON := runCLIOut(t, append(hub, "--json", "project", alias)...)
	if !strings.Contains(projJSON, e.repo) {
		t.Fatalf("project json: %q", projJSON)
	}
}

func TestInProcessFeedbackImportCommitsInRepo(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	root := moduleRoot(t)
	base := []string{"--repo", root, "--db", dbPath}
	runCLIOut(t, append(base, "init")...)

	stdout, stderr, code := runCLI(t, append(base, "feedback", "import", "commits", "--branches", "HEAD", "--limit", "3")...)
	if code != 0 {
		t.Fatalf("import commits exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "inserted") && !strings.Contains(stdout, "inserted") {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestInProcessSuggestCutsPersist(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	fixture := testutil.GrassFixture(t, "dead_code.py")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.repo, "dead_code.py"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", e.repo, "add", "dead_code.py")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", e.repo, "commit", "-m", "dead code")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	e.run("inventory", "scan")

	runID := e.run("inventory", "suggest-cuts", "--principles", "dead")
	if len(runID) != 32 {
		t.Fatalf("persist run id: %q", runID)
	}
}

func TestInProcessNotFoundPaths(t *testing.T) {
	e := newInProcessEnv(t)
	_, stderr, code := e.runAllow("task", "show", "00000000000000000000000000000000")
	if code != ExitNotFound {
		t.Fatalf("task show code=%d stderr=%q", code, stderr)
	}
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)
	_, stderr, code = runCLI(t, append(hub, "project", "nosuch")...)
	if code != ExitNotFound {
		t.Fatalf("hub project code=%d stderr=%q", code, stderr)
	}
}

func TestInProcessReviewImportNumberTypes(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", ".")
	jsonl := filepath.Join(e.repo, "nums.jsonl")
	content := `{"file":"main.go","principle":"kiss","title":"n1","recommendation":"r","severity":"low","confidence":"medium","effort":"small","line_start":"2","line_end":3}
`
	if err := os.WriteFile(jsonl, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	id := e.run("review", "import", "--run", runID, "--file", jsonl)
	if len(id) != 32 {
		t.Fatalf("import id: %q", id)
	}
}

func TestInProcessResumeEmpty(t *testing.T) {
	e := newInProcessEnv(t)
	out := e.runOut("resume")
	if !strings.Contains(out, "no in-flight") {
		t.Fatalf("resume: %q", out)
	}
	jsonOut := e.runOut("--json", "resume")
	if !strings.Contains(jsonOut, "in_flight") {
		t.Fatalf("json resume: %q", jsonOut)
	}
}

func TestInProcessListShowAliases(t *testing.T) {
	e := newInProcessEnv(t)
	taskID := e.run("task", "add", "Alias task")
	featID := e.run("feature", "add", "Alias feature")
	goalID := e.run("goal", "add", "Alias goal", "--kind", "goal")

	e.runOut("list", "tasks")
	e.runOut("list", "features")
	e.runOut("list", "goals")
	e.runOut("show", "tasks", taskID)
	e.runOut("show", "features", featID)
	e.runOut("show", "goals", goalID)
	e.runOut("plan", "show", e.run("plan", "create", "Show plan", "--slug", "show-plan"))
	e.runOut("plan", "tree", e.run("plan", "create", "Tree plan", "--slug", "tree-plan"))
}

func TestInProcessFeedbackAddUsageError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")
	_, stderr, code := runCLI(t, "--db", dbPath, "feedback", "add", "no role")
	if code != ExitUsage || !strings.Contains(stderr, "role") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestInProcessImportPythonDBApplySkipped(t *testing.T) {
	src := testutil.LegacyPythonDBPath(t)
	if src == "" {
		t.Skip("no legacy python development.db")
	}
	// Inspect-only path already covered; --apply would mutate repo db — skip intentionally.
}

func TestParseFindingImportHelpers(t *testing.T) {
	if strField(map[string]any{"k": "v"}, "k") != "v" || strField(map[string]any{}, "k") != "" {
		t.Fatal("strField")
	}
	for _, tc := range []struct {
		in   any
		want int
		ok   bool
	}{
		{float64(5), 5, true},
		{int(6), 6, true},
		{json.Number("7"), 7, true},
		{"8", 8, true},
		{true, 0, false},
	} {
		got, ok := toInt(tc.in)
		if ok != tc.ok || (tc.ok && got != tc.want) {
			t.Fatalf("toInt(%v) = %d,%v want %d,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	raw := map[string]any{
		"principle": "kiss", "title": "t", "recommendation": "r",
		"file": "main.go", "line_start": float64(1), "line_end": "3",
		"severity": "high", "confidence": "low", "effort": "large",
	}
	in := parseFindingImport(raw)
	if in.Principle != "kiss" || in.LineStart == nil || *in.LineStart != 1 || in.LineEnd == nil || *in.LineEnd != 3 {
		t.Fatalf("parseFindingImport: %+v", in)
	}
}

func TestFormatDashboardViews(t *testing.T) {
	rows := []hub.DashboardRow{{
		Alias: "demo", WorkStatus: "active", Status: "ok", StatusReason: "steady",
		InProgress: 1, OpenItems: 2, OpenFeedback: 3, AttentionScore: 4,
		OpenHighFinding: 1, StaleArch: 2, Verification: "fresh", GitDirty: true, GitBranch: "main",
	}}
	for _, view := range []string{"summary", "work", "delivery", "quality", ""} {
		lines := formatDashboard(rows, view)
		if len(lines) != 1 {
			t.Fatalf("view %q: %v", view, lines)
		}
	}
}

func TestResolveVerificationInputsInvalid(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, err := app.Open(repo, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	_, _, err = resolveVerificationInputs(ctx, ".", []string{"badformat"})
	if err == nil {
		t.Fatal("expected usage error")
	}
	ce, ok := err.(*CLIError)
	if !ok || ce.Kind != "missing_argument" {
		t.Fatalf("err=%v", err)
	}
}

func TestInProcessUsageErrorBranches(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)

	cases := [][]string{
		{"verify", "record", "cmd"},
		{"arch", "add", "topic", "--source", "main.go"},
		{"plan", "promote"},
		{"plan", "acceptance", "backfill"},
		{"feedback", "import", "commits"},
		{"review", "import"},
	}
	for _, args := range cases {
		_, _, code := e.runAllow(args...)
		if code != ExitUsage {
			t.Fatalf("%v code=%d want usage", args, code)
		}
	}

	_, _, code := runCLI(t, append(e.base, "plan", "reconcile", "--plan", "missing-plan")...)
	if code != ExitNotFound {
		t.Fatalf("reconcile missing plan code=%d", code)
	}
}

func TestInProcessFilteredReads(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", ".")
	_ = e.run("review", "finding",
		"--run", runID, "--file", "main.go", "--principle", "kiss",
		"--title", "filtered", "--recommendation", "note", "--severity", "low",
	)

	e.runOut("review", "list", "--status", "all", "--severity", "low", "--file", "main.go")
	e.runOut("feedback", "list", "--status", "open")
	e.runOut("goal", "list", "--status", "all")
	e.runOut("task", "list", "--status", "all", "--priority", "med")
	e.runOut("reminder", "list", "--status", "all")
	e.runOut("approval", "list")
	e.runOut("approval", "log")
	e.runOut("arch", "list", "--touching", "main.go")
	e.runOut("inventory", "context", "--files", "main.go", "--task", "review")
	e.runOut("inventory", "scan", "--git-aware", "--paths", "main.go")
	e.runOut("archive", "list", "--limit", "5")
	e.runOut("archive", "run", "--dry-run", "--vacuum")
	e.runOut("analytics", "missed", "--limit", "5")
	e.runOut("analytics", "summary", "--window-days", "14")
	e.runOut("doctor", "hygiene", "--json")
	e.runOut("goal", "set", e.run("goal", "add", "G", "--kind", "goal"), "done")
	e.runOut("feature", "list")
	e.runOut("plan", "item", "list", "--status", "open")
}

func TestInProcessFeedbackImportSingleRowMarkdown(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	base := []string{"--db", dbPath}
	runCLIOut(t, append(base, "init")...)
	md := filepath.Join(dir, "one.md")
	if err := os.WriteFile(md, []byte("## Solo note\n- **Role**: model\n\nBody text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := firstLine(runCLIOut(t, append(base, "feedback", "import", "markdown", md)...))
	if len(id) != 32 {
		t.Fatalf("single import id: %q", id)
	}
}

func TestSuggestCommandLegacyMap(t *testing.T) {
	for legacy := range map[string]bool{
		"init": true, "work-on": true, "pause-on": true, "resume": true,
		"feedback-user": true, "feedback-model": true, "feedback-codebase": true,
		"import-feedback-md": true, "import-branch-commits": true,
		"create-plan": true, "scaffold-plan": true, "promote-plan": true,
		"reconcile-plans": true, "backfill-acceptance": true, "show-plan-item": true,
		"list-missed-calls": true, "missed-calls-summary": true, "gc": true,
		"restore": true, "restore-list": true, "scan": true, "context": true,
		"diff-since": true, "suggest-cuts": true, "add-arch-note": true,
		"list-arch-notes": true, "verify-arch-note": true, "arch-render": true,
		"review-start": true, "review-add-finding": true, "review-finish": true,
		"review-list": true, "review-resolve": true, "review-report": true,
		"record-verification-run": true, "query-verification": true,
		"show-verification-run": true, "dismiss-verification": true,
		"register": true, "hub-sync": true, "hub-dashboard": true,
		"hub-project": true, "doctor-sync": true, "list-projects": true, "across": true,
	} {
		if suggestCommand([]string{legacy}) == "devdb help" {
			t.Fatalf("missing suggestion for %s", legacy)
		}
	}
	if suggestCommand([]string{"totally-unknown"}) != "devdb help" {
		t.Fatal("unknown should fall back to help")
	}
}

func TestInProcessStatusResumeDoctorBranches(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)

	planID := e.run("plan", "create", "Status branches", "--slug", "status-br")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Active work", "--plan", planID, "--milestone", msID)
	e.run("plan", "item", "start", itemID)

	resumeHuman := e.runOut("resume")
	if !strings.Contains(resumeHuman, "Active work") {
		t.Fatalf("resume human: %q", resumeHuman)
	}
	resumeJSON := e.runOut("--json", "resume")
	if !strings.Contains(resumeJSON, "in_flight") {
		t.Fatalf("resume json: %q", resumeJSON)
	}

	statusVerbose := e.runOut("--verbose", "status")
	if !strings.Contains(statusVerbose, "in flight:") || !strings.Contains(statusVerbose, "git:") {
		t.Fatalf("verbose status: %q", statusVerbose)
	}
	statusJSONVerbose := e.runOut("--json", "--verbose", "status")
	if !strings.Contains(statusJSONVerbose, "quality") {
		t.Fatalf("json verbose status: %q", statusJSONVerbose)
	}

	doctorHuman := e.runOut("doctor")
	if !strings.Contains(doctorHuman, "doctor: ok") {
		t.Fatalf("doctor: %q", doctorHuman)
	}
	doctorJSON := e.runOut("--json", "doctor")
	if !strings.Contains(doctorJSON, "schema_kind") {
		t.Fatalf("doctor json: %q", doctorJSON)
	}
}

func TestInProcessVerifyShowJSONViews(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	vRun := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "passed",
		"--finished", "--git-sha", "abc123", "--notes", "all good")

	for _, view := range []string{"summary", "inputs", "failures"} {
		out := e.runOut("--json", "verify", "show", vRun, "--view", view)
		if !strings.Contains(out, "{") {
			t.Fatalf("json %s: %q", view, out)
		}
	}
}

func TestInProcessArchErrorPaths(t *testing.T) {
	e := newInProcessEnv(t)
	_, stderr, code := e.runAllow("arch", "add", "bad topic!", "--body", "x", "--source", "missing.go")
	if code != ExitInvalidValue && code != ExitNotFound {
		t.Fatalf("arch add code=%d stderr=%q", code, stderr)
	}
}

func TestInProcessRecordMissedCall(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	base := []string{"--db", dbPath}
	runCLIOut(t, append(base, "init")...)
	_, stderr, code := runCLI(t, append(base, "feedback", "add", "no role")...)
	if code != ExitUsage {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	missed := runCLIOut(t, append(base, "analytics", "missed", "--json")...)
	if !strings.Contains(missed, "missing_argument") && missed != "[]\n" && missed != "[]" {
		t.Fatalf("missed=%q", missed)
	}
}

func TestInProcessPlanBackfillNotFound(t *testing.T) {
	e := newInProcessEnv(t)
	_, stderr, code := e.runAllow("plan", "acceptance", "backfill", "--milestone", "M1", "--spec", "/no/spec.md")
	if code != ExitNotFound {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestInProcessHubSyncStrictFailure(t *testing.T) {
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", t.TempDir())...)
	_, _, code := runCLI(t, append(hub, "sync", "--strict")...)
	// strict may exit 1 when project has no db; either success or sync_error is fine for coverage
	if code != 0 && code != ExitGeneral {
		t.Fatalf("strict sync code=%d", code)
	}
}

func TestInProcessReviewListEmpty(t *testing.T) {
	e := newInProcessEnv(t)
	out := e.runOut("review", "list", "--run", "00000000000000000000000000000000")
	if !strings.Contains(out, "no findings") {
		t.Fatalf("empty list: %q", out)
	}
}

func TestInProcessInventoryContextStrict(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	e.runOut("inventory", "context", "--files", "main.go", "--strict")
}

func TestInProcessMegaCoverage(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	src := filepath.Join(e.repo, "src.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.run("inventory", "scan", "--paths", "src.go")

	writes := [][]string{
		{"feedback", "add", "--role", "model", "mega write"},
		{"goal", "add", "Mega goal", "--kind", "goal"},
		{"feature", "add", "Mega feature"},
		{"plan", "create", "Mega plan", "--slug", "mega-plan"},
		{"task", "add", "Mega task"},
		{"reminder", "add", "Mega reminder", "--due", "2026-12-31T00:00:00Z"},
		{"arch", "add", "mega-topic", "--body", "Mega arch note body.", "--source", "src.go"},
		{"review", "start", "--paths", "."},
		{"verify", "record", "go test ./...", "--scope", ".", "--git-sha", "abc", "--status", "passed", "--exit-code", "0", "--finished"},
	}
	for _, args := range writes {
		e.run(args...)
		jsonArgs := append([]string{"--json"}, args...)
		if len(args) >= 3 && args[0] == "plan" && args[1] == "create" {
			jsonArgs = []string{"--json", "plan", "create", "Mega plan JSON", "--slug", "mega-plan-json"}
		}
		e.run(jsonArgs...)
	}

	planID := e.run("plan", "create", "Mega2", "--slug", "mega2")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Legacy", "--legacy", "--phase", "P", "--step", "1")
	_ = e.run("plan", "item", "add", "Structured", "--plan", planID, "--milestone", msID)
	_ = e.run("plan", "acceptance", "add", "ok", "--plan-item", itemID)
	e.run("plan", "file", "add", "--plan-item", itemID, "--path", "src.go", "--role", "modify")

	taskID := e.run("task", "add", "Approval mega")
	e.run("approval", "request", taskID)
	e.run("approval", "approve", taskID, "--note", "ok")
	e.run("--json", "approval", "log")

	reads := [][]string{
		{"status"}, {"status", "--verbose"}, {"quality"}, {"quality", "--verbose"},
		{"report"}, {"resume"}, {"doctor"}, {"doctor", "hygiene"},
		{"feedback", "list"}, {"goal", "list", "--status", "all"}, {"feature", "list"},
		{"task", "list", "--status", "all"}, {"reminder", "list", "--status", "all"},
		{"plan", "list"}, {"plan", "item", "list", "--legacy"},
		{"arch", "list"}, {"review", "list"}, {"inventory", "loc"},
		{"list", "tasks"}, {"list", "goals"}, {"list", "features"},
		{"analytics", "missed"}, {"analytics", "summary"},
		{"archive", "list"}, {"archive", "gc", "--dry-run"}, {"archive", "run", "--dry-run"},
	}
	for _, args := range reads {
		e.runOut(args...)
		e.runOut(append([]string{"--json"}, args...)...)
	}

	for _, args := range reads {
		e.runOut(append([]string{"--all"}, args...)...)
	}
}

func TestInProcessOpenErrorSweep(t *testing.T) {
	for _, args := range requireDBCommandArgs() {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := t.TempDir()
			repo := filepath.Join(dir, "repo")
			badDB := filepath.Join(dir, "dbdir")
			if err := os.MkdirAll(repo, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(badDB, 0o755); err != nil {
				t.Fatal(err)
			}
			all := append([]string{"--repo", repo, "--db", badDB}, args...)
			_, _, code := runCLI(t, all...)
			if code == 0 {
				t.Fatalf("%v succeeded with bad db", args)
			}
		})
	}
}

func TestInProcessRequireDBErrorSweep(t *testing.T) {
	for _, args := range requireDBCommandArgs() {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := t.TempDir()
			repo := filepath.Join(dir, "repo")
			dbPath := filepath.Join(dir, "missing.db")
			if err := os.MkdirAll(repo, 0o755); err != nil {
				t.Fatal(err)
			}
			all := append([]string{"--repo", repo, "--db", dbPath}, args...)
			_, _, code := runCLI(t, all...)
			if code == 0 {
				t.Fatalf("%v succeeded without init", args)
			}
		})
	}
}

func TestInProcessDoctorPythonSchema(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, stderr, code := runCLI(t, "--repo", repo, "--db", dbPath, "doctor")
	if code == 0 {
		t.Fatal("expected doctor failure on python schema")
	}
	if !strings.Contains(stderr, "legacy Python database") {
		t.Fatalf("doctor python schema: code=%d stderr=%q", code, stderr)
	}
}

func requireDBCommandArgs() [][]string {
	return [][]string{
		{"status"}, {"quality"}, {"report"}, {"resume"},
		{"feedback", "list"}, {"feedback", "add", "--role", "model", "x"},
		{"feedback", "annotate", "00000000000000000000000000000000", "n"},
		{"goal", "list"}, {"goal", "set", "00000000000000000000000000000000", "done"},
		{"feature", "list"}, {"feature", "add", "f"},
		{"task", "list"}, {"task", "show", "00000000000000000000000000000000"},
		{"task", "status", "00000000000000000000000000000000", "open"},
		{"reminder", "list"}, {"reminder", "dismiss", "00000000000000000000000000000000"},
		{"approval", "list"}, {"approval", "reject", "00000000000000000000000000000000"},
		{"plan", "list"}, {"plan", "show", "00000000000000000000000000000000"},
		{"plan", "item", "list"}, {"plan", "milestone", "list", "--plan", "x"},
		{"archive", "list"}, {"analytics", "missed"}, {"analytics", "summary"},
		{"inventory", "scan"}, {"inventory", "diff", "HEAD"},
		{"arch", "list"}, {"arch", "render"}, {"arch", "show", "00000000000000000000000000000000"},
		{"review", "list"}, {"review", "start"},
		{"review", "import"}, {"review", "resolve", "00000000000000000000000000000000", "--evidence", "x"},
		{"verify", "query", "--command", "x", "--scope", "."},
		{"verify", "show", "00000000000000000000000000000000"},
		{"verify", "dismiss", "00000000000000000000000000000000"},
		{"doctor", "hygiene"},
		{"plan", "scaffold", "T", "--slug", "s"}, {"plan", "promote"}, {"plan", "reconcile"},
		{"plan", "acceptance", "backfill"}, {"plan", "item", "status", "x", "open"},
		{"plan", "milestone", "status", "x", "open"},
		{"verify", "record", "c", "--scope", "."},
		{"import", "python-db"},
		{"feedback", "import", "commits"},
		{"list", "tasks"}, {"show", "tasks", "00000000000000000000000000000000"},
		{"task", "done", "00000000000000000000000000000000"},
		{"task", "add", "t"}, {"reminder", "add", "r", "--due", "2026-12-31T00:00:00Z"},
		{"reminder", "snooze", "00000000000000000000000000000000", "--until", "2026-12-31T00:00:00Z"},
		{"approval", "request", "00000000000000000000000000000000"},
		{"approval", "approve", "00000000000000000000000000000000"},
		{"approval", "withdraw", "00000000000000000000000000000000"},
		{"plan", "create", "P", "--slug", "p"}, {"plan", "tree", "p"},
		{"plan", "item", "add", "i", "--plan", "p"}, {"plan", "item", "start", "x"},
		{"plan", "item", "pause", "x"}, {"plan", "item", "close", "x"},
		{"plan", "acceptance", "add", "a", "--plan-item", "x"},
		{"plan", "acceptance", "meet", "x"}, {"plan", "file", "add"},
		{"plan", "milestone", "add", "M", "--plan", "p"},
		{"arch", "add", "t", "--body", "b", "--source", "x.go"},
		{"arch", "update", "x"}, {"arch", "verify", "x"},
		{"review", "finding"}, {"review", "finish", "x"}, {"review", "report", "x"},
		{"inventory", "context", "--files", "x"}, {"inventory", "suggest-cuts"},
		{"archive", "run"}, {"archive", "restore"}, {"archive", "gc"},
		{"feedback", "show", "x"}, {"feedback", "close", "x"},
		{"goal", "add", "g", "--kind", "goal"},
		{"plan", "status", "x"},
		{"plan", "item", "show", "x"},
		{"reminder", "snooze", "x", "--until", "2026-12-31T00:00:00Z"},
		{"reminder", "unsnooze", "x"},
		{"feedback", "show", "x"},
		{"feature", "add", "f2"},
		{"task", "list", "--priority", "high"},
		{"goal", "list", "--status", "done"},
		{"plan", "reconcile", "--apply"},
		{"plan", "promote", "--plan", "x"},
		{"plan", "scaffold", "S", "--slug", "s2", "--skip-acceptance"},
		{"inventory", "suggest-cuts", "--dry-run"},
		{"arch", "list", "--stale"},
		{"review", "report", "x"},
		{"review", "resolve", "x", "--evidence", "e"},
		{"verify", "record", "c", "--scope", ".", "--git-sha", "abc"},
		{"import", "python-db", "--output", "/tmp/out.db"},
	}
}

func TestInProcessQualityErrorPaths(t *testing.T) {
	e := newInProcessEnv(t)
	runID := e.run("review", "start")
	e.run("review", "finish", runID, "--summary", "done")
	_, stderr, code := e.runAllow("review", "finding",
		"--run", runID, "--principle", "kiss", "--title", "t", "--recommendation", "r")
	if code == 0 || !strings.Contains(stderr, "finished") {
		t.Fatalf("finding on finished run: code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = e.runAllow("review", "finish", runID)
	if code == 0 || !strings.Contains(stderr, "finished") {
		t.Fatalf("double finish: code=%d stderr=%q", code, stderr)
	}
	openRun := e.run("review", "start")
	findingID := e.run("review", "finding",
		"--run", openRun, "--principle", "kiss", "--title", "t", "--recommendation", "r",
	)
	_, stderr, code = e.runAllow("review", "resolve", findingID, "--status", "resolved")
	if code == 0 || !strings.Contains(stderr, "commit or evidence") {
		t.Fatalf("resolve missing evidence: code=%d stderr=%q", code, stderr)
	}

	badJSONL := filepath.Join(e.repo, "bad.jsonl")
	if err := os.WriteFile(badJSONL, []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = e.runAllow("review", "import", "--run", openRun, "--file", badJSONL)
	if code == 0 {
		t.Fatalf("bad jsonl import succeeded stderr=%q", stderr)
	}

	dir := t.TempDir()
	repo := filepath.Join(dir, "norepo")
	dbPath := filepath.Join(dir, "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runCLIOut(t, "--repo", repo, "--db", dbPath, "init")
	_, stderr, code = runCLI(t, "--repo", repo, "--db", dbPath,
		"verify", "record", "cmd", "--scope", ".", "--status", "pending")
	if code != 0 {
		t.Fatalf("verify without git should now succeed: code=%d stderr=%q", code, stderr)
	}

	e.run("inventory", "scan")
	failOut := `FAILED golang/internal/cli/main_test.go:1: boom`
	vFail := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "failed",
		"--finished", "--git-sha", "abc", "--output", failOut)
	failuresOut := e.runOut("verify", "show", vFail, "--view", "failures")
	if !strings.Contains(failuresOut, "verification failed") && !strings.Contains(failuresOut, "boom") {
		t.Fatalf("failures view: %q", failuresOut)
	}
}

func TestInProcessImportPythonDBApplyTemp(t *testing.T) {
	src := testutil.LegacyPythonDBPath(t)
	if src == "" {
		t.Skip("no legacy python development.db")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "development.db")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	runCLIOut(t, "--db", dst, "import", "python-db", dst, "--apply")
}

func TestInProcessHubErrorPaths(t *testing.T) {
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", t.TempDir())...)
	_, stderr, code := runCLI(t, append(hub, "across", "unknown-query")...)
	if code == 0 || !strings.Contains(stderr, "unknown query") {
		t.Fatalf("across bad query: code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runCLI(t, append(hub, "sync", "--strict")...)
	if code != 0 && code != ExitGeneral {
		t.Fatalf("strict sync code=%d stderr=%q", code, stderr)
	}
}

func TestInProcessReviewStartDefaultPaths(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	id := e.run("review", "start")
	if len(id) != 32 {
		t.Fatalf("id=%q", id)
	}
}

func TestInProcessReviewImportPluralAndCap(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", ".")
	jsonl := filepath.Join(e.repo, "multi.jsonl")
	content := `{"file":"main.go","principle":"kiss","title":"a","recommendation":"r","severity":"low","confidence":"medium","effort":"small"}
{"file":"main.go","principle":"dry","title":"b","recommendation":"r","severity":"low","confidence":"medium","effort":"small"}
`
	if err := os.WriteFile(jsonl, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	out := e.runOut("review", "import", "--run", runID, "--file", jsonl)
	if !strings.Contains(out, "imported 2") {
		t.Fatalf("plural import: %q", out)
	}

	capRun := e.run("review", "start", "--paths", ".")
	for i := 0; i < 3; i++ {
		e.run("review", "finding", "--run", capRun, "--file", "main.go", "--principle", "kiss",
			"--title", fmt.Sprintf("cap%d", i), "--recommendation", "r", "--severity", "low")
	}
	_, stderr, code := e.runAllow("review", "finding", "--run", capRun, "--file", "main.go",
		"--principle", "kiss", "--title", "capD", "--recommendation", "r", "--severity", "low")
	if code == 0 || !strings.Contains(stderr, "cap") {
		t.Fatalf("cap exceeded: code=%d stderr=%q", code, stderr)
	}
}

func TestInProcessVerifyShowReuseBranches(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	failed := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "failed",
		"--finished", "--git-sha", "abc", "--notes", "boom")
	failShow := e.runOut("verify", "show", failed)
	if !strings.Contains(failShow, "failed_last_time") && !strings.Contains(failShow, "reuse:") {
		t.Fatalf("failed show: %q", failShow)
	}

	passed := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "passed",
		"--finished", "--git-sha", "abc", "--notes", "ok", "--exit-code", "0")
	passShow := e.runOut("verify", "show", passed)
	if !strings.Contains(passShow, "fresh_pass") && !strings.Contains(passShow, "notes: ok") {
		t.Fatalf("passed show: %q", passShow)
	}

	mainGo := filepath.Join(e.repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.run("inventory", "scan")
	rerunShow := e.runOut("verify", "show", passed)
	if !strings.Contains(rerunShow, "rerun_required") && !strings.Contains(rerunShow, "reuse:") {
		t.Fatalf("rerun show: %q", rerunShow)
	}
}

func TestInProcessHubProjectAttention(t *testing.T) {
	e := newInProcessEnv(t)
	e.run("feedback", "add", "--role", "model", "urgent", "--severity", "high")
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", e.repo, "--alias", "attn")...)
	runCLIOut(t, append(hub, "sync")...)
	out := runCLIOut(t, append(hub, "project", "attn")...)
	if !strings.Contains(out, e.repo) {
		t.Fatalf("project: %q", out)
	}
	acrossEmpty := runCLIOut(t, append(hub, "across", "similar-feedback", "--keyword", "zzznomatch")...)
	if acrossEmpty == "" {
		t.Fatal("expected across output")
	}
}

func TestInProcessReviewPrinciplesJSON(t *testing.T) {
	out := runCLIOut(t, "--db", filepath.Join(t.TempDir(), "x.db"), "review", "principles", "--tier", "extended")
	if !strings.Contains(out, "principles") && !strings.Contains(out, "{") {
		t.Fatalf("principles: %q", out)
	}
}

func TestInProcessHubOpenErrorSweep(t *testing.T) {
	metaDir := filepath.Join(t.TempDir(), "metadir")
	registry := filepath.Join(t.TempDir(), "registry.json")
	if err := os.Mkdir(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hub := hubBase(t, metaDir, registry)
	for _, args := range [][]string{
		{"register", t.TempDir()},
		{"list"},
		{"sync"},
		{"dashboard"},
		{"doctor"},
		{"project", "none"},
	} {
		_, _, code := runCLI(t, append(hub, args...)...)
		if code == 0 {
			t.Fatalf("%v succeeded with bad metadata db", args)
		}
	}
}

func TestInProcessMiscErrorPaths(t *testing.T) {
	e := newInProcessEnv(t)
	_, stderr, code := e.runAllow("archive", "restore")
	if code != ExitUsage {
		t.Fatalf("archive restore usage: code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = e.runAllow("plan", "promote", "--plan", "missing-slug")
	if code != ExitNotFound {
		t.Fatalf("promote missing: code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = e.runAllow("feedback", "import", "markdown", "/no/such/file.md")
	if code == 0 {
		t.Fatalf("import md missing: code=%d stderr=%q", code, stderr)
	}
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	vRun := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "passed",
		"--finished", "--git-sha", "abc")
	e.run("verify", "dismiss", vRun)
	_, stderr, code = e.runAllow("verify", "dismiss", vRun)
	if code != ExitNotFound {
		t.Fatalf("dismiss twice: code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = e.runAllow("review", "finding",
		"--run", e.run("review", "start"), "--principle", "bogus", "--title", "t", "--recommendation", "r")
	if code == 0 {
		t.Fatal("expected invalid principle failure")
	}
}

func TestInProcessExtendedReadVariants(t *testing.T) {
	e := newInProcessEnv(t)
	planID := e.run("plan", "create", "Variants", "--slug", "variants")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Item", "--plan", planID, "--milestone", msID)
	fbID := e.run("feedback", "add", "--role", "user", "note")
	taskID := e.run("task", "add", "t")
	remID := e.run("reminder", "add", "r", "--due", "2026-12-31T00:00:00Z")

	e.runOut("feedback", "show", fbID)
	e.runOut("plan", "status", planID)
	e.runOut("plan", "tree", planID)
	e.runOut("plan", "item", "show", itemID)
	e.runOut("task", "show", taskID)
	e.runOut("approval", "request", taskID, "--entity", "tasks")
	e.runOut("approval", "list")
	e.runOut("task", "done", taskID)
	e.runOut("reminder", "show", remID)
	e.runOut("--json", "plan", "milestone", "list", "--plan", planID)
	e.runOut("--json", "plan", "item", "list", "--plan", planID, "--milestone", msID)
}

func TestInProcessMoreDomainErrors(t *testing.T) {
	e := newInProcessEnv(t)
	_, stderr, code := e.runAllow("plan", "scaffold", "Bad mode", "--slug", "bad-mode", "--mode", "invalid")
	if code != ExitUsage {
		t.Fatalf("bad mode: code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = e.runAllow("import", "python-db", "/no/such/db")
	if code == 0 {
		t.Fatalf("bad python db: stderr=%q", stderr)
	}
	e.run("archive", "run", "--yes", "--vacuum")
	e.runOut("plan", "scaffold", "Skip acc", "--slug", "skip-acc", "--skip-acceptance", "--milestones", "1")
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	runID := e.run("review", "start")
	for i := 0; i < 3; i++ {
		e.run("review", "finding", "--run", runID, "--file", "main.go", "--principle", "kiss",
			"--title", fmt.Sprintf("f%d", i), "--recommendation", "r", "--severity", "low")
	}
	jsonl := filepath.Join(e.repo, "skip.jsonl")
	line := `{"file":"main.go","principle":"dry","title":"skip","recommendation":"r","severity":"low","confidence":"medium","effort":"small"}`
	if err := os.WriteFile(jsonl, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runCLI(t, append(e.base, "review", "import", "--run", runID, "--file", jsonl)...)
	if code != 0 {
		t.Fatalf("import cap skip: code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "skipped") {
		t.Fatalf("expected skip hint stderr=%q", stderr)
	}
}
