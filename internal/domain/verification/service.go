package verification

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

var (
	broadScopeRoles = map[string]bool{
		"fixture": true, "config": true, "dependency": true, "tooling": true,
	}
	pytestSummaryRE = regexp.MustCompile(`^(FAILED|ERROR|XPASS|XFAIL)\s+(.+?)(?:\s+-\s+(.*))?$`)
)

// Run is a verification run row.
type Run struct {
	ID              string `json:"id"`
	Command         string `json:"command"`
	Scope           string `json:"scope"`
	Status          string `json:"status"`
	GitSHA          string `json:"git_sha"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	Output          string `json:"output,omitempty"`
	Notes           string `json:"notes,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
	DismissedAt     string `json:"dismissed_at,omitempty"`
	DismissedReason string `json:"dismissed_reason,omitempty"`
	CreatedAt       string `json:"created_at"`
	ModelID         string `json:"model_id"`
}

// Input is one tracked input file for a verification run.
type Input struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	FilePath    string `json:"file_path"`
	Role        string `json:"role"`
	ContentHash string `json:"content_hash"`
	CreatedAt   string `json:"created_at"`
	ModelID     string `json:"model_id"`
}

// Failure is a structured failure summary.
type Failure struct {
	ID             string `json:"id"`
	RunID          string `json:"run_id"`
	TestID         string `json:"test_id,omitempty"`
	FilePath       string `json:"file_path,omitempty"`
	Headline       string `json:"headline"`
	MessageExcerpt string `json:"message_excerpt,omitempty"`
	FailureKind    string `json:"failure_kind,omitempty"`
	CreatedAt      string `json:"created_at"`
	ModelID        string `json:"model_id"`
}

// ReuseDecision is the outcome of a reuse query.
type ReuseDecision struct {
	Decision     string           `json:"decision"`
	Reason       string           `json:"reason"`
	RunID        string           `json:"run_id,omitempty"`
	RunStatus    string           `json:"run_status,omitempty"`
	ChangedFiles []ChangedFile    `json:"changed_files"`
	FailedTests  []map[string]any `json:"failed_tests"`
}

// ChangedFile is a file change event or synthetic freshness row.
type ChangedFile struct {
	Path       string `json:"path,omitempty"`
	ChangeKind string `json:"change_kind,omitempty"`
	OldHash    string `json:"old_hash,omitempty"`
	NewHash    string `json:"new_hash,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// QueryResult is the CLI-facing reuse query payload.
type QueryResult struct {
	Command         string           `json:"command"`
	Scope           string           `json:"scope"`
	Status          string           `json:"status"`
	Fresh           bool             `json:"fresh"`
	Reason          string           `json:"reason"`
	RunID           string           `json:"run_id,omitempty"`
	Decision        string           `json:"decision"`
	ChangedFiles    []ChangedFile    `json:"changed_files"`
	FailedTests     []map[string]any `json:"failed_tests"`
	AutoInputs      bool             `json:"auto_inputs,omitempty"`
	InputsCollected int              `json:"inputs_collected"`
}

// ShowSummary is a compact run detail with reuse verdict.
type ShowSummary struct {
	Run        Run           `json:"run"`
	Inputs     []Input       `json:"inputs"`
	InputCount int           `json:"input_count"`
	Failures   []Failure     `json:"failures"`
	Reuse      ReuseDecision `json:"reuse"`
}

// RecordRun creates a new verification run.
func RecordRun(db *sql.DB, command, scope, gitSHA, status string, exitCode *int, output, notes, modelID string) (string, error) {
	runID, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO verification_runs
			(id, command, scope, status, git_sha, exit_code, output, notes,
			 started_at, finished_at, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		runID, command, scope, status, gitSHA, exitCode, nullIfEmpty(output), nullIfEmpty(notes),
		now, now, modelID,
	)
	return runID, err
}

// AddInputs attaches input triples to a run.
func AddInputs(db *sql.DB, runID string, inputs [][3]string, modelID string) error {
	now := storage.NowUTC()
	for _, triple := range inputs {
		id, err := storage.NewID()
		if err != nil {
			return err
		}
		if _, err := db.Exec(`
			INSERT INTO verification_inputs (id, run_id, file_path, role, content_hash, created_at, model_id)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, runID, triple[0], triple[1], triple[2], now, modelID,
		); err != nil {
			return err
		}
	}
	return nil
}

