package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestEmitFeedbackImportSingleRowHuman(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	md := filepath.Join(dir, "one.md")
	if err := os.WriteFile(md, []byte("## Solo entry\n- **Role**: user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLIOut(t, "--db", dbPath, "init")
	stdout, stderr, code := runCLI(t, "--db", dbPath, "feedback", "import", "markdown", md)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "imported 1 row") {
		t.Fatalf("stderr=%q", stderr)
	}
	if len(strings.TrimSpace(stdout)) != 32 {
		t.Fatalf("expected single id stdout, got %q", stdout)
	}
}

func TestCmdHelpAndImportBranches(t *testing.T) {
	helpOut := runCLIOut(t, "help")
	if !strings.Contains(helpOut, "devdb") {
		t.Fatalf("help=%q", helpOut)
	}
	helpSub := runCLIOut(t, "import", "python-db", "--help")
	if !strings.Contains(helpSub, "legacy Python database") {
		t.Fatalf("import help=%q", helpSub)
	}
	runCLIOut(t, "help", "status")

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")
	legacyPath := filepath.Join(dir, "legacy.db")
	legacyDB, err := storage.Open(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python')`); err != nil {
		t.Fatal(err)
	}
	legacyDB.Close()
	inspectOut := runCLIOut(t, "--db", dbPath, "import", "python-db", legacyPath)
	if !strings.Contains(inspectOut, "tables") && !strings.Contains(inspectOut, "version") {
		t.Fatalf("inspect=%q", inspectOut)
	}
}

func TestHubDoctorProjectAcrossHuman(t *testing.T) {
	e := newInProcessEnv(t)
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(e.t, metaDB, registry)
	alias := firstLine(runCLIOut(e.t, append(hub, "register", e.repo, "--alias", "hubhuman")...))
	runCLIOut(e.t, append(hub, "sync")...)

	doctor := runCLIOut(e.t, append(hub, "doctor", "--project", alias)...)
	if !strings.Contains(doctor, "freshness=") {
		t.Fatalf("doctor=%q", doctor)
	}
	doctorJSON := runCLIOut(e.t, append(hub, "--json", "doctor")...)
	if !strings.Contains(doctorJSON, "freshness_status") {
		t.Fatalf("doctor json=%q", doctorJSON)
	}

	proj := runCLIOut(e.t, append(hub, "project", alias)...)
	if !strings.Contains(proj, "status=") {
		t.Fatalf("project=%q", proj)
	}

	across := runCLIOut(e.t, append(hub, "across", "open-debt")...)
	if across == "" {
		t.Fatal("across empty")
	}
	acrossJSON := runCLIOut(e.t, append(hub, "--json", "across", "similar-feedback", "--keyword", "nothing")...)
	if !strings.Contains(acrossJSON, "project") && acrossJSON != "null" && acrossJSON != "[]" {
		t.Fatalf("across json=%q", acrossJSON)
	}
}

func TestApprovalLogReminderShowInventoryContext(t *testing.T) {
	e := newInProcessEnv(t)
	taskID := e.run("task", "add", "ship it")
	e.run("approval", "request", taskID, "--note", "ready")
	e.runOut("approval", "log")
	e.runOut("--json", "approval", "log")

	remID := e.run("reminder", "add", "check tests", "--due", "2099-01-01T00:00:00Z")
	e.runOut("reminder", "show", remID)
	e.runOut("--json", "reminder", "show", remID)

	e.runOut("inventory", "context", "--files", "main.go")
	e.runOut("--json", "inventory", "context", "--files", "main.go")
}

func TestHubDoctorEmptyRegistryHuman(t *testing.T) {
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "empty-registry")
	hub := hubBase(t, metaDB, registry)
	out := runCLIOut(t, append(hub, "doctor")...)
	if !strings.Contains(out, "no registered projects") {
		t.Fatalf("doctor=%q", out)
	}
}

func TestHubAcrossNoRowsHuman(t *testing.T) {
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry")
	if err := os.WriteFile(registry, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	hub := hubBase(t, metaDB, registry)
	out := runCLIOut(t, append(hub, "across", "open-debt")...)
	if out != "no rows" {
		t.Fatalf("across=%q", out)
	}
}

func TestImportPythonDBOutputPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")
	legacyPath := filepath.Join(dir, "legacy.db")
	legacyDB, err := storage.Open(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python')`); err != nil {
		t.Fatal(err)
	}
	legacyDB.Close()
	outDB := filepath.Join(dir, "out.db")
	runCLIOut(t, "--db", dbPath, "import", "python-db", legacyPath, "--output", outDB, "--replace")
	if _, err := os.Stat(outDB); err != nil {
		t.Fatalf("output db: %v", err)
	}
}

