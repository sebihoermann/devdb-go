package inventory

import (
	"database/sql"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

var externalNamespaceRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ExternalSource is a file owned outside the target repository but tracked by
// a provider adapter for source-hash freshness.
type ExternalSource struct {
	Path        string
	Language    string
	Lines       int
	ContentHash string
	SizeBytes   int64
}

// ExternalSyncResult summarizes a namespaced external-source refresh.
type ExternalSyncResult struct {
	RunID        string            `json:"run_id"`
	Paths        map[string]string `json:"paths"`
	FilesAdded   int               `json:"files_added"`
	FilesChanged int               `json:"files_changed"`
	FilesRemoved int               `json:"files_removed"`
}

// SyncExternalSources replaces one provider namespace without disturbing
// repository inventory or other external namespaces.
func SyncExternalSources(db *sql.DB, namespace string, sources []ExternalSource, modelID string) (ExternalSyncResult, error) {
	if !externalNamespaceRE.MatchString(namespace) {
		return ExternalSyncResult{}, fmt.Errorf("invalid external source namespace %q", namespace)
	}
	prefix := "external/" + namespace + "/"
	normalized := make([]ExternalSource, 0, len(sources))
	paths := make(map[string]string, len(sources))
	seen := map[string]bool{}
	for _, source := range sources {
		rel, err := normalizeExternalPath(source.Path)
		if err != nil {
			return ExternalSyncResult{}, err
		}
		if seen[rel] {
			return ExternalSyncResult{}, fmt.Errorf("duplicate external source path %q", rel)
		}
		seen[rel] = true
		source.Path = rel
		normalized = append(normalized, source)
		paths[rel] = prefix + rel
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path })

	existing := map[string]string{}
	rows, err := db.Query(`SELECT path, COALESCE(content_hash,'') FROM repo_files WHERE kind='external' AND path LIKE ?`, prefix+"%")
	if err != nil {
		return ExternalSyncResult{}, err
	}
	for rows.Next() {
		var inventoryPath, hash string
		if err := rows.Scan(&inventoryPath, &hash); err != nil {
			rows.Close()
			return ExternalSyncResult{}, err
		}
		existing[inventoryPath] = hash
	}
	if err := rows.Close(); err != nil {
		return ExternalSyncResult{}, err
	}

	runID, err := storage.NewID()
	if err != nil {
		return ExternalSyncResult{}, err
	}
	now := storage.NowUTC()
	result := ExternalSyncResult{RunID: runID, Paths: paths}
	err = storage.WithTx(db, func(tx *sql.Tx) error {
		for _, source := range normalized {
			inventoryPath := paths[source.Path]
			oldHash, exists := existing[inventoryPath]
			if !exists {
				result.FilesAdded++
			} else if oldHash != source.ContentHash {
				result.FilesChanged++
			}
			if _, err := tx.Exec(`
				INSERT INTO repo_files(path, language, kind, lines, content_hash, size_bytes, last_seen_at, last_scan_run_id)
				VALUES (?, ?, 'external', ?, ?, ?, ?, ?)
				ON CONFLICT(path) DO UPDATE SET language=excluded.language, kind='external', lines=excluded.lines,
					content_hash=excluded.content_hash, size_bytes=excluded.size_bytes,
					last_seen_at=excluded.last_seen_at, last_scan_run_id=excluded.last_scan_run_id`,
				inventoryPath, nullStr(source.Language), source.Lines, nullStr(source.ContentHash), source.SizeBytes, now, runID,
			); err != nil {
				return err
			}
			delete(existing, inventoryPath)
		}
		for inventoryPath := range existing {
			if _, err := tx.Exec(`DELETE FROM repo_files WHERE path=? AND kind='external'`, inventoryPath); err != nil {
				return err
			}
			result.FilesRemoved++
		}
		_, err := tx.Exec(`
			INSERT INTO scan_runs(id, started_at, finished_at, files_seen, files_added, files_changed, files_removed, model_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			runID, now, now, len(normalized), result.FilesAdded, result.FilesChanged, result.FilesRemoved, modelID,
		)
		return err
	})
	return result, err
}

func normalizeExternalPath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	clean := path.Clean(value)
	if clean == "." || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid external source path %q", value)
	}
	return clean, nil
}
