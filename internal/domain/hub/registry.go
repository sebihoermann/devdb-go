package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RegisteredProject is one federation registry entry.
type RegisteredProject struct {
	Alias   string `json:"alias"`
	Root    string `json:"root"`
	DBPath  string `json:"db_path"`
	Exists  bool   `json:"exists"`
}

// ReadRegistry parses ~/.devdb-projects.
func ReadRegistry(path string) ([]RegisteredProject, error) {
	path = ResolveRegistry(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []RegisteredProject
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		root, err := filepath.Abs(expandHome(parts[0]))
		if err != nil {
			return nil, err
		}
		alias := defaultAlias(root)
		if len(parts) > 1 {
			alias = parts[1]
		}
		dbPath := filepath.Join(root, ".devdb", "development.db")
		_, statErr := os.Stat(dbPath)
		out = append(out, RegisteredProject{
			Alias:  alias,
			Root:   root,
			DBPath: dbPath,
			Exists: statErr == nil,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out, nil
}

// WriteRegistry persists registry entries sorted by alias.
func WriteRegistry(path string, projects []RegisteredProject) error {
	path = ResolveRegistry(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	sorted := append([]RegisteredProject(nil), projects...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Alias < sorted[j].Alias })
	var lines []string
	for _, p := range sorted {
		lines = append(lines, fmt.Sprintf("%s %s", p.Root, p.Alias))
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func defaultAlias(root string) string {
	name := filepath.Base(root)
	name = strings.ReplaceAll(name, " ", "-")
	if name == "" {
		return "project"
	}
	return name
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
