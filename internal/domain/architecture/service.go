package architecture

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

var (
	topicRE      = regexp.MustCompile(`^[a-z][a-z0-9-]{2,40}$`)
	bannedTopics = map[string]bool{
		"misc": true, "general": true, "notes": true, "stuff": true, "improvements": true,
	}
)

// Note is an architecture knowledge row.
type Note struct {
	ID             string            `json:"id"`
	Topic          string            `json:"topic"`
	Body           string            `json:"body"`
	SourcePaths    []string          `json:"source_paths"`
	SourceHashes   map[string]string `json:"source_hashes"`
	Confidence     string            `json:"confidence"`
	Status         string            `json:"status"`
	LastVerifiedAt string            `json:"last_verified_at"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
	Stale          bool              `json:"stale,omitempty"`
}

// ListFilter selects architecture notes.
type ListFilter struct {
	TopicSubstr  string
	TouchingPath string
	Stale        bool
	Status       string
	Limit        int
}

// ValidateTopic checks topic naming rules.
func ValidateTopic(topic string) error {
	if !topicRE.MatchString(topic) || bannedTopics[topic] {
		return fmt.Errorf("invalid topic")
	}
	return nil
}

// Add creates a note; source paths must exist in repo_files.
func Add(db *sql.DB, topic, body string, sourcePaths []string, confidence, modelID string) (string, error) {
	if err := ValidateTopic(topic); err != nil {
		return "", err
	}
	if confidence == "" {
		confidence = "medium"
	}
	hashes, err := sourceSnapshot(db, sourcePaths)
	if err != nil {
		return "", err
	}
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	pathsJSON, _ := json.Marshal(sourcePaths)
	hashesJSON, _ := json.Marshal(hashes)
	_, err = db.Exec(`
		INSERT INTO architecture_notes
			(id, topic, body, source_paths, source_hashes, confidence, status, last_verified_at, created_at, updated_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?)`,
		id, topic, body, string(pathsJSON), string(hashesJSON), confidence, now, now, now, modelID,
	)
	return id, err
}

// Update changes note fields; returns false when not found.
func Update(db *sql.DB, idPrefix string, body *string, sourcePaths []string, confidence *string) (string, bool, error) {
	id, err := resolveNoteID(db, idPrefix)
	if err != nil {
		return "", false, err
	}
	note, err := Get(db, id)
	if err != nil {
		return "", false, err
	}
	if note == nil {
		return "", false, nil
	}
	newBody := note.Body
	if body != nil {
		newBody = *body
	}
	newSources := note.SourcePaths
	if sourcePaths != nil {
		newSources = sourcePaths
	}
	newConfidence := note.Confidence
	if confidence != nil {
		newConfidence = *confidence
	}
	hashes, err := sourceSnapshot(db, newSources)
	if err != nil {
		return "", false, err
	}
	now := storage.NowUTC()
	pathsJSON, _ := json.Marshal(newSources)
	hashesJSON, _ := json.Marshal(hashes)
	_, err = db.Exec(`
		UPDATE architecture_notes
		SET body=?, source_paths=?, source_hashes=?, confidence=?, status='active', updated_at=?, last_verified_at=?
		WHERE id=?`,
		newBody, string(pathsJSON), string(hashesJSON), newConfidence, now, now, id,
	)
	return id, true, err
}

// VerifyAllResult summarizes bulk architecture note verification.
type VerifyAllResult struct {
	Verified int `json:"verified"`
	Stale    int `json:"stale"`
}

// VerifyAll checks every active note and updates last_verified_at on fresh ones.
func VerifyAll(db *sql.DB) (VerifyAllResult, error) {
	notes, err := List(db, ListFilter{Status: "active"})
	if err != nil {
		return VerifyAllResult{}, err
	}
	var res VerifyAllResult
	for _, note := range notes {
		status, wasVerified, _, err := Verify(db, note.ID)
		if err != nil {
			return res, err
		}
		if wasVerified && status == "ok" {
			res.Verified++
		} else {
			res.Stale++
		}
	}
	return res, nil
}

// Verify checks staleness; updates last_verified_at when still fresh.
func Verify(db *sql.DB, idPrefix string) (status string, ok bool, id string, err error) {
	id, err = resolveNoteID(db, idPrefix)
	if err != nil {
		return "", false, "", err
	}
	note, err := Get(db, id)
	if err != nil {
		return "", false, id, err
	}
	if note == nil {
		return "", false, id, fmt.Errorf("note not found")
	}
	if note.Stale {
		return "stale", false, id, nil
	}
	now := storage.NowUTC()
	_, err = db.Exec(`UPDATE architecture_notes SET last_verified_at=?, updated_at=? WHERE id=?`, now, now, id)
	return "ok", true, id, err
}

// Get loads one note by id prefix.
func Get(db *sql.DB, idPrefix string) (*Note, error) {
	id, err := resolveNoteID(db, idPrefix)
	if err != nil {
		return nil, err
	}
	row := db.QueryRow(`SELECT id, topic, body, source_paths, source_hashes, confidence, status,
		last_verified_at, created_at, updated_at FROM architecture_notes WHERE id=?`, id)
	note, err := scanNote(row)
	if err != nil || note == nil {
		return note, err
	}
	fileHashes, _ := repoFileHashes(db)
	note.Stale = noteIsStale(*note, fileHashes)
	return note, nil
}

// List returns notes matching filters.
func List(db *sql.DB, f ListFilter) ([]Note, error) {
	fileHashes, _ := repoFileHashes(db)
	rows, err := db.Query(`SELECT id, topic, body, source_paths, source_hashes, confidence, status,
		last_verified_at, created_at, updated_at FROM architecture_notes ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		note, err := scanNoteFromRows(rows)
		if err != nil {
			return nil, err
		}
		note.Stale = noteIsStale(*note, fileHashes)
		if f.TopicSubstr != "" && !strings.Contains(strings.ToLower(note.Topic), strings.ToLower(f.TopicSubstr)) {
			continue
		}
		if f.TouchingPath != "" && !containsPath(note.SourcePaths, f.TouchingPath) {
			continue
		}
		if f.Status != "" && note.Status != f.Status {
			continue
		}
		if f.Stale && !note.Stale && note.Status != "stale" {
			continue
		}
		notes = append(notes, *note)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if f.Limit > 0 && len(notes) > f.Limit {
		notes = notes[:f.Limit]
	}
	return notes, nil
}