func TestHubListAndDashboardHuman(t *testing.T) {
	e := newInProcessEnv(t)
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", e.repo, "--alias", "listme")...)
	runCLIOut(t, append(hub, "sync")...)
	list := runCLIOut(t, append(hub, "list")...)
	if !strings.Contains(list, "listme") {
		t.Fatalf("list=%q", list)
	}
	dash := runCLIOut(t, append(hub, "dashboard", "--view", "delivery")...)
	if !strings.Contains(dash, "listme") {
		t.Fatalf("dashboard=%q", dash)
	}
}

func TestInventoryScanAndVerifyShow(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.runOut("inventory", "scan", "--paths", ".")
	e.runOut("inventory", "loc", "--paths", "main.go")
	_, _, code := e.runAllow("inventory", "context", "--files", "main.go", "--strict")
	if code != 0 && code != ExitUsage {
		t.Fatalf("strict context exit=%d", code)
	}
	e.runOut("verify", "record", "go test ./...", "--scope", ".", "--status", "passed", "--finished")
	show := e.runOut("verify", "query", "--command", "go test ./...", "--scope", ".")
	if show == "" {
		t.Fatal("verify query empty")
	}
}

func TestArchVerifyAndInitExisting(t *testing.T) {
	e := newInProcessEnv(t)
	e.run("inventory", "scan", "--paths", ".")
	noteID := e.run("arch", "add", "topic", "--body", "note body", "--source", "main.go")
	e.runOut("arch", "verify", noteID)
	e.runOut("arch", "verify", "--all")
	e.runOut("--json", "arch", "verify", "all")

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "existing.db")
	runCLIOut(t, "--db", dbPath, "init")
	runCLIOut(t, "--db", dbPath, "init")
}

// TestPlanAddAcceptsSlugOrID is a regression for feedback dd1bf6f5:
// 'plan milestone add' and 'plan item add' must accept --plan as a slug
// or as a uuid (or id prefix) without an FK constraint failure.
func TestPlanAddAcceptsSlugOrID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")
	planID := runCLIOut(t, "--db", dbPath, "plan", "create", "Slug Plan", "--slug", "slug-plan")
	planID = strings.TrimSpace(planID)
	if len(planID) != 32 {
		t.Fatalf("plan id=%q", planID)
	}

	// Milestone add with slug must succeed.
	runCLIOut(t, "--db", dbPath, "plan", "milestone", "add", "M-slug", "--plan", "slug-plan")
	// Milestone add with uuid must still work (no regression).
	runCLIOut(t, "--db", dbPath, "plan", "milestone", "add", "M-uuid", "--plan", planID)

	// Item add with slug must succeed.
	runCLIOut(t, "--db", dbPath, "plan", "item", "add", "Item-slug", "--plan", "slug-plan")
	// Item add with uuid must still work (no regression).
	runCLIOut(t, "--db", dbPath, "plan", "item", "add", "Item-uuid", "--plan", planID)
}

// TestHelpDispatchesToSubcommand is a regression for feedback d97f61ce:
// 'devdb help <subcommand>' must dispatch to that subcommand's help text,
// not print global help unconditionally.
func TestHelpDispatchesToSubcommand(t *testing.T) {
	global := runCLIOut(t, "help")
	if !strings.Contains(global, "Available Commands") {
		t.Fatalf("global help missing command list: %q", global)
	}

	planHelp := runCLIOut(t, "help", "plan")
	if !strings.Contains(planHelp, "devdb plan [command]") {
		t.Fatalf("help plan should dispatch to plan's help: %q", planHelp)
	}

	hubAuditHelp := runCLIOut(t, "help", "hub", "audit")
	if !strings.Contains(hubAuditHelp, "devdb hub audit [flags]") {
		t.Fatalf("help hub audit should dispatch to hub audit's help: %q", hubAuditHelp)
	}

	bogusStdout, bogusStderr, bogusCode := runCLI(t, "help", "bogus")
	if bogusCode != 0 {
		t.Fatalf("help bogus must not crash; exit=%d stderr=%s", bogusCode, bogusStderr)
	}
	if !strings.Contains(bogusStderr, "unknown command") {
		t.Fatalf("help bogus should report unknown command on stderr: %q", bogusStderr)
	}
	if !strings.Contains(bogusStdout, "Available Commands") {
		t.Fatalf("help bogus should fall back to global help: %q", bogusStdout)
	}
}
