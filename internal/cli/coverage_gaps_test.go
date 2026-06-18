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
