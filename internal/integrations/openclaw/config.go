package openclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config identifies the OpenClaw workspace and the independent devdb target.
type Config struct {
	Workspace string `json:"workspace"`
	Repo      string `json:"repo"`
	JSON      bool   `json:"-"`
}

// ResolveConfig applies flag, environment, and home-directory defaults.
func ResolveConfig(workspace, repo string, jsonOutput bool) (Config, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = os.Getenv("OPENCLAW_WORKSPACE")
	}
	if strings.TrimSpace(workspace) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve home directory: %w", err)
		}
		workspace = filepath.Join(home, ".openclaw", "workspace")
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return Config{}, fmt.Errorf("resolve workspace: %w", err)
	}

	if strings.TrimSpace(repo) == "" {
		repo = os.Getenv("OPENCLAW_DEVDB_TARGET_REPO")
	}
	if strings.TrimSpace(repo) == "" {
		repo = workspace
	}
	repo, err = filepath.Abs(repo)
	if err != nil {
		return Config{}, fmt.Errorf("resolve target repository: %w", err)
	}

	return Config{Workspace: filepath.Clean(workspace), Repo: filepath.Clean(repo), JSON: jsonOutput}, nil
}
