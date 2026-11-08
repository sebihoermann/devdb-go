package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// seedFilterFixture writes a spread of feedback rows covering the columns the
// new filters expose: severity, category, role and context. Names are chosen so
// a filter that silently matches nothing (or everything) is obvious.
func seedFilterFixture(t *testing.T, dbPath string) {
	t.Helper()
	runCLIOut(t, "--db", dbPath, "init")
	rows := []struct {
		note, role, category, severity, context string
	}{
		{"high dx one", "model", "dx", "high", "internal/cli/commands.go"},
		{"high dx two", "model", "dx", "high", "internal/cli/feedback.go"},
		{"high dx three", "user", "dx", "high", "internal/cli/limits.go"},
		{"low dx four", "model", "dx", "low", "internal/cli/commands.go"},
		{"high data five", "model", "data", "high", "internal/domain/planning"},
		{"med ops six", "codebase", "ops", "med", "scripts/deploy.sh"},
	}
	for _, r := range rows {
		out := runCLIOut(t, "--db", dbPath, "feedback", "add", r.note,
			"--role", r.role, "--category", r.category, "--severity", r.severity, "--context", r.context)
		if strings.TrimSpace(out) == "" {
			t.Fatalf("feedback add %q returned no id", r.note)
		}
	}
}

// TestFeedbackListFiltersMatchEveryRow covers acceptance 1 and 2: a combined
// --severity/--category filter with --limit must return at most the limit and
// every row must satisfy both predicates, in both output modes.
func TestFeedbackListFiltersMatchEveryRow(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	seedFilterFixture(t, dbPath)

	type row struct {
		Note     string `json:"note"`
		Category string `json:"category"`
		Severity string `json:"severity"`
		Role     string `json:"role"`
		Context  string `json:"context"`
	}

	parse := func(out string) []row {
		t.Helper()
		var rows []row
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("output is not a JSON array: %v\n%s", err, out)
		}
		return rows
	}

	t.Run("json", func(t *testing.T) {
		out := runCLIOut(t, "--db", dbPath, "feedback", "list",
			"--severity", "high", "--category", "dx", "--limit", "5", "--status", "all", "--json")
		rows := parse(out)
		if len(rows) > 5 {
			t.Fatalf("got %d rows, want <= 5", len(rows))
		}
		if len(rows) != 3 {
			t.Fatalf("got %d rows, want 3 high+dx rows (notes: %+v)", len(rows), rows)
		}
		for _, r := range rows {
			if r.Severity != "high" || r.Category != "dx" {
				t.Fatalf("row does not match both filters: %+v", r)
			}
		}
	})

	t.Run("human", func(t *testing.T) {
		htmlOut := runCLIOut(t, "--db", dbPath, "feedback", "list",
			"--severity", "high", "--category", "dx", "--limit", "5", "--status", "all")
		// Human mode for this verb still prints the row array (compact lines
		// for this verb are JSON), so the same parse applies; the contract
		// under test is that the row set is identical to the --json row set.
		humanRows := parse(htmlOut)
		if len(humanRows) != 3 {
			t.Fatalf("human mode returned %d rows, want 3", len(humanRows))
		}
		for _, r := range humanRows {
			if r.Severity != "high" || r.Category != "dx" {
				t.Fatalf("human row does not match both filters: %+v", r)
			}
		}
	})
}

// TestFeedbackListFilterLimitCaps verifies --limit actually caps the result
// rather than being swallowed by the default.
func TestFeedbackListFilterLimitCaps(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	seedFilterFixture(t, dbPath)

	out := runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "all", "--limit", "2", "--json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("--limit 2 returned %d rows, want 2", len(rows))
	}

	// --all must beat --limit, since that is what the global flag promises.
	outAll := runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "all", "--limit", "2", "--all", "--json")
	var allRows []map[string]any
	if err := json.Unmarshal([]byte(outAll), &allRows); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(allRows) != 6 {
		t.Fatalf("--all returned %d rows, want all 6", len(allRows))
	}
}

