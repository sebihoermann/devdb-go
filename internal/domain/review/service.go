package review

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

var (
	defaultPrinciples = map[string]bool{
		"correctness": true, "kiss": true, "dry": true, "yagni": true,
		"separation-of-concerns": true, "error-handling": true, "test-coverage": true, "other": true,
	}
	grassPrinciples = map[string]bool{
		"dead": true, "inlinable": true, "sprawl": true, "swelling": true, "duplication": true, "staleness": true,
	}
	extendedPrinciples = map[string]bool{
		"security-footguns": true, "performance-hotspots": true, "migration-safety": true,
		"observability-debuggability": true, "api-boundaries": true, "naming-clarity": true,
		"dead-code": true, "coupling-cohesion": true, "configuration-clarity": true,
		"documentation-accuracy": true, "dependency-hygiene": true,
	}
	severityWeights   = map[string]int{"info": 1, "low": 2, "med": 4, "high": 8, "critical": 16}
	confidenceWeights = map[string]int{"low": 1, "medium": 2, "high": 3}
	effortWeights     = map[string]int{"trivial": 1, "small": 2, "med": 4, "large": 8}
	tierCap           = map[string]int{"default": 3, "extended": 5, "grass-cutter": 50}
)

// ErrRunNotFound means the review run id does not exist.
var ErrRunNotFound = errors.New("run not found")

// ErrRunFinished means the review run is already finished.
var ErrRunFinished = errors.New("run finished")

// ErrFindingNotFound means the finding id does not exist.
var ErrFindingNotFound = errors.New("finding not found")

// ErrCapExceeded means the per-file finding cap for the tier was hit.
var ErrCapExceeded = errors.New("finding cap exceeded")

// Run is a code review run row.
type Run struct {
	ID             string   `json:"id"`
	ScopePaths     []string `json:"scope_paths"`
	Tier           string   `json:"tier"`
	StartedAt      string   `json:"started_at"`
	FinishedAt     string   `json:"finished_at,omitempty"`
	GitSHA         string   `json:"git_sha,omitempty"`
	FilesTotal     *int     `json:"files_total,omitempty"`
	FilesReviewed  *int     `json:"files_reviewed,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	ModelID        string   `json:"model_id"`
}

// Finding is a review finding row.
type Finding struct {
	ID               string   `json:"id"`
	RunID            string   `json:"run_id"`
	FilePath         string   `json:"file_path,omitempty"`
	LineStart        *int     `json:"line_start,omitempty"`
	LineEnd          *int     `json:"line_end,omitempty"`
	Principle        string   `json:"principle"`
	Title            string   `json:"title"`
	Recommendation   string   `json:"recommendation"`
	Severity         string   `json:"severity"`
	Confidence       string   `json:"confidence"`
	Effort           string   `json:"effort"`
	Status           string   `json:"status"`
	ResolvedInCommit string   `json:"resolved_in_commit,omitempty"`
	SourceHash       string   `json:"source_hash,omitempty"`
	CreatedAt        string   `json:"created_at"`
	ModelID          string   `json:"model_id"`
	Priority         float64  `json:"priority,omitempty"`
}

// ImportResult is the outcome of a batch finding import.
type ImportResult struct {
	Imported   []string         `json:"imported"`
	SkippedCap []map[string]string `json:"skipped_cap"`
	Errors     []map[string]string `json:"errors"`
}

// ListFilter selects findings.
type ListFilter struct {
	Status    string
	RunID     string
	Principle string
	FilePath  string
	Severity  string
	Limit     int
}

// PrinciplesForTier returns sorted allowed principle tokens.
func PrinciplesForTier(tier string) []string {
	allowed := allowedPrinciples(tier)
	out := make([]string, 0, len(allowed))
	for p := range allowed {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func allowedPrinciples(tier string) map[string]bool {
	switch tier {
	case "extended":
		out := make(map[string]bool, len(defaultPrinciples)+len(extendedPrinciples))
		for k, v := range defaultPrinciples {
			out[k] = v
		}
		for k, v := range extendedPrinciples {
			out[k] = v
		}
		return out
	case "grass-cutter":
		out := make(map[string]bool, len(defaultPrinciples)+len(grassPrinciples))
		for k, v := range defaultPrinciples {
			out[k] = v
		}
		for k, v := range grassPrinciples {
			out[k] = v
		}
		return out
	default:
		return defaultPrinciples
	}
}

// NormalizeSeverity maps aliases to canonical severity tokens.
func NormalizeSeverity(v string) string {
	switch strings.ToLower(v) {
	case "medium", "med":
		return "med"
	case "low", "high", "critical", "info":
		return strings.ToLower(v)
	default:
		return v
	}
}

// NormalizeConfidence maps aliases to canonical confidence tokens.
func NormalizeConfidence(v string) string {
	switch strings.ToLower(v) {
	case "med", "medium":
		return "medium"
	case "low", "high":
		return strings.ToLower(v)
	default:
		return v
	}
}

// NormalizeEffort maps aliases to canonical effort tokens.
func NormalizeEffort(v string) string {
	switch strings.ToLower(v) {
	case "medium", "med":
		return "med"
	case "trivial", "small", "large":
		return strings.ToLower(v)
	default:
		return v
	}
}

// StartRun opens a new review run.
func StartRun(db *sql.DB, scopePaths []string, tier, gitSHA, modelID string) (string, error) {
	if tier == "" {
		tier = "default"
	}
	if len(scopePaths) == 0 {
		scopePaths = []string{"."}
	}
	runID, err := storage.NewID()
	if err != nil {
		return "", err
	}
	clause, params := scopeClause(scopePaths)
	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM repo_files"+clause, params...).Scan(&total); err != nil {
		return "", err
	}
	scopeJSON, _ := json.Marshal(scopePaths)
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO review_runs
			(id, scope_paths, tier, started_at, finished_at, git_sha, files_total, files_reviewed, summary, model_id)
		VALUES (?, ?, ?, ?, NULL, ?, ?, NULL, NULL, ?)`,
		runID, string(scopeJSON), tier, now, nullIfEmpty(gitSHA), total, modelID,
	)
	return runID, err
}

