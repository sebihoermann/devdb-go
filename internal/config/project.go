package config

import (
	"os"
	"path/filepath"
)

const DevDBDirName = ".devdb"
const DefaultDBName = "development.db"

// Project holds resolved paths for a devdb workspace.
type Project struct {
	RepoRoot string
	DevDBDir string
	DBPath   string
}

// Resolve finds the repository root and database path.
// repoFlag overrides auto-discovery when non-empty.
func Resolve(repoFlag, dbFlag string) (Project, error) {
	root, err := resolveRepoRoot(repoFlag)
	if err != nil {
		return Project{}, err
	}
	devdbDir := filepath.Join(root, DevDBDirName)
	dbPath := dbFlag
	if dbPath == "" {
		dbPath = filepath.Join(devdbDir, DefaultDBName)
	}
	return Project{
		RepoRoot: root,
		DevDBDir: devdbDir,
		DBPath:   dbPath,
	}, nil
}

func resolveRepoRoot(repoFlag string) (string, error) {
	if repoFlag != "" {
		abs, err := filepath.Abs(repoFlag)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, DevDBDirName)); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
		dir = parent
	}
}