// CountStale returns active notes whose sources drifted.
func CountStale(db *sql.DB) (int, error) {
	notes, err := List(db, ListFilter{Status: "active"})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, n := range notes {
		if n.Stale {
			count++
		}
	}
	staleMarked := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM architecture_notes WHERE status='stale'`).Scan(&staleMarked)
	return count + staleMarked, nil
}

// RenderMarkdown produces human-readable architecture doc.
func RenderMarkdown(db *sql.DB) (string, error) {
	all, err := List(db, ListFilter{})
	if err != nil {
		return "", err
	}
	freshByTopic := map[string][]Note{}
	staleByTopic := map[string][]Note{}
	for _, note := range all {
		if note.Status != "active" {
			continue
		}
		if note.Stale {
			staleByTopic[note.Topic] = append(staleByTopic[note.Topic], note)
		} else {
			freshByTopic[note.Topic] = append(freshByTopic[note.Topic], note)
		}
	}

	var lines []string
	lines = append(lines, "# Architecture Notes")
	for _, topic := range sortedKeys(freshByTopic) {
		lines = append(lines, "## "+topic)
		for _, note := range freshByTopic[topic] {
			lines = append(lines, strings.TrimSpace(note.Body), "")
			lines = append(lines, "_sources: "+strings.Join(note.SourcePaths, ", ")+"_")
			lines = append(lines, "_verified: "+note.LastVerifiedAt+"_")
			lines = append(lines, fmt.Sprintf("_confidence: %s; status: %s_", note.Confidence, note.Status), "")
		}
	}
	if len(staleByTopic) > 0 {
		lines = append(lines, "## Stale notes (needs re-verification)")
		for _, topic := range sortedKeys(staleByTopic) {
			lines = append(lines, "### "+topic)
			for _, note := range staleByTopic[topic] {
				lines = append(lines, strings.TrimSpace(note.Body), "")
				lines = append(lines, "_sources: "+strings.Join(note.SourcePaths, ", ")+"_")
				lines = append(lines, "_last verified: "+note.LastVerifiedAt+"_")
				lines = append(lines, fmt.Sprintf("_confidence: %s; status: %s_", note.Confidence, note.Status))
				lines = append(lines, "_Source files may have changed. Run devdb arch verify to update._", "")
			}
		}
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func sourceSnapshot(db *sql.DB, sourcePaths []string) (map[string]string, error) {
	hashes := map[string]string{}
	for _, p := range sourcePaths {
		var hash sql.NullString
		err := db.QueryRow(`SELECT content_hash FROM repo_files WHERE path=?`, p).Scan(&hash)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("missing source path: %s", p)
		}
		if err != nil {
			return nil, err
		}
		if hash.Valid {
			hashes[p] = hash.String
		} else {
			hashes[p] = ""
		}
	}
	return hashes, nil
}

func repoFileHashes(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT path, content_hash FROM repo_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var path string
		var hash sql.NullString
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		if hash.Valid {
			out[path] = hash.String
		} else {
			out[path] = ""
		}
	}
	return out, rows.Err()
}

func noteIsStale(note Note, fileHashes map[string]string) bool {
	for _, path := range note.SourcePaths {
		current, ok := fileHashes[path]
		if !ok {
			return true
		}
		saved := note.SourceHashes[path]
		if current != saved {
			return true
		}
	}
	return false
}

func scanNote(row *sql.Row) (*Note, error) {
	var n Note
	var pathsJSON, hashesJSON string
	if err := row.Scan(&n.ID, &n.Topic, &n.Body, &pathsJSON, &hashesJSON, &n.Confidence, &n.Status,
		&n.LastVerifiedAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(pathsJSON), &n.SourcePaths)
	_ = json.Unmarshal([]byte(hashesJSON), &n.SourceHashes)
	return &n, nil
}

func scanNoteFromRows(rows *sql.Rows) (*Note, error) {
	var n Note
	var pathsJSON, hashesJSON string
	if err := rows.Scan(&n.ID, &n.Topic, &n.Body, &pathsJSON, &hashesJSON, &n.Confidence, &n.Status,
		&n.LastVerifiedAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(pathsJSON), &n.SourcePaths)
	_ = json.Unmarshal([]byte(hashesJSON), &n.SourceHashes)
	return &n, nil
}

func resolveNoteID(db *sql.DB, prefix string) (string, error) {
	rows, err := db.Query(`SELECT id FROM architecture_notes`)
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

func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string][]Note) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MissingSourceError indicates a source path is not in repo_files.
type MissingSourceError struct {
	Path string
}

func (e *MissingSourceError) Error() string {
	return "missing source path: " + e.Path
}