// AddFinding inserts one finding into an open run.
func AddFinding(db *sql.DB, runID string, f FindingInput, modelID string) (string, error) {
	run, err := GetRun(db, runID)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", ErrRunNotFound
	}
	if run.FinishedAt != "" {
		return "", ErrRunFinished
	}
	f.Severity = NormalizeSeverity(f.Severity)
	f.Confidence = NormalizeConfidence(f.Confidence)
	f.Effort = NormalizeEffort(f.Effort)
	if err := validateFinding(run.Tier, f); err != nil {
		return "", err
	}
	var existing int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM review_findings WHERE run_id=? AND file_path=?`,
		runID, nullIfEmpty(f.FilePath),
	).Scan(&existing); err != nil {
		return "", err
	}
	cap := tierCap[run.Tier]
	if cap == 0 {
		cap = 3
	}
	if existing >= cap {
		return "", ErrCapExceeded
	}
	var sourceHash sql.NullString
	if f.FilePath != "" {
		_ = db.QueryRow(`SELECT content_hash FROM repo_files WHERE path=?`, f.FilePath).Scan(&sourceHash)
	}
	findingID, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO review_findings
			(id, run_id, file_path, line_start, line_end, principle, title, recommendation,
			 severity, confidence, effort, status, resolved_in_commit, source_hash, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', NULL, ?, ?, ?)`,
		findingID, runID, nullIfEmpty(f.FilePath), f.LineStart, f.LineEnd,
		f.Principle, f.Title, f.Recommendation,
		f.Severity, f.Confidence, f.Effort,
		sourceHash, now, modelID,
	)
	return findingID, err
}

// FindingInput is the payload for adding or importing a finding.
type FindingInput struct {
	FilePath       string
	LineStart      *int
	LineEnd        *int
	Principle      string
	Title          string
	Recommendation string
	Severity       string
	Confidence     string
	Effort         string
}

