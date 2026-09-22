package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPlanListVerbsAcceptSlug covers the dogfood gap (feedback fdb41483):
// `plan milestone list` and `plan item list` must accept --plan <slug>
// (and id prefixes) like the ADD verbs do, and must fail closed with
// exit code 3 on an unknown ref instead of silently returning [].
func TestPlanListVerbsAcceptSlug(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()
		all := append([]string{"--repo", repo, "--db", dbPath}, args...)
		stdout, stderr, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
		}
		return firstLine(stdout)
	}

	runJSON := func(args ...string) string {
		t.Helper()
		all := append([]string{"--repo", repo, "--db", dbPath, "--json"}, args...)
		stdout, stderr, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s", args, code, stderr)
		}
		return stdout
	}

	run("init")
	planID := run("plan", "create", "Slug list plan", "--slug", "slug-list")
	msID := run("plan", "milestone", "add", "--plan", "slug-list", "Milestone via slug")
	run("plan", "item", "add", "--plan", "slug-list", "--milestone", msID, "Item via slug")

	// milestone list: slug must resolve (was returning [] before the fix).
	msOut := runJSON("plan", "milestone", "list", "--plan", "slug-list")
	if !strings.Contains(msOut, msID) {
		t.Fatalf("milestone list by slug missing milestone %s: %s", msID, msOut)
	}

	// item list: slug must resolve.
	itemOut := runJSON("plan", "item", "list", "--plan", "slug-list")
	var items []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(itemOut)), &items); err != nil {
		t.Fatalf("item list json: %v (%s)", err, itemOut)
	}
	if len(items) != 1 {
		t.Fatalf("item list by slug returned %d rows, want 1: %s", len(items), itemOut)
	}
	if items[0]["plan_id"] != planID {
		t.Fatalf("item plan_id=%v want %s", items[0]["plan_id"], planID)
	}

	// UUID still works on both verbs.
	if out := runJSON("plan", "item", "list", "--plan", planID); !strings.Contains(out, planID) {
		t.Fatalf("item list by UUID: %s", out)
	}
	if out := runJSON("plan", "milestone", "list", "--plan", planID); !strings.Contains(out, msID) {
		t.Fatalf("milestone list by UUID: %s", out)
	}

	// Unknown ref must fail closed with exit 3, not print [].
	for _, verb := range [][]string{
		{"plan", "item", "list", "--plan", "no-such-plan"},
		{"plan", "milestone", "list", "--plan", "no-such-plan"},
	} {
		all := append([]string{"--repo", repo, "--db", dbPath}, verb...)
		_, stderr, code := runDevdb(t, bin, all...)
		if code != ExitNotFound {
			t.Fatalf("%v exit %d, want %d (stderr=%s)", verb, code, ExitNotFound, stderr)
		}
		if !strings.Contains(stderr, "plan not found") {
			t.Fatalf("%v stderr missing 'plan not found': %s", verb, stderr)
		}
	}
}
