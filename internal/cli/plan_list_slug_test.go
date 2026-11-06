package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestPlanListAcceptsSlugOrID is a regression for the dogfood finding
// fdb41483f94e55f172c1ab4954a08f41 (category dogfood, severity high):
// 'plan milestone list' and 'plan item list' must accept --plan as a slug
// or as a uuid (or id prefix) — the add verbs were fixed in 471ba2b, the
// list siblings were missed and silently returned an empty result.
// An unknown plan ref must fail closed instead of returning an empty list.
func TestPlanListAcceptsSlugOrID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")
	planID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "create", "Slug Plan", "--slug", "slug-plan"))
	if len(planID) != 32 {
		t.Fatalf("plan id=%q", planID)
	}

	// Seed one milestone and one item against the plan (by slug).
	runCLIOut(t, "--db", dbPath, "plan", "milestone", "add", "M-slug", "--plan", "slug-plan")
	runCLIOut(t, "--db", dbPath, "plan", "item", "add", "Item-slug", "--plan", "slug-plan")

	// Milestone list by slug must return the seeded milestone.
	msSlug := decodeRows(t, runCLIOut(t, "--db", dbPath, "plan", "milestone", "list", "--plan", "slug-plan"))
	if len(msSlug) != 1 || msSlug[0]["title"] != "M-slug" {
		t.Fatalf("milestone list by slug: got %v", msSlug)
	}

	// Item list by slug must return the seeded item.
	itemsSlug := decodeRows(t, runCLIOut(t, "--db", dbPath, "plan", "item", "list", "--plan", "slug-plan"))
	if len(itemsSlug) != 1 || itemsSlug[0]["title"] != "Item-slug" {
		t.Fatalf("item list by slug: got %v", itemsSlug)
	}

	// Milestone list by uuid must keep working (no regression).
	msUUID := decodeRows(t, runCLIOut(t, "--db", dbPath, "plan", "milestone", "list", "--plan", planID))
	if len(msUUID) != 1 || msUUID[0]["title"] != "M-slug" {
		t.Fatalf("milestone list by uuid: got %v", msUUID)
	}

	// Item list by uuid must keep working (no regression).
	itemsUUID := decodeRows(t, runCLIOut(t, "--db", dbPath, "plan", "item", "list", "--plan", planID))
	if len(itemsUUID) != 1 || itemsUUID[0]["title"] != "Item-slug" {
		t.Fatalf("item list by uuid: got %v", itemsUUID)
	}

	// An unknown plan ref must fail closed — not silently return an empty list.
	for _, verb := range [][]string{
		{"plan", "milestone", "list", "--plan", "no-such-plan"},
		{"plan", "item", "list", "--plan", "no-such-plan"},
	} {
		_, stderr, code := runCLI(t, append([]string{"--db", dbPath}, verb...)...)
		if code == 0 {
			t.Fatalf("%v: expected non-zero exit for unknown plan, got 0", verb)
		}
		if !strings.Contains(strings.ToLower(stderr), "plan not found") {
			t.Fatalf("%v: expected 'plan not found' in stderr, got %q", verb, stderr)
		}
	}
}

// decodeRows parses a JSON array printed by a list verb.
func decodeRows(t *testing.T, out string) []map[string]any {
	t.Helper()
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return rows
}
