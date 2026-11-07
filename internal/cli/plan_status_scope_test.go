package cli

import (
	"encoding/json"
	"testing"
)

// TestPlanStatusScopedToPlan covers feedback b2abf3a4 (pitfall 14a/18b):
// `plan status <ref>` must count only the queried plan's items. Before the
// fix it ignored the argument entirely and returned the fleet aggregate,
// so sibling plans in one DB produced identical counts and verify scripts
// reported phantom misses.
func TestPlanStatusScopedToPlan(t *testing.T) {
	e := newInProcessEnv(t)

	planA := e.run("plan", "create", "Plan A", "--slug", "plan-a")
	planB := e.run("plan", "create", "Plan B", "--slug", "plan-b")
	msA := e.run("plan", "milestone", "add", "MA", "--plan", planA)
	msB := e.run("plan", "milestone", "add", "MB", "--plan", planB)

	// A: 2 items, one closed. B: 1 item, left planned.
	itemA1 := e.run("plan", "item", "add", "A1", "--plan", planA, "--milestone", msA)
	e.run("plan", "item", "add", "A2", "--plan", planA, "--milestone", msA)
	e.run("plan", "item", "close", itemA1, "--evidence", "test")
	e.run("plan", "item", "add", "B1", "--plan", planB, "--milestone", msB)

	statuses := func(refs ...string) map[string]int {
		args := append([]string{"--json", "plan", "status"}, refs...)
		var out map[string]int
		if err := json.Unmarshal([]byte(e.runOut(args...)), &out); err != nil {
			t.Fatalf("plan status %v: %v", refs, err)
		}
		return out
	}

	a := statuses("plan-a")
	if a["done"] != 1 || a["planned"] != 1 || a["in_progress"] != 0 {
		t.Fatalf("plan A scoped counts wrong: %v (want done=1 planned=1)", a)
	}
	b := statuses(planB) // UUID ref resolves too
	if b["done"] != 0 || b["planned"] != 1 || b["in_progress"] != 0 {
		t.Fatalf("plan B scoped counts wrong: %v (want planned=1)", b)
	}
	if a["planned"] == b["planned"] && a["done"] == b["done"] {
		t.Fatalf("scoped statuses identical — plan status is still aggregating: A=%v B=%v", a, b)
	}

	// No arg and --global keep the legacy fleet aggregate (A+B = done 1, planned 2).
	g := statuses()
	if g["done"] != 1 || g["planned"] != 2 {
		t.Fatalf("global aggregate wrong: %v (want done=1 planned=2)", g)
	}
	var gf map[string]int
	if err := json.Unmarshal([]byte(e.runOut("--json", "plan", "status", "plan-a", "--global")), &gf); err != nil {
		t.Fatalf("plan status --global: %v", err)
	}
	if gf["planned"] != 2 {
		t.Fatalf("--global should ignore the plan ref: %v", gf)
	}

	// Unknown ref must fail loudly, not silently report zeros.
	if _, _, code := runCLI(t, append(e.base, "plan", "status", "no-such-plan")...); code == 0 {
		t.Fatal("plan status with unknown ref should exit non-zero")
	}
}
