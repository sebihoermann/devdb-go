package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// feedbackRowModelID is the machine contract a caller uses to confirm
// provenance: `devdb feedback list --json | jq '.[0].model_id'`. The column was
// written correctly but omitted from the read shape, so the check failed on
// rows that were in fact stamped.
type feedbackRowModelID struct {
	ID      string `json:"id"`
	Note    string `json:"note"`
	ModelID string `json:"model_id"`
}

func TestFeedbackListJSONExposesModelID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	out := runCLIOut(t, "--db", dbPath, "--json", "--model-id", "vendor/live-model-7",
		"feedback", "add", "stamped through the flag", "--role", "model", "--severity", "low")

	var created map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("add output is not JSON: %v\n%s", err, out)
	}
	if got, _ := created["model_id"].(string); got != "vendor/live-model-7" {
		t.Fatalf("add echoed model_id=%v, want vendor/live-model-7", created["model_id"])
	}

	listOut := runCLIOut(t, "--db", dbPath, "--json", "feedback", "list")
	var rows []feedbackRowModelID
	if err := json.Unmarshal([]byte(listOut), &rows); err != nil {
		t.Fatalf("list output is not a JSON array: %v\n%s", err, listOut)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%+v, want 1", rows)
	}
	if rows[0].ModelID != "vendor/live-model-7" {
		t.Fatalf("list model_id=%q, want vendor/live-model-7", rows[0].ModelID)
	}
}

// TestFeedbackShowExposesModelID keeps the single-row read in step with the
// list read; a provenance column visible in one and absent in the other is the
// same reporting bug.
func TestFeedbackShowExposesModelID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	created := runCLIOut(t, "--db", dbPath, "--json", "--model-id", "vendor/show-model",
		"feedback", "add", "shown", "--role", "user")
	var addResult map[string]any
	if err := json.Unmarshal([]byte(created), &addResult); err != nil {
		t.Fatalf("add output: %v", err)
	}
	id, _ := addResult["id"].(string)
	if id == "" {
		t.Fatalf("no id in %s", created)
	}

	var row feedbackRowModelID
	if err := json.Unmarshal([]byte(runCLIOut(t, "--db", dbPath, "--json", "feedback", "show", id)), &row); err != nil {
		t.Fatalf("show output: %v", err)
	}
	if row.ModelID != "vendor/show-model" {
		t.Fatalf("show model_id=%q, want vendor/show-model", row.ModelID)
	}
}

// TestFeedbackModelIDFlagBeatsEnv pins the precedence rule. The flag exists
// because a long-lived process cannot change its environment per turn, so it
// must win over the ambient value rather than being ignored when both are set.
func TestFeedbackModelIDFlagBeatsEnv(t *testing.T) {
	t.Setenv("DEVDB_MODEL_ID", "env/model")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	runCLIOut(t, "--db", dbPath, "--model-id", "flag/model",
		"feedback", "add", "flag wins", "--role", "model")

	var rows []feedbackRowModelID
	if err := json.Unmarshal([]byte(runCLIOut(t, "--db", dbPath, "--json", "feedback", "list")), &rows); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelID != "flag/model" {
		t.Fatalf("rows=%+v, want one row with model_id=flag/model", rows)
	}
}

// TestFeedbackModelIDFallsBackToEnv covers the ambient path used by scripts
// that export the id once.
func TestFeedbackModelIDFallsBackToEnv(t *testing.T) {
	t.Setenv("DEVDB_MODEL_ID", "env/only")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	runCLIOut(t, "--db", dbPath, "feedback", "add", "env stamps", "--role", "model")

	var rows []feedbackRowModelID
	if err := json.Unmarshal([]byte(runCLIOut(t, "--db", dbPath, "--json", "feedback", "list")), &rows); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelID != "env/only" {
		t.Fatalf("rows=%+v, want model_id=env/only", rows)
	}
}

// TestFeedbackModelIDUnsetIsVisibleAndUnknown documents the deliberate
// fallback. An unset id is recorded as "unknown" — never a fabricated model
// name — and the read path must still surface that value, because "unknown"
// visible is honest and an omitted field is not.
func TestFeedbackModelIDUnsetIsVisibleAndUnknown(t *testing.T) {
	t.Setenv("DEVDB_MODEL_ID", "")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	runCLIOut(t, "--db", dbPath, "feedback", "add", "no model", "--role", "user")

	var rows []feedbackRowModelID
	if err := json.Unmarshal([]byte(runCLIOut(t, "--db", dbPath, "--json", "feedback", "list")), &rows); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].ModelID != "unknown" {
		t.Fatalf("rows=%+v, want model_id=unknown", rows)
	}
}

// TestFeedbackModelIDHelpDocumentsFlag guards the discoverability half: a flag
// nobody can find does not fix the friction it was added for.
func TestFeedbackModelIDHelpDocumentsFlag(t *testing.T) {
	stdout, _, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
	if !strings.Contains(stdout, "--model-id") {
		t.Fatalf("help does not mention --model-id:\n%s", stdout)
	}
	_ = filepath.Join
}
