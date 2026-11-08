package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionFlagPrintsVersionAndExitsZero covers the first acceptance
// criterion of the improvements plan: `devdb --version` must print a
// version string and exit 0. Before this, the root command had no Version
// field, so cobra rejected --version as an unknown flag.
func TestVersionFlagPrintsVersionAndExitsZero(t *testing.T) {
	out := runCLIOut(t, "--version")
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Fatal("--version printed nothing")
	}
	// cobra prints "<use> version <version>"; assert both halves so a bare
	// error string can never satisfy this.
	if !strings.HasPrefix(trimmed, "devdb version ") {
		t.Fatalf("--version output = %q, want it to start with %q", trimmed, "devdb version ")
	}
	if strings.TrimSpace(strings.TrimPrefix(trimmed, "devdb version ")) == "" {
		t.Fatalf("--version output = %q, want a non-empty version after %q", trimmed, "devdb version ")
	}
}

// TestVersionVariableIsInjectedByBuild guards the release path: Version is a
// package variable so -ldflags "-X .../internal/cli.Version=<v>" can set it.
// If someone turns it into a const, releases would silently ship "dev".
func TestVersionVariableIsInjectedByBuild(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be the empty string")
	}
	Version = "v9.9.9-test"
	defer func() { Version = "dev" }()
	if Version != "v9.9.9-test" {
		t.Fatal("Version must be assignable; a const would break -ldflags -X")
	}
}

// TestFeedbackAddRejectsUnknownRoleWithAllowedValues covers the third
// acceptance criterion. The role column carries a CHECK constraint, so an
// unknown role used to surface as a raw sqlite "constraint failed: CHECK
// constraint failed: role IN (...)" with exit code 1. It must instead be a
// usage error: exit 2, naming the allowed values.
func TestFeedbackAddRejectsUnknownRoleWithAllowedValues(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	stdout, stderr, code := runCLI(t, "--db", dbPath, "feedback", "add", "x", "--role", "agent")
	if code != 2 {
		t.Fatalf("feedback add --role agent exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	msg := stderr + stdout
	for _, want := range []string{"role", "agent", "user", "model", "codebase"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("feedback add --role agent message = %q, want it to mention %q", msg, want)
		}
	}
	// A raw sqlite constraint failure must not be what the user sees.
	if strings.Contains(msg, "CHECK constraint failed") {
		t.Fatalf("feedback add --role agent leaked a sqlite constraint error: %q", msg)
	}
}

// TestFeedbackAddRejectsUnknownSeverityWithAllowedValues is the same contract
// for --severity, which also has a CHECK constraint and also used to leak.
func TestFeedbackAddRejectsUnknownSeverityWithAllowedValues(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	stdout, stderr, code := runCLI(t, "--db", dbPath, "feedback", "add", "x", "--role", "model", "--severity", "medium")
	if code != 2 {
		t.Fatalf("feedback add --severity medium exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	msg := stderr + stdout
	if !strings.Contains(msg, "severity") || !strings.Contains(msg, "medium") {
		t.Fatalf("feedback add --severity medium message = %q, want it to name the flag and value", msg)
	}
	if !strings.Contains(msg, "critical") {
		t.Fatalf("feedback add --severity medium message = %q, want it to list the allowed values", msg)
	}
	if strings.Contains(msg, "CHECK constraint failed") {
		t.Fatalf("feedback add --severity medium leaked a sqlite constraint error: %q", msg)
	}
}

// TestFeedbackAddAcceptsEveryValidRole proves the new validation did not
// narrow the accepted set: all three documented roles must still insert.
func TestFeedbackAddAcceptsEveryValidRole(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	for _, role := range []string{"user", "model", "codebase"} {
		id := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "feedback", "add", "note for "+role, "--role", role))
		if id == "" {
			t.Fatalf("feedback add --role %s returned no id", role)
		}
	}
}

// TestEmptyListsUseHumanPhrases covers the second acceptance criterion across
// every list verb that used to print a bare [] in human mode. --json must keep
// emitting [] for all of them, so an agent's parse never depends on wording.
func TestEmptyListsUseHumanPhrases(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	cases := []struct {
		args   []string
		phrase string
	}{
		{[]string{"feedback", "list"}, "no feedback"},
		{[]string{"plan", "list"}, "no plans"},
		{[]string{"goal", "list"}, "no goals"},
		{[]string{"feature", "list"}, "no features"},
		{[]string{"task", "list"}, "no tasks"},
		{[]string{"reminder", "list"}, "no reminders"},
		{[]string{"approval", "list"}, "no pending approvals"},
	}
	for _, tc := range cases {
		name := strings.Join(tc.args, " ")
		t.Run(name, func(t *testing.T) {
			human := strings.TrimSpace(runCLIOut(t, append([]string{"--db", dbPath}, tc.args...)...))
			if human != tc.phrase {
				t.Fatalf("%s (human) = %q, want %q", name, human, tc.phrase)
			}
			jsonOut := strings.TrimSpace(runCLIOut(t, append(append([]string{"--db", dbPath}, tc.args...), "--json")...))
			if jsonOut != "[]" {
				t.Fatalf("%s --json = %q, want []", name, jsonOut)
			}
		})
	}
}
