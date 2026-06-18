package feedback

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/git"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

var (
	headingRE     = regexp.MustCompile(`^##\s+(.+?)\s*$`)
	metaLineRE    = regexp.MustCompile(`^\s*-\s+\*\*([^*]+)\*\*:\s*(.*)\s*$`)
	intentPrefixRE = regexp.MustCompile(`^(?P<intent>[a-z]+)(?:\([^)]+\))?:\s*`)
)

var intentPrefixes = map[string]string{
	"feat": "feat", "fix": "fix", "refactor": "refactor", "docs": "docs",
	"chore": "chore", "test": "test", "perf": "perf", "ci": "ci",
	"build": "build", "style": "style",
}

var featureVerbs = []string{
	"add", "build", "create", "enable", "implement", "introduce", "make", "support",
}

// MarkdownEntry is one ## section from a feedback markdown archive.
type MarkdownEntry struct {
	Title   string            `json:"title"`
	Meta    map[string]string `json:"meta"`
	HasMeta bool              `json:"has_meta"`
}

// ImportMarkdownResult reports rows created from a markdown file.
type ImportMarkdownResult struct {
	Imported int      `json:"imported"`
	IDs      []string `json:"ids"`
}

// ImportCommitsResult reports commit archeology rows inserted.
type ImportCommitsResult struct {
	Inserted int      `json:"inserted"`
	IDs      []string `json:"ids"`
	Warnings []string `json:"warnings,omitempty"`
}

// DetectIntent infers a coarse intent tag from free-form text (commit subject, note, etc.).
func DetectIntent(text string) string {
	candidate := strings.TrimSpace(strings.ToLower(text))
	if candidate == "" {
		return "other"
	}
	if m := intentPrefixRE.FindStringSubmatch(candidate); len(m) > 1 {
		intent := m[1]
		if mapped, ok := intentPrefixes[intent]; ok {
			return mapped
		}
		return intent
	}
	for _, verb := range featureVerbs {
		if candidate == verb || strings.HasPrefix(candidate, verb+" ") {
			return "feat"
		}
	}
	if strings.HasPrefix(candidate, "bug") || strings.HasPrefix(candidate, "error") || strings.HasPrefix(candidate, "broken") {
		return "fix"
	}
	return "other"
}

// IterMarkdownEntries parses feedback markdown: each ## heading starts an entry;
// lines like `- **Role**: model` contribute metadata.
func IterMarkdownEntries(text string) []MarkdownEntry {
	var out []MarkdownEntry
	var currentTitle string
	var currentMeta map[string]string
	var sawContent bool

	flush := func() {
		if currentTitle == "" {
			return
		}
		out = append(out, MarkdownEntry{
			Title:   currentTitle,
			Meta:    currentMeta,
			HasMeta: sawContent,
		})
	}

	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if m := headingRE.FindStringSubmatch(line); m != nil {
			flush()
			currentTitle = strings.TrimSpace(m[1])
			currentMeta = map[string]string{}
			sawContent = false
			continue
		}
		if m := metaLineRE.FindStringSubmatch(line); m != nil && currentTitle != "" {
			currentMeta[strings.ToLower(strings.TrimSpace(m[1]))] = strings.TrimSpace(m[2])
			sawContent = true
		}
	}
	flush()
	return out
}

// ImportMarkdown reads PATH and inserts feedback rows for entries with metadata lines.
func ImportMarkdown(db *sql.DB, path, modelID string) (ImportMarkdownResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ImportMarkdownResult{}, fmt.Errorf("read markdown: %w", err)
	}
	return ImportMarkdownText(db, string(data), modelID)
}

// ImportMarkdownText inserts feedback from parsed markdown text.
func ImportMarkdownText(db *sql.DB, text, modelID string) (ImportMarkdownResult, error) {
	result := ImportMarkdownResult{}
	for _, entry := range IterMarkdownEntries(text) {
		if !entry.HasMeta {
			continue
		}
		note := strings.TrimSpace(entry.Title)
		if note == "" {
			continue
		}
		role := strings.ToLower(entry.Meta["role"])
		if role == "" {
			role = "codebase"
		}
		if role != "user" && role != "model" && role != "codebase" {
			return ImportMarkdownResult{}, fmt.Errorf("invalid role %q in entry %q", role, note)
		}
		context := entry.Meta["context"]
		if story := entry.Meta["story-dir"]; story != "" {
			if context == "" {
				context = "story-dir: " + story
			} else {
				context = context + "\nstory-dir: " + story
			}
		}
		id, err := storage.NewID()
		if err != nil {
			return ImportMarkdownResult{}, err
		}
		now := storage.NowUTC()
		status := entry.Meta["status"]
		if status == "" {
			status = "open"
		}
		_, err = db.Exec(`
			INSERT INTO feedback(id, role, category, severity, note, context, status, proposed_fix, created_at, model_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, role, nullIfEmpty(entry.Meta["category"]), normalizeSeverity(entry.Meta["severity"]),
			note, nullIfEmpty(context), status, nullIfEmpty(entry.Meta["proposed-fix"]),
			now, modelID,
		)
		if err != nil {
			return ImportMarkdownResult{}, err
		}
		result.Imported++
		result.IDs = append(result.IDs, id)
	}
	return result, nil
}

// ImportCommits walks git history for branches and stores rows in commit_archeology.
func ImportCommits(db *sql.DB, repoRoot string, branches []string, limit int, modelID string) (ImportCommitsResult, error) {
	if limit <= 0 {
		limit = 200
	}
	result := ImportCommitsResult{}
	seen := map[string]bool{}
	rows, err := db.Query(`SELECT sha FROM commit_archeology`)
	if err != nil {
		return ImportCommitsResult{}, err
	}
	for rows.Next() {
		var sha string
		if err := rows.Scan(&sha); err != nil {
			rows.Close()
			return ImportCommitsResult{}, err
		}
		seen[sha] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ImportCommitsResult{}, err
	}
	rows.Close()

	for _, branch := range branches {
		commits, err := git.Log(repoRoot, branch, limit)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to read branch %q: %v", branch, err))
			continue
		}
		for _, commit := range commits {
			if seen[commit.SHA] {
				continue
			}
			seen[commit.SHA] = true
			id, err := storage.NewID()
			if err != nil {
				return ImportCommitsResult{}, err
			}
			intent := DetectIntent(commit.Subject + "\n" + commit.Body)
			now := storage.NowUTC()
			res, err := db.Exec(`
				INSERT OR IGNORE INTO commit_archeology
				(id, branch, sha, author, committed_at, subject, body, intent_tag, created_at, model_id)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, branch, commit.SHA, nullIfEmpty(commit.Author), nullIfEmpty(commit.Date),
				commit.Subject, commit.Body, intent, now, modelID,
			)
			if err != nil {
				return ImportCommitsResult{}, err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				continue
			}
			result.Inserted++
			result.IDs = append(result.IDs, id)
		}
	}
	return result, nil
}