// ImportFindings batch-imports findings into an open run.
func ImportFindings(db *sql.DB, runID string, items []FindingInput, forceCap bool, modelID string) (ImportResult, error) {
	var result ImportResult
	for i, item := range items {
		id, err := AddFinding(db, runID, item, modelID)
		if errors.Is(err, ErrCapExceeded) {
			if forceCap {
				id, err = addFindingUncapped(db, runID, item, modelID)
				if err != nil {
					result.Errors = append(result.Errors, map[string]string{
						"index": fmt.Sprint(i), "error": err.Error(),
					})
					continue
				}
				result.Imported = append(result.Imported, id)
				continue
			}
			result.SkippedCap = append(result.SkippedCap, map[string]string{
				"index": fmt.Sprint(i), "file": item.FilePath,
			})
			continue
		}
		if errors.Is(err, ErrRunNotFound) || errors.Is(err, ErrRunFinished) {
			return result, err
		}
		if err != nil {
			result.Errors = append(result.Errors, map[string]string{
				"index": fmt.Sprint(i), "error": err.Error(),
			})
			continue
		}
		result.Imported = append(result.Imported, id)
	}
	return result, nil
}

// FinishRun marks a review run complete.
func FinishRun(db *sql.DB, runID, summary string) (bool, error) {
	run, err := GetRun(db, runID)
	if err != nil {
		return false, err
	}
	if run == nil {
		return false, nil
	}
	if run.FinishedAt != "" {
		return false, ErrRunFinished
	}
	var filesReviewed int
	if err := db.QueryRow(`
		SELECT COUNT(DISTINCT file_path) FROM review_findings
		WHERE run_id=? AND file_path IS NOT NULL`, runID).Scan(&filesReviewed); err != nil {
		return false, err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		UPDATE review_runs SET finished_at=?, files_reviewed=?, summary=? WHERE id=?`,
		now, filesReviewed, nullIfEmpty(summary), runID,
	)
	return true, err
}

// ListFindings returns findings sorted by priority descending.
func ListFindings(db *sql.DB, f ListFilter) ([]Finding, error) {
	clauses := []string{}
	params := []any{}
	if f.Status != "" {
		clauses = append(clauses, "status=?")
		params = append(params, f.Status)
	}
	if f.RunID != "" {
		clauses = append(clauses, "run_id=?")
		params = append(params, f.RunID)
	}
	if f.Principle != "" {
		clauses = append(clauses, "principle=?")
		params = append(params, f.Principle)
	}
	if f.FilePath != "" {
		clauses = append(clauses, "file_path=?")
		params = append(params, f.FilePath)
	}
	if f.Severity != "" {
		clauses = append(clauses, "severity=?")
		params = append(params, NormalizeSeverity(f.Severity))
	}
	sqlText := `SELECT id, run_id, COALESCE(file_path,''), line_start, line_end, principle, title,
		recommendation, severity, confidence, effort, status, COALESCE(resolved_in_commit,''),
		COALESCE(source_hash,''), created_at, model_id FROM review_findings`
	if len(clauses) > 0 {
		sqlText += " WHERE " + strings.Join(clauses, " AND ")
	}
	rows, err := db.Query(sqlText, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		f, err := scanFinding(rows)
		if err != nil {
			return nil, err
		}
		f.Priority = priorityValue(*f)
		out = append(out, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// ResolveFinding updates finding status.
func ResolveFinding(db *sql.DB, findingID, commitSHA, status, evidence string) (bool, error) {
	id, err := resolveFindingID(db, findingID)
	if err != nil {
		return false, err
	}
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_findings WHERE id=?`, id).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, nil
	}
	resolution := commitSHA
	if resolution == "" {
		resolution = evidence
	}
	if status == "resolved" && resolution == "" {
		return false, fmt.Errorf("commit or evidence required for status=resolved")
	}
	_, err = db.Exec(`
		UPDATE review_findings
		SET status=?, resolved_in_commit=COALESCE(?, resolved_in_commit) WHERE id=?`,
		status, nullIfEmpty(resolution), id,
	)
	return true, err
}

