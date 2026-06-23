package openclaw

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var bootstrapDescriptions = []struct {
	Path        string
	Description string
}{
	{"AGENTS.md", "Operating instructions, session startup rules, and boundaries"},
	{"SOUL.md", "Persona, tone, and behavioral boundaries"},
	{"USER.md", "User context and interaction preferences"},
	{"IDENTITY.md", "Agent name and identity"},
	{"TOOLS.md", "Local tool conventions and notes"},
	{"HEARTBEAT.md", "Heartbeat poll checklist"},
	{"BOOT.md", "Gateway-startup checklist"},
	{"BOOTSTRAP.md", "One-time first-run ritual"},
	{"MEMORY.md", "Curated long-term memory"},
}

// MemoryFile is one supported OpenClaw memory source.
type MemoryFile struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Exists      bool   `json:"exists"`
	Description string `json:"description"`
}

// Discovery is the deterministic list result for a workspace.
type Discovery struct {
	Workspace string       `json:"workspace"`
	Files     []MemoryFile `json:"files"`
}

// Discover lists supported memory files without following symlinks.
func Discover(workspace string) (Discovery, error) {
	info, err := os.Stat(workspace)
	if err != nil {
		return Discovery{}, fmt.Errorf("workspace unavailable: %w", err)
	}
	if !info.IsDir() {
		return Discovery{}, fmt.Errorf("workspace is not a directory: %s", workspace)
	}

	result := Discovery{Workspace: workspace}
	for _, entry := range bootstrapDescriptions {
		exists := safeRegularFile(filepath.Join(workspace, entry.Path))
		result.Files = append(result.Files, MemoryFile{
			Path: entry.Path, Kind: "bootstrap", Exists: exists, Description: entry.Description,
		})
	}

	daily, err := filepath.Glob(filepath.Join(workspace, "memory", "????-??-??.md"))
	if err != nil {
		return Discovery{}, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(daily)))
	for _, path := range daily {
		if !safeRegularFile(path) {
			continue
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		result.Files = append(result.Files, MemoryFile{
			Path: filepath.ToSlash(rel), Kind: "daily", Exists: true, Description: firstHeading(path),
		})
	}
	return result, nil
}

func safeRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func firstHeading(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "(no heading)"
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		trimmed := strings.TrimLeft(line, "#")
		if len(trimmed) == len(line) || (trimmed != "" && trimmed[0] != ' ') {
			continue
		}
		if heading := strings.TrimSpace(trimmed); heading != "" {
			return heading
		}
	}
	return "(no heading)"
}

func newListCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List OpenClaw memory files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			config, err := ResolveConfig(opts.workspace, opts.repo, opts.json)
			if err != nil {
				return err
			}
			result, err := Discover(config.Workspace)
			if err != nil {
				return err
			}
			if config.JSON {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetEscapeHTML(false)
				return encoder.Encode(result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "workspace: %s\n", result.Workspace)
			for _, file := range result.Files {
				state := "missing"
				if file.Exists {
					state = "present"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-9s %s\n", file.Path, state, file.Description)
			}
			return nil
		},
	}
}