// FinishRun completes a verification run and parses failures when status is failed.
func FinishRun(db *sql.DB, runID, status string, exitCode *int, output string) error {
	now := storage.NowUTC()
	_, err := db.Exec(`
		UPDATE verification_runs
		SET status=?, exit_code=?, output=?, finished_at=? WHERE id=?`,
		status, exitCode, nullIfEmpty(output), now, runID,
	)
	if err != nil {
		return err
	}
	if status == "failed" {
		return replaceFailures(db, runID, output)
	}
	return nil
}

// Dismiss marks a run dismissed so it is excluded from failure counts.
func Dismiss(db *sql.DB, runID, reason string) (bool, error) {
	id, err := resolveRunID(db, runID)
	if err != nil {
		return false, err
	}
	now := storage.NowUTC()
	res, err := db.Exec(`
		UPDATE verification_runs
		SET dismissed_at=COALESCE(dismissed_at, ?), dismissed_reason=COALESCE(dismissed_reason, ?)
		WHERE id=? AND dismissed_at IS NULL`,
		now, nullIfEmpty(reason), id,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetRun loads a run by id prefix.
func GetRun(db *sql.DB, idPrefix string) (*Run, error) {
	id, err := resolveRunID(db, idPrefix)
	if err != nil {
		return nil, err
	}
	row := db.QueryRow(`SELECT id, command, scope, status, git_sha, exit_code, COALESCE(output,''),
		COALESCE(notes,''), started_at, COALESCE(finished_at,''), COALESCE(dismissed_at,''),
		COALESCE(dismissed_reason,''), created_at, model_id FROM verification_runs WHERE id=?`, id)
	return scanRun(row)
}

// GetInputs returns inputs for a run ordered by file path.
func GetInputs(db *sql.DB, runID string) ([]Input, error) {
	id, err := resolveRunID(db, runID)
	if err != nil {
		return nil, err
	}
	return getInputsByExactRunID(db, id)
}

func getInputsByExactRunID(db *sql.DB, runID string) ([]Input, error) {
	rows, err := db.Query(`
		SELECT id, run_id, file_path, role, content_hash, created_at, model_id
		FROM verification_inputs WHERE run_id=? ORDER BY file_path`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Input
	for rows.Next() {
		var in Input
		if err := rows.Scan(&in.ID, &in.RunID, &in.FilePath, &in.Role, &in.ContentHash, &in.CreatedAt, &in.ModelID); err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// GetFailures returns structured failures for a run.
func GetFailures(db *sql.DB, runID string, limit int) ([]Failure, error) {
	id, err := resolveRunID(db, runID)
	if err != nil {
		return nil, err
	}
	sqlText := `SELECT id, run_id, COALESCE(test_id,''), COALESCE(file_path,''), headline,
		COALESCE(message_excerpt,''), COALESCE(failure_kind,''), created_at, model_id
		FROM verification_failures WHERE run_id=? ORDER BY created_at ASC, id ASC`
	params := []any{id}
	if limit > 0 {
		sqlText += " LIMIT ?"
		params = append(params, limit)
	}
	rows, err := db.Query(sqlText, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Failure
	for rows.Next() {
		var f Failure
		if err := rows.Scan(&f.ID, &f.RunID, &f.TestID, &f.FilePath, &f.Headline,
			&f.MessageExcerpt, &f.FailureKind, &f.CreatedAt, &f.ModelID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Show builds a summary view for one run.
func Show(db *sql.DB, runID string) (*ShowSummary, error) {
	run, err := GetRun(db, runID)
	if err != nil || run == nil {
		return nil, err
	}
	inputs, err := GetInputs(db, run.ID)
	if err != nil {
		return nil, err
	}
	failures, err := GetFailures(db, run.ID, 0)
	if err != nil {
		return nil, err
	}
	triples := inputsToTriples(inputs)
	reuse := EvaluateReuse(db, run.Command, run.Scope, triples)
	return &ShowSummary{
		Run:        *run,
		Inputs:     inputs,
		InputCount: len(inputs),
		Failures:   failures,
		Reuse:      reuse,
	}, nil
}

// CollectInputsForScope gathers path:role:hash triples from repo_files.
func CollectInputsForScope(db *sql.DB, scope string) ([][3]string, error) {
	prefixes := scopePrefixes(scope)
	includeBroad := scopeIsFullRun(scope)
	rows, err := db.Query(`SELECT path, content_hash FROM repo_files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var inputs [][3]string
	for rows.Next() {
		var path string
		var hash sql.NullString
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		if path == "" || seen[path] {
			continue
		}
		role := inferRoleFromPath(path)
		if pathInScope(path, prefixes) || (includeBroad && broadScopeRoles[role]) {
			inputs = append(inputs, [3]string{path, role, hash.String})
			seen[path] = true
		}
	}
	return inputs, rows.Err()
}

// EvaluateFreshness checks whether stored inputs still match repo_files.
func EvaluateFreshness(db *sql.DB, runID string) (bool, string) {
	inputs, err := GetInputs(db, runID)
	if err != nil {
		inputs, err = getInputsByExactRunID(db, runID)
	}
	if err != nil || len(inputs) == 0 {
		return false, "no_inputs_stored"
	}
	stored := map[string][2]string{}
	for _, in := range inputs {
		stored[in.FilePath] = [2]string{in.Role, in.ContentHash}
	}
	run, err := GetRun(db, runID)
	if err != nil {
		run, err = getRunByExactID(db, runID)
		if err != nil {
			return false, "run_lookup_failed"
		}
	}
	if run == nil {
		return false, "run_missing"
	}
	cutoff := run.StartedAt
	if run.FinishedAt != "" {
		cutoff = run.FinishedAt
	}
	fullRun := scopeIsFullRun("")
	fullRun = scopeIsFullRun(run.Scope)
	for path, pair := range stored {
		var currentHash sql.NullString
		err := db.QueryRow(`SELECT content_hash FROM repo_files WHERE path=?`, path).Scan(&currentHash)
		if err == sql.ErrNoRows {
			return false, "input_file_removed: " + path
		}
		if currentHash.String != pair[1] {
			return false, "input_hash_changed: " + path
		}
	}
	events, _ := listFileChangeEventsSince(db, cutoff)
	if len(events) > 0 {
		for _, ev := range events {
			if _, ok := stored[ev.Path]; ok {
				continue
			}
			role := inferRoleFromPath(ev.Path)
			if broadScopeRoles[role] && !fullRun {
				continue
			}
			if broadScopeRoles[role] && fullRun {
				return false, fmt.Sprintf("broad_scope_file_changed: %s (%s)", ev.Path, role)
			}
			return false, "file_changed_since_run: " + ev.Path
		}
	}
	return true, "fresh"
}

func getRunByExactID(db *sql.DB, runID string) (*Run, error) {
	row := db.QueryRow(`SELECT id, command, scope, status, git_sha, exit_code, COALESCE(output,''),
		COALESCE(notes,''), started_at, COALESCE(finished_at,''), COALESCE(dismissed_at,''),
		COALESCE(dismissed_reason,''), created_at, model_id FROM verification_runs WHERE id=?`, runID)
	return scanRun(row)
}

// EvaluateReuse decides whether a prior passing run can be reused.
func EvaluateReuse(db *sql.DB, command, scope string, expected [][3]string) ReuseDecision {
	runID := findLatestRun(db, command, scope, expected, nil)
	if runID == "" {
		return ReuseDecision{Decision: "unknown", Reason: "no_prior_run"}
	}
	run, err := GetRun(db, runID)
	if err != nil || run == nil {
		return ReuseDecision{Decision: "unknown", Reason: "matching_run_missing", RunID: runID}
	}
	if run.Status == "failed" {
		failures, _ := GetFailures(db, runID, 5)
		failed := failuresToMaps(failures)
		return ReuseDecision{
			Decision: "failed_last_time", Reason: "latest_matching_run_failed",
			RunID: runID, RunStatus: run.Status, FailedTests: failed,
		}
	}
	if run.Status != "passed" {
		return ReuseDecision{
			Decision: "unknown", Reason: "latest_matching_run_status_" + run.Status,
			RunID: runID, RunStatus: run.Status,
		}
	}
	cutoff := run.FinishedAt
	if cutoff == "" {
		cutoff = run.StartedAt
	}
	fresh, reason := EvaluateFreshness(db, runID)
	events, _ := listFileChangeEventsSince(db, cutoff)
	if !fresh {
		changed := events
		if parsed := parseFreshnessReason(reason); parsed != nil {
			changed = append([]ChangedFile{*parsed}, changed...)
		}
		return ReuseDecision{
			Decision: "rerun_required", Reason: reason,
			RunID: runID, RunStatus: run.Status, ChangedFiles: changed,
		}
	}
	if len(events) > 0 {
		return ReuseDecision{
			Decision: "rerun_required", Reason: "file_changes_since_last_green_run",
			RunID: runID, RunStatus: run.Status, ChangedFiles: events,
		}
	}
	return ReuseDecision{
		Decision: "reusable", Reason: "no_file_changes_since_last_green_run",
		RunID: runID, RunStatus: run.Status,
	}
}

// Query builds the CLI query payload from command, scope, and inputs.
func Query(db *sql.DB, command, scope string, inputs [][3]string, autoInputs bool) QueryResult {
	decision := EvaluateReuse(db, command, scope, inputs)
	result := QueryResult{
		Command:         command,
		Scope:           scope,
		Status:          "unsupported",
		Fresh:           false,
		Reason:          decision.Reason,
		Decision:        decision.Decision,
		ChangedFiles:    decision.ChangedFiles,
		FailedTests:     decision.FailedTests,
		AutoInputs:      autoInputs,
		InputsCollected: len(inputs),
	}
	if decision.RunID != "" {
		result.RunID = decision.RunID
		result.Status = decision.Decision
		result.Reason = decision.Reason
		switch decision.Decision {
		case "reusable":
			result.Fresh = true
			result.Status = "fresh_pass"
		case "rerun_required":
			result.Status = "stale_pass"
		case "failed_last_time":
			result.Status = "failed_last_time"
		}
	} else if decision.Reason != "no_prior_run" {
		result.Reason = decision.Reason
	}
	return result
}

// CompactQueryLine formats a human reuse query line.
func CompactQueryLine(q QueryResult) string {
	var message string
	switch q.Status {
	case "fresh_pass":
		message = "skip rerun"
	case "stale_pass":
		message = "rerun required"
	case "failed_last_time":
		message = "latest run failed"
	default:
		message = q.Status
	}
	line := fmt.Sprintf("%s: %s", message, q.Reason)
	if q.RunID != "" {
		line += fmt.Sprintf(" (run %s...)", q.RunID[:8])
	}
	if len(q.ChangedFiles) > 0 {
		var paths []string
		for _, item := range q.ChangedFiles {
			if item.Path != "" {
				paths = append(paths, item.Path)
			}
			if len(paths) >= 5 {
				break
			}
		}
		if len(paths) > 0 {
			line += " [changed: " + strings.Join(paths, ", ") + "]"
		}
	}
	if len(q.FailedTests) > 0 {
		var names []string
		for _, item := range q.FailedTests {
			if tid, ok := item["test_id"].(string); ok && tid != "" {
				names = append(names, tid)
			} else if h, ok := item["headline"].(string); ok {
				names = append(names, h)
			}
			if len(names) >= 5 {
				break
			}
		}
		if len(names) > 0 {
			line += " [failed: " + strings.Join(names, ", ") + "]"
		}
	}
	return line
}

func findLatestRun(db *sql.DB, command, scope string, expected [][3]string, statuses []string) string {
	sqlText := `SELECT id FROM verification_runs WHERE command=? AND scope=?`
	params := []any{command, scope}
	if len(statuses) > 0 {
		sorted := append([]string(nil), statuses...)
		sort.Strings(sorted)
		placeholders := make([]string, len(sorted))
		for i, s := range sorted {
			placeholders[i] = "?"
			params = append(params, s)
		}
		sqlText += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}
	sqlText += " ORDER BY COALESCE(finished_at, started_at, created_at) DESC LIMIT 20"
	rows, err := db.Query(sqlText, params...)
	if err != nil {
		return ""
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ""
		}
		candidates = append(candidates, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ""
	}
	expectedMap := triplesToMap(expected)
	for _, id := range candidates {
		stored, err := GetInputs(db, id)
		if err != nil {
			continue
		}
		if mapsEqual(triplesToMap(inputsToTriples(stored)), expectedMap) {
			return id
		}
	}
	return ""
}

func replaceFailures(db *sql.DB, runID, output string) error {
	if _, err := db.Exec(`DELETE FROM verification_failures WHERE run_id=?`, runID); err != nil {
		return err
	}
	run, _ := GetRun(db, runID)
	modelID := "unknown"
	if run != nil {
		modelID = run.ModelID
	}
	failures := extractFailures(run, output)
	now := storage.NowUTC()
	for _, f := range failures {
		id, err := storage.NewID()
		if err != nil {
			return err
		}
		_, err = db.Exec(`
			INSERT INTO verification_failures
				(id, run_id, test_id, file_path, headline, message_excerpt, failure_kind, created_at, model_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, runID, nullIfEmpty(f.TestID), nullIfEmpty(f.FilePath), f.Headline,
			nullIfEmpty(f.MessageExcerpt), nullIfEmpty(f.FailureKind), now, modelID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func extractFailures(run *Run, output string) []Failure {
	if run != nil {
		fields := strings.Fields(run.Command)
		if len(fields) > 0 && fields[0] == "pytest" {
			if parsed := parsePytestFailures(output); len(parsed) > 0 {
				return parsed
			}
		}
	}
	return genericFailures(output)
}

func parsePytestFailures(output string) []Failure {
	var out []Failure
	capture := false
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.Contains(strings.ToLower(line), "short test summary info") {
			capture = true
			continue
		}
		if !capture {
			continue
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "=") {
			capture = false
			continue
		}
		m := pytestSummaryRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kind, testID, excerpt := m[1], m[2], m[3]
		filePath := testID
		if idx := strings.Index(testID, "::"); idx >= 0 {
			filePath = testID[:idx]
		}
		out = append(out, Failure{
			TestID: testID, FilePath: filePath,
			Headline:       strings.ToLower(kind) + " " + testID,
			MessageExcerpt: excerpt, FailureKind: strings.ToLower(kind),
		})
	}
	return out
}

func genericFailures(output string) []Failure {
	if output == "" {
		return nil
	}
	var excerpt string
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line != "" {
			excerpt = line
			break
		}
	}
	return []Failure{{
		Headline: "verification failed", MessageExcerpt: excerpt, FailureKind: "generic",
	}}
}

func listFileChangeEventsSince(db *sql.DB, timestamp string) ([]ChangedFile, error) {
	rows, err := db.Query(`
		SELECT path, change_kind, COALESCE(old_hash,''), COALESCE(new_hash,''), created_at
		FROM file_change_events WHERE created_at > ? ORDER BY created_at ASC, path ASC`, timestamp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChangedFile
	for rows.Next() {
		var ev ChangedFile
		if err := rows.Scan(&ev.Path, &ev.ChangeKind, &ev.OldHash, &ev.NewHash, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func parseFreshnessReason(reason string) *ChangedFile {
	if !strings.Contains(reason, ": ") {
		return nil
	}
	parts := strings.SplitN(reason, ": ", 2)
	kind, remainder := parts[0], parts[1]
	path := remainder
	if kind == "broad_scope_file_changed" && strings.Contains(remainder, " (") {
		path = strings.Split(remainder, " (")[0]
	}
	return &ChangedFile{Path: path, ChangeKind: kind}
}

func inferRoleFromPath(path string) string {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "conftest") || strings.Contains(lower, "fixture") || strings.Contains(lower, "setup") {
		return "fixture"
	}
	for _, name := range []string{"pytest.ini", "setup.cfg", "pyproject.toml", ".env", "config", "tox.ini"} {
		if strings.Contains(lower, name) {
			return "config"
		}
	}
	for _, name := range []string{"requirements", "poetry.lock", "package.json", "cargo.toml", "go.sum"} {
		if strings.Contains(lower, name) {
			return "dependency"
		}
	}
	for _, name := range []string{"makefile", "dockerfile", "github", "gitlab", "ci/", ".github/"} {
		if strings.Contains(lower, name) {
			return "tooling"
		}
	}
	if strings.Contains(lower, "test") || strings.HasPrefix(lower, "tests") {
		return "test"
	}
	return "source"
}

func scopePrefixes(scope string) []string {
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "." {
		return []string{"."}
	}
	var out []string
	for _, part := range strings.Fields(scope) {
		out = append(out, strings.TrimRight(part, "/"))
	}
	return out
}

func scopeIsFullRun(scope string) bool {
	prefixes := scopePrefixes(scope)
	return len(prefixes) == 1 && prefixes[0] == "."
}

func pathInScope(path string, prefixes []string) bool {
	if len(prefixes) == 1 && prefixes[0] == "." {
		return true
	}
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func mapsEqual(a, b map[string][2]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va != vb {
			return false
		}
	}
	return true
}

func triplesToMap(triples [][3]string) map[string][2]string {
	out := make(map[string][2]string, len(triples))
	for _, t := range triples {
		out[t[0]] = [2]string{t[1], t[2]}
	}
	return out
}

func inputsToTriples(inputs []Input) [][3]string {
	out := make([][3]string, len(inputs))
	for i, in := range inputs {
		out[i] = [3]string{in.FilePath, in.Role, in.ContentHash}
	}
	return out
}

func failuresToMaps(failures []Failure) []map[string]any {
	out := make([]map[string]any, len(failures))
	for i, f := range failures {
		out[i] = map[string]any{
			"test_id": f.TestID, "file_path": f.FilePath,
			"headline": f.Headline, "message_excerpt": f.MessageExcerpt,
			"failure_kind": f.FailureKind,
		}
	}
	return out
}

func resolveRunID(db *sql.DB, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if len(prefix) == 32 {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM verification_runs WHERE id=?`, prefix).Scan(&n); err != nil {
			return "", err
		}
		if n == 1 {
			return prefix, nil
		}
	}
	rows, err := db.Query(`SELECT id FROM verification_runs`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	return storage.ResolveID(prefix, ids)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(row rowScanner) (*Run, error) {
	var r Run
	var exitCode sql.NullInt64
	var output, notes, finished, dismissed, dismissedReason sql.NullString
	if err := row.Scan(&r.ID, &r.Command, &r.Scope, &r.Status, &r.GitSHA, &exitCode,
		&output, &notes, &r.StartedAt, &finished, &dismissed, &dismissedReason,
		&r.CreatedAt, &r.ModelID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		r.ExitCode = &v
	}
	if output.Valid {
		r.Output = output.String
	}
	if notes.Valid {
		r.Notes = notes.String
	}
	if finished.Valid {
		r.FinishedAt = finished.String
	}
	if dismissed.Valid {
		r.DismissedAt = dismissed.String
	}
	if dismissedReason.Valid {
		r.DismissedReason = dismissedReason.String
	}
	return &r, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