// GetRun loads a review run by id prefix.
func GetRun(db *sql.DB, idPrefix string) (*Run, error) {
	id, err := resolveRunID(db, idPrefix)
	if err != nil {
		return nil, err
	}
	row := db.QueryRow(`SELECT id, scope_paths, tier, started_at, COALESCE(finished_at,''),
		COALESCE(git_sha,''), files_total, files_reviewed, COALESCE(summary,''), model_id
		FROM review_runs WHERE id=?`, id)
	return scanRun(row)
}

// GetFinding loads one finding by id prefix.
func GetFinding(db *sql.DB, idPrefix string) (*Finding, error) {
	id, err := resolveFindingID(db, idPrefix)
	if err != nil {
		return nil, err
	}
	row := db.QueryRow(`SELECT id, run_id, COALESCE(file_path,''), line_start, line_end, principle, title,
		recommendation, severity, confidence, effort, status, COALESCE(resolved_in_commit,''),
		COALESCE(source_hash,''), created_at, model_id FROM review_findings WHERE id=?`, id)
	f, err := scanFinding(row)
	if err != nil || f == nil {
		return f, err
	}
	f.Priority = priorityValue(*f)
	return f, nil
}

// RenderReport returns compact markdown for a review run.
func RenderReport(db *sql.DB, runID string) (string, error) {
	run, err := GetRun(db, runID)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", ErrRunNotFound
	}
	findings, err := ListFindings(db, ListFilter{RunID: run.ID})
	if err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("# Review Run %s", run.ID),
		fmt.Sprintf("- tier: %s", run.Tier),
		fmt.Sprintf("- started: %s", run.StartedAt),
		fmt.Sprintf("- finished: %s", run.FinishedAt),
	}
	if run.FilesTotal != nil {
		lines = append(lines, fmt.Sprintf("- files_total: %d", *run.FilesTotal))
	}
	if run.FilesReviewed != nil {
		lines = append(lines, fmt.Sprintf("- files_reviewed: %d", *run.FilesReviewed))
	}
	if run.Summary != "" {
		lines = append(lines, fmt.Sprintf("- summary: %s", run.Summary))
	}
	lines = append(lines, "", "## Findings")
	for _, f := range findings {
		loc := f.FilePath
		if loc == "" {
			loc = "cross-cutting"
		}
		if f.LineStart != nil {
			loc = fmt.Sprintf("%s:%d", loc, *f.LineStart)
		}
		lines = append(lines, fmt.Sprintf("- %s · %s · %s (%s/%s/%s)",
			loc, f.Principle, f.Title, f.Severity, f.Confidence, f.Effort))
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// CompactLines formats findings for human list output.
func CompactLines(findings []Finding) []string {
	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		loc := f.FilePath
		if loc == "" {
			loc = "cross-cutting"
		}
		if f.LineStart != nil {
			loc = fmt.Sprintf("%s:%d", loc, *f.LineStart)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %s p=%.2f",
			f.ID[:8], loc, f.Principle, f.Title, f.Priority))
	}
	return lines
}

func validateFinding(tier string, f FindingInput) error {
	if !allowedPrinciples(tier)[f.Principle] {
		return fmt.Errorf("invalid principle for tier %s; allowed: %s",
			tier, strings.Join(PrinciplesForTier(tier), ", "))
	}
	if _, ok := severityWeights[f.Severity]; !ok {
		return fmt.Errorf("invalid severity")
	}
	if _, ok := confidenceWeights[f.Confidence]; !ok {
		return fmt.Errorf("invalid confidence; allowed: low, medium, high")
	}
	if _, ok := effortWeights[f.Effort]; !ok {
		return fmt.Errorf("invalid effort")
	}
	return nil
}

