package hub

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// UnregisteredRepo is a directory holding a .devdb/development.db that the
// hub registry does not know about.
type UnregisteredRepo struct {
	Root   string `json:"root"`
	DBPath string `json:"db_path"`
	// SuggestedAlias mirrors AutoRegister's aliasing (directory base name).
	SuggestedAlias string `json:"suggested_alias"`
}

// AutoRegisterResult reports what an auto-register walk found and wrote.
type AutoRegisterResult struct {
	Discovered []string        `json:"discovered"`
	Registered []string        `json:"registered"`
	Conflicts  []aliasConflict `json:"alias_conflicts,omitempty"`
}

// aliasConflict records an alias that was already bound to a different root, so
// the walk refused to repoint it.
type aliasConflict struct {
	Alias   string `json:"alias"`
	Kept    string `json:"kept"`
	Skipped string `json:"skipped"`
}

// FindUnregistered walks each scope for .devdb/development.db files and
// returns the ones whose repo root is not present in the registry. This is
// the doctor-side complement to `hub register --auto`: audit/dashboards only
// see registered repos, so an unregistered ledger is invisible debt. Walk
// rules match AutoRegister (.git/node_modules/vendor skipped).
func FindUnregistered(scopes []string, registry string) ([]UnregisteredRepo, error) {
	projects, err := ReadRegistry(registry)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, p := range projects {
		if abs, err := filepath.Abs(expandHome(p.Root)); err == nil {
			known[abs] = true
		}
		known[p.Root] = true
	}

	seen := map[string]bool{}
	var out []UnregisteredRepo
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		absScope, err := filepath.Abs(expandHome(scope))
		if err != nil {
			return nil, err
		}
		walkErr := filepath.WalkDir(absScope, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return nil // unreadable dir: skip, keep scanning
				}
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == "vendor" || name == ".devdb-archive" {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() != "development.db" || filepath.Base(filepath.Dir(path)) != ".devdb" {
				return nil
			}
			repoRoot := filepath.Dir(filepath.Dir(path))
			if seen[repoRoot] {
				return nil
			}
			seen[repoRoot] = true
			if !known[repoRoot] {
				out = append(out, UnregisteredRepo{
					Root:           repoRoot,
					DBPath:         path,
					SuggestedAlias: filepath.Base(repoRoot),
				})
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out, nil
}
