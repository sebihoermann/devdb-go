package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
)

// makeFakeLedger creates <root>/<name>/.devdb/development.db (an empty file is
// enough for discovery — FindUnregistered only looks at paths).
func makeFakeLedger(t *testing.T, root, name string) string {
	t.Helper()
	repo := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(repo, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(repo, ".devdb", "development.db")
	if err := os.WriteFile(dbPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// TestHubDoctorScanUnregistered covers feedback (hub blind spot): 15 of 19
// .devdb repos were invisible to hub audit/dashboard because nothing flagged
// unregistered ledgers. `hub doctor --scan DIR` must list .devdb repos under
// DIR that the registry does not know, suggest the alias AutoRegister would
// use, and stay silent (clean line) when everything is registered.
func TestHubDoctorScanUnregistered(t *testing.T) {
	metaDB := filepath.Join(t.TempDir(), "metadata.db")
	registry := filepath.Join(t.TempDir(), "registry")
	hubArgs := hubBase(t, metaDB, registry)

	scope := t.TempDir()
	known := makeFakeLedger(t, scope, "known-repo")
	hidden := makeFakeLedger(t, scope, "hidden-repo")

	// A .devdb nested under .git/node_modules must NOT be discovered.
	vcsNoise := filepath.Join(scope, "vendor-repo", "node_modules", "pkg", ".devdb")
	if err := os.MkdirAll(vcsNoise, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vcsNoise, "development.db"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	runCLIOut(t, append(hubArgs, "register", known, "--alias", "known-repo")...)

	// Domain function directly.
	found, err := hub.FindUnregistered([]string{scope}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Root != hidden || found[0].SuggestedAlias != "hidden-repo" {
		t.Fatalf("FindUnregistered = %+v, want exactly hidden-repo", found)
	}

	// CLI human output names the repo and the fix command.
	out := runCLIOut(t, append(hubArgs, "doctor", "--scan", scope)...)
	if !strings.Contains(out, "1 unregistered .devdb repo") {
		t.Fatalf("doctor scan line missing: %q", out)
	}
	if !strings.Contains(out, "hidden-repo") {
		t.Fatalf("doctor should name the hidden repo: %q", out)
	}
	if !strings.Contains(out, "hub register --auto") {
		t.Fatalf("doctor should suggest the fix command: %q", out)
	}
	if strings.Contains(out, "known-repo (alias would be") {
		t.Fatalf("registered repo reported as unregistered: %q", out)
	}
	if strings.Contains(out, "vendor-repo") {
		t.Fatalf("node_modules noise was scanned: %q", out)
	}

	// CLI JSON shape: {projects, unregistered}.
	var js struct {
		Projects     []map[string]any     `json:"projects"`
		Unregistered []hub.UnregisteredRepo `json:"unregistered"`
	}
	jsonOut := runCLIOut(t, append(hubArgs, "--json", "doctor", "--scan", scope)...)
	start, end := strings.Index(jsonOut, "{"), strings.LastIndex(jsonOut, "}")
	if err := json.Unmarshal([]byte(jsonOut[start:end+1]), &js); err != nil {
		t.Fatalf("doctor --scan json: %v (%s)", err, jsonOut)
	}
	if len(js.Unregistered) != 1 || js.Unregistered[0].Root != hidden {
		t.Fatalf("json unregistered = %+v", js.Unregistered)
	}

	// After auto-registering, the scan is clean.
	runCLIOut(t, append(hubArgs, "register", "--auto", "--scope", scope)...)
	out = runCLIOut(t, append(hubArgs, "doctor", "--scan", scope)...)
	if !strings.Contains(out, "no unregistered .devdb repos found") {
		t.Fatalf("scan should be clean after auto-register: %q", out)
	}

	// Without --scan the legacy shape is unchanged (bare array, no scan line).
	legacyOut := runCLIOut(t, append(hubArgs, "doctor")...)
	if strings.Contains(legacyOut, "unregistered") || strings.Contains(legacyOut, "scan:") {
		t.Fatalf("plain doctor gained scan output: %q", legacyOut)
	}
}
