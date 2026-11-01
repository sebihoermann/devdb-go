package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPlanItemPhasePersists covers feedback 0bfc81be (skill pitfall 8):
// `plan item add --phase` registered the flag but AddItem never wrote the
// column, so the value was silently dropped — worse than a loud error,
// because agents built phase-numbered titles as a workaround. The phase
// (and step) must now survive add -> show and render in the plan tree.
func TestPlanItemPhasePersists(t *testing.T) {
	e := newInProcessEnv(t)

	planID := e.run("plan", "create", "Phase Plan", "--slug", "phase-plan")
	msID := e.run("plan", "milestone", "add", "Phase milestone", "--plan", planID)

	// phase + step round-trip through add -> show
	withStep := e.run("plan", "item", "add", "Item with step",
		"--plan", planID, "--milestone", msID, "--phase", "M2", "--step", "3")
	var show struct {
		Item struct {
			Title string `json:"title"`
			Phase string `json:"phase"`
			Step  string `json:"step"`
		} `json:"item"`
	}
	out := e.runOut("--json", "plan", "item", "show", withStep)
	if err := json.Unmarshal([]byte(extractJSON(out)), &show); err != nil {
		t.Fatalf("item show json: %v (%s)", err, out)
	}
	if show.Item.Phase != "M2" || show.Item.Step != "3" {
		t.Fatalf("phase/step not persisted: phase=%q step=%q (want M2/3)", show.Item.Phase, show.Item.Step)
	}

	// phase alone persists with an empty step
	phaseOnly := e.run("plan", "item", "add", "Item phase only",
		"--plan", planID, "--milestone", msID, "--phase", "M4")
	show = struct {
		Item struct {
			Title string `json:"title"`
			Phase string `json:"phase"`
			Step  string `json:"step"`
		} `json:"item"`
	}{} // reset: step is omitempty, a stale struct would keep the previous value
	out = e.runOut("--json", "plan", "item", "show", phaseOnly)
	if err := json.Unmarshal([]byte(extractJSON(out)), &show); err != nil {
		t.Fatal(err)
	}
	if show.Item.Phase != "M4" {
		t.Fatalf("phase not persisted: %q", show.Item.Phase)
	}
	if show.Item.Step != "" {
		t.Fatalf("step should be empty when not given, got %q", show.Item.Step)
	}

	// plan tree renders the labels: "[M2.3] ..." and "[M4] ...", plain title when no phase
	plain := e.run("plan", "item", "add", "Plain item", "--plan", planID, "--milestone", msID)
	tree := e.runOut("plan", "tree", planID)
	for _, want := range []string{"[M2.3] Item with step", "[M4] Item phase only", "Plain item"} {
		if !strings.Contains(tree, want) {
			t.Fatalf("tree missing %q:\n%s", want, tree)
		}
	}
	if strings.Contains(tree, "[.] Plain item") || strings.Contains(tree, "[] Plain item") {
		t.Fatalf("phaseless item rendered an empty label bracket:\n%s", tree)
	}

	// Omitting --phase leaves the column NULL (no regression for the common path).
	show = struct {
		Item struct {
			Title string `json:"title"`
			Phase string `json:"phase"`
			Step  string `json:"step"`
		} `json:"item"`
	}{}
	out = e.runOut("--json", "plan", "item", "show", plain)
	if err := json.Unmarshal([]byte(extractJSON(out)), &show); err != nil {
		t.Fatal(err)
	}
	if show.Item.Phase != "" || show.Item.Step != "" {
		t.Fatalf("phaseless item should have empty phase/step, got %q/%q", show.Item.Phase, show.Item.Step)
	}
}

// extractJSON pulls the JSON object out of `--json` output that may be
// preceded by hint lines or followed by the bare-id stdout line.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}
