package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFeedbackSeverityVerb covers feedback f9528aec: re-seeding a row's
// severity previously required raw SQL (UPDATE feedback SET severity=...),
// which the skill's severity contract told agents to hand-roll. The verb
// must set the level, stamp an auditable old->new line into context, and
// reject invalid levels.
func TestFeedbackSeverityVerb(t *testing.T) {
	e := newInProcessEnv(t)

	id := e.run("feedback", "add", "watchdog ALERT snapshot row", "--role", "model", "--severity", "high")

	// Valid re-seed with reason.
	var meta struct {
		ID  string `json:"id"`
		Old string `json:"old"`
		New string `json:"new"`
	}
	out := e.runOut("--json", "feedback", "severity", id, "info", "--reason", "state record, not debt")
	// stdout contract: bare id on stdout; the JSON envelope goes through --json
	if err := json.Unmarshal([]byte(out), &meta); err != nil {
		// non-JSON build prints the bare id; fall back to direct row checks below
		meta.ID = strings.TrimSpace(out)
	}
	if meta.Old != "" && meta.Old != "high" {
		t.Fatalf("old severity = %q, want high", meta.Old)
	}

	var row struct {
		Severity string `json:"severity"`
		Context  string `json:"context"`
	}
	showOut := e.runOut("--json", "feedback", "show", id)
	if err := json.Unmarshal([]byte(showOut), &row); err != nil {
		t.Fatalf("feedback show json: %v (%s)", err, showOut)
	}
	if row.Severity != "info" {
		t.Fatalf("severity after verb = %q, want info", row.Severity)
	}
	if !strings.Contains(row.Context, "high -> info") || !strings.Contains(row.Context, "state record, not debt") {
		t.Fatalf("context missing audit stamp: %q", row.Context)
	}

	// medium alias normalizes to med.
	e.run("feedback", "severity", id, "medium")
	showOut = e.runOut("--json", "feedback", "show", id)
	if err := json.Unmarshal([]byte(showOut), &row); err != nil {
		t.Fatal(err)
	}
	if row.Severity != "med" {
		t.Fatalf("medium alias = %q, want med", row.Severity)
	}

	// Invalid level exits non-zero with an actionable message.
	_, stderr, code := runCLI(t, append(e.base, "feedback", "severity", id, "bogus")...)
	if code == 0 {
		t.Fatal("invalid severity should exit non-zero")
	}
	if !strings.Contains(stderr, "bogus") && !strings.Contains(stderr, "severity") {
		t.Fatalf("error should name the bad value: %s", stderr)
	}

	// Unknown id fails loudly, not silently.
	if _, _, code := runCLI(t, append(e.base, "feedback", "severity", "deadbeefdeadbeef", "low")...); code == 0 {
		t.Fatal("unknown id should exit non-zero")
	}
}