// TestFeedbackListContextIsSubstring covers the documented asymmetry: --context
// is a substring match, so one path fragment finds every row mentioning it.
func TestFeedbackListContextIsSubstring(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	seedFilterFixture(t, dbPath)

	out := runCLIOut(t, "--db", dbPath, "feedback", "list",
		"--status", "all", "--context", "cli/commands.go", "--json")
	var rows []struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("context substring matched %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if !strings.Contains(r.Context, "cli/commands.go") {
			t.Fatalf("context row does not contain the fragment: %q", r.Context)
		}
	}
}

// TestFeedbackListRoleFilter covers --role, which is a closed vocabulary.
func TestFeedbackListRoleFilter(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	seedFilterFixture(t, dbPath)

	out := runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "all", "--role", "user", "--json")
	var rows []struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("--role user returned %d rows, want 1", len(rows))
	}
	if rows[0].Role != "user" {
		t.Fatalf("row role = %q, want user", rows[0].Role)
	}
}

// TestFeedbackListFiltersCombineWithStatus proves the filters AND together with
// --status rather than replacing it.
func TestFeedbackListFiltersCombineWithStatus(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	seedFilterFixture(t, dbPath)

	// Close one of the two high+dx model rows; the default status=open must
	// then return one row while --status all returns both.
	out := runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "all", "--severity", "high", "--category", "dx", "--role", "model", "--json")
	var rows []struct {
		ID   string `json:"id"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("high+dx+model returned %d rows, want 2 (%+v)", len(rows), rows)
	}
	runCLIOut(t, "--db", dbPath, "feedback", "close", rows[0].ID, "--proposed-fix", "done")

	openOut := runCLIOut(t, "--db", dbPath, "feedback", "list", "--severity", "high", "--category", "dx", "--role", "model", "--json")
	var openRows []struct {
		Note string `json:"note"`
	}
	if err := json.Unmarshal([]byte(openOut), &openRows); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(openRows) != 1 {
		t.Fatalf("status=open + filters returned %d rows, want 1", len(openRows))
	}
}

// TestFeedbackListRejectsBadFilterEnums covers acceptance 3: a typo in a
// validated enum is a usage error naming the allowed values, never an empty
// result that reads like "no such feedback".
func TestFeedbackListRejectsBadFilterEnums(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	cases := []struct {
		flag    string
		value   string
		allowed []string
	}{
		{"severity", "nope", []string{"info", "low", "med", "high", "critical"}},
		{"role", "agent", []string{"user", "model", "codebase"}},
		{"status", "done", []string{"open", "closed", "all"}},
	}
	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, "--db", dbPath, "feedback", "list", "--"+tc.flag, tc.value)
			if code != 2 {
				t.Fatalf("--%s %s exit code = %d, want 2 (stdout=%q stderr=%q)", tc.flag, tc.value, code, stdout, stderr)
			}
			msg := stderr + stdout
			if !strings.Contains(msg, tc.flag) || !strings.Contains(msg, tc.value) {
				t.Fatalf("message = %q, want it to name --%s and %q", msg, tc.flag, tc.value)
			}
			for _, want := range tc.allowed {
				if !strings.Contains(msg, want) {
					t.Fatalf("message = %q, want it to list %q", msg, want)
				}
			}
		})
	}
}

// TestFeedbackListFiltersEmptyResultPhrase keeps the 2.3 contract intact: a
// filter that matches nothing still prints the human phrase in human mode.
func TestFeedbackListFiltersEmptyResultPhrase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	seedFilterFixture(t, dbPath)

	human := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "all", "--category", "no-such-category"))
	if human != "no feedback" {
		t.Fatalf("empty filtered list (human) = %q, want %q", human, "no feedback")
	}
	jsonOut := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "all", "--category", "no-such-category", "--json"))
	if jsonOut != "[]" {
		t.Fatalf("empty filtered list --json = %q, want []", jsonOut)
	}
}