func addFindingUncapped(db *sql.DB, runID string, f FindingInput, modelID string) (string, error) {
	run, err := GetRun(db, runID)
	if err != nil || run == nil {
		return "", ErrRunNotFound
	}
	f.Severity = NormalizeSeverity(f.Severity)
	f.Confidence = NormalizeConfidence(f.Confidence)
	f.Effort = NormalizeEffort(f.Effort)
	if err := validateFinding(run.Tier, f); err != nil {
		return "", err
	}
	var sourceHash sql.NullString
	if f.FilePath != "" {
		_ = db.QueryRow(`SELECT content_hash FROM repo_files WHERE path=?`, f.FilePath).Scan(&sourceHash)
	}
	findingID, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO review_findings
			(id, run_id, file_path, line_start, line_end, principle, title, recommendation,
			 severity, confidence, effort, status, resolved_in_commit, source_hash, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', NULL, ?, ?, ?)`,
		findingID, runID, nullIfEmpty(f.FilePath), f.LineStart, f.LineEnd,
		f.Principle, f.Title, f.Recommendation,
		f.Severity, f.Confidence, f.Effort,
		sourceHash, now, modelID,
	)
	return findingID, err
}

func priorityValue(f Finding) float64 {
	sev := severityWeights[f.Severity]
	if sev == 0 {
		sev = 1
	}
	conf := confidenceWeights[f.Confidence]
	if conf == 0 {
		conf = 1
	}
	eff := effortWeights[f.Effort]
	if eff == 0 {
		eff = 1
	}
	return float64(sev*conf) / float64(eff)
}

func scopeClause(scopePaths []string) (string, []any) {
	if len(scopePaths) == 0 || (len(scopePaths) == 1 && scopePaths[0] == ".") {
		return "", nil
	}
	clauses := make([]string, 0, len(scopePaths))
	params := make([]any, 0, len(scopePaths)*2)
	for _, scope := range scopePaths {
		normalized := strings.TrimRight(scope, "/")
		clauses = append(clauses, "(path=? OR path LIKE ?)")
		params = append(params, normalized, normalized+"/%")
	}
	return " WHERE " + strings.Join(clauses, " OR "), params
}

func resolveRunID(db *sql.DB, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if len(prefix) == 32 {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM review_runs WHERE id=?`, prefix).Scan(&n); err != nil {
			return "", err
		}
		if n == 1 {
			return prefix, nil
		}
	}
	rows, err := db.Query(`SELECT id FROM review_runs`)
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

func resolveFindingID(db *sql.DB, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if len(prefix) == 32 {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM review_findings WHERE id=?`, prefix).Scan(&n); err != nil {
			return "", err
		}
		if n == 1 {
			return prefix, nil
		}
	}
	rows, err := db.Query(`SELECT id FROM review_findings`)
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
	var scopeJSON string
	var finished sql.NullString
	var gitSHA sql.NullString
	var summary sql.NullString
	var filesTotal, filesReviewed sql.NullInt64
	if err := row.Scan(&r.ID, &scopeJSON, &r.Tier, &r.StartedAt, &finished, &gitSHA,
		&filesTotal, &filesReviewed, &summary, &r.ModelID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopeJSON), &r.ScopePaths)
	if finished.Valid {
		r.FinishedAt = finished.String
	}
	if gitSHA.Valid {
		r.GitSHA = gitSHA.String
	}
	if summary.Valid {
		r.Summary = summary.String
	}
	if filesTotal.Valid {
		v := int(filesTotal.Int64)
		r.FilesTotal = &v
	}
	if filesReviewed.Valid {
		v := int(filesReviewed.Int64)
		r.FilesReviewed = &v
	}
	return &r, nil
}

func scanFinding(row rowScanner) (*Finding, error) {
	var f Finding
	var lineStart, lineEnd sql.NullInt64
	if err := row.Scan(&f.ID, &f.RunID, &f.FilePath, &lineStart, &lineEnd,
		&f.Principle, &f.Title, &f.Recommendation, &f.Severity, &f.Confidence, &f.Effort,
		&f.Status, &f.ResolvedInCommit, &f.SourceHash, &f.CreatedAt, &f.ModelID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if lineStart.Valid {
		v := int(lineStart.Int64)
		f.LineStart = &v
	}
	if lineEnd.Valid {
		v := int(lineEnd.Int64)
		f.LineEnd = &v
	}
	return &f, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
