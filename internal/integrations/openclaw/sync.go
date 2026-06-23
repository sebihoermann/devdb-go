package openclaw

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/spf13/cobra"
)

var topicPartRE = regexp.MustCompile(`[^a-z0-9]+`)

// SyncResult summarizes an OpenClaw memory synchronization.
type SyncResult struct {
	Namespace      string `json:"namespace"`
	Files          int    `json:"files"`
	NotesCreated   int    `json:"notes_created"`
	NotesUpdated   int    `json:"notes_updated"`
	SourcesRemoved int    `json:"sources_removed"`
}

// Sync indexes current workspace memory files and upserts architecture notes.
func Sync(db *sql.DB, workspace, modelID string) (SyncResult, error) {
	discovery, err := Discover(workspace)
	if err != nil {
		return SyncResult{}, err
	}
	var sources []inventory.ExternalSource
	for _, file := range discovery.Files {
		if !file.Exists {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(file.Path)))
		if err != nil {
			return SyncResult{}, fmt.Errorf("read %s: %w", file.Path, err)
		}
		hash := sha256.Sum256(data)
		lines := 0
		if len(data) > 0 {
			lines = strings.Count(string(data), "\n")
			if data[len(data)-1] != '\n' {
				lines++
			}
		}
		sources = append(sources, inventory.ExternalSource{
			Path: file.Path, Language: "markdown", Lines: lines,
			ContentHash: hex.EncodeToString(hash[:]), SizeBytes: int64(len(data)),
		})
	}
	namespace := workspaceNamespace(workspace)
	external, err := inventory.SyncExternalSources(db, namespace, sources, modelID)
	if err != nil {
		return SyncResult{}, err
	}

	notes, err := architecture.List(db, architecture.ListFilter{})
	if err != nil {
		return SyncResult{}, err
	}
	bySource := map[string]architecture.Note{}
	for _, note := range notes {
		for _, source := range note.SourcePaths {
			if strings.HasPrefix(source, "external/"+namespace+"/") {
				bySource[source] = note
			}
		}
	}

	result := SyncResult{Namespace: namespace, Files: len(sources), SourcesRemoved: external.FilesRemoved}
	for _, source := range sources {
		inventoryPath := external.Paths[source.Path]
		body := fmt.Sprintf("%s is an OpenClaw memory source indexed from %s. The canonical content remains Markdown in the workspace; this note tracks source freshness and resolves as %s.", source.Path, workspace, MemoryRef(source.Path, ""))
		if note, ok := bySource[inventoryPath]; ok {
			if _, found, err := architecture.Update(db, note.ID, nil, []string{inventoryPath}, nil); err != nil {
				return SyncResult{}, err
			} else if !found {
				return SyncResult{}, fmt.Errorf("architecture note disappeared during sync: %s", note.ID)
			}
			result.NotesUpdated++
			continue
		}
		if _, err := architecture.Add(db, memoryTopic(source.Path), body, []string{inventoryPath}, "medium", modelID); err != nil {
			return SyncResult{}, err
		}
		result.NotesCreated++
	}
	return result, nil
}

// MemoryRef returns the canonical opaque reference for a workspace-relative path.
func MemoryRef(relPath, fragment string) string {
	ref := "openclaw:" + filepath.ToSlash(relPath)
	if fragment != "" {
		ref += "#" + fragment
	}
	return ref
}

func workspaceNamespace(workspace string) string {
	hash := sha256.Sum256([]byte(filepath.Clean(workspace)))
	return "openclaw-" + hex.EncodeToString(hash[:6])
}

func memoryTopic(relPath string) string {
	part := strings.Trim(topicPartRE.ReplaceAllString(strings.ToLower(strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))), "-"), "-")
	if len(part) > 18 {
		part = part[:18]
	}
	if part == "" {
		part = "source"
	}
	hash := sha256.Sum256([]byte(filepath.ToSlash(relPath)))
	return "openclaw-" + part + "-" + hex.EncodeToString(hash[:4])
}

func newSyncCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Synchronize memory sources into devdb",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(opts, true)
			if err != nil {
				return err
			}
			defer runtime.close()
			result, err := Sync(runtime.app.DB, runtime.config.Workspace, runtime.app.ModelID)
			if err != nil {
				return err
			}
			if runtime.config.JSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "files=%d created=%d updated=%d removed=%d\n", result.Files, result.NotesCreated, result.NotesUpdated, result.SourcesRemoved)
			return nil
		},
	}
}
