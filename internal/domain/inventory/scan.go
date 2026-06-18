package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/git"
)

var (
	defaultIgnoredDirs = map[string]bool{
		".git": true, ".devdb": true, "__pycache__": true, ".venv": true,
		"node_modules": true, "dist": true, "build": true,
	}
	agentDocNames = map[string]bool{
		"agents.md": true, "claude.md": true, "agent.md": true, "copilot-instructions.md": true,
	}
	docExtensions = map[string]bool{".md": true, ".rst": true, ".txt": true}
	configExtensions = map[string]bool{
		".toml": true, ".json": true, ".yaml": true, ".yml": true,
		".ini": true, ".cfg": true, ".env": true, ".lock": true,
	}
	codeExtensions = map[string]bool{
		".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true, ".rs": true,
		".go": true, ".java": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true,
		".cc": true, ".rb": true, ".php": true, ".sh": true, ".bash": true, ".sql": true,
	}
	languageByExt = map[string]string{
		".py": "python", ".md": "markdown", ".rst": "rst", ".txt": "text",
		".toml": "toml", ".json": "json", ".yaml": "yaml", ".yml": "yaml",
		".ini": "ini", ".cfg": "ini", ".sh": "shell", ".bash": "shell", ".sql": "sql",
		".js": "javascript", ".ts": "typescript", ".tsx": "tsx", ".jsx": "jsx",
		".rs": "rust", ".go": "go", ".java": "java", ".c": "c", ".h": "c",
		".cpp": "cpp", ".hpp": "cpp", ".cc": "cpp", ".rb": "ruby", ".php": "php",
	}
)

// FileRecord is one scanned file row.
type FileRecord struct {
	Path        string `json:"path"`
	Language    string `json:"language,omitempty"`
	Kind        string `json:"kind"`
	Lines       *int   `json:"lines,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	SizeBytes   int    `json:"size_bytes"`
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isBinary(data []byte) bool {
	return strings.Contains(string(data), "\x00")
}

func languageFor(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	if lang, ok := languageByExt[ext]; ok {
		return lang
	}
	return ""
}

func kindFor(p string, binary bool) string {
	if binary {
		return "binary"
	}
	base := strings.ToLower(filepath.Base(p))
	parts := strings.Split(filepath.ToSlash(p), "/")
	partSet := make(map[string]bool, len(parts))
	for _, part := range parts {
		partSet[strings.ToLower(part)] = true
	}
	if agentDocNames[base] || partSet["claude"] || partSet["agents"] {
		return "agent_doc"
	}
	if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") || partSet["tests"] {
		return "test"
	}
	ext := strings.ToLower(filepath.Ext(p))
	if base == ".gitignore" || base == ".dockerignore" || configExtensions[ext] {
		return "config"
	}
	if base == "readme.md" || base == "changelog.md" || base == "license" || base == "license.md" ||
		docExtensions[ext] || partSet["docs"] {
		return "doc"
	}
	if ext == ".lock" {
		return "generated"
	}
	if codeExtensions[ext] {
		return "code"
	}
	return "other"
}

func isTransientArtifact(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "-journal") || strings.HasSuffix(lower, "-wal") || strings.HasSuffix(lower, "-shm") {
		return true
	}
	if strings.HasSuffix(lower, ".tmp") || strings.HasSuffix(lower, ".swp") ||
		strings.HasSuffix(lower, ".swx") || strings.HasSuffix(lower, "~") {
		return true
	}
	if strings.HasPrefix(lower, ".#") || strings.HasSuffix(lower, "#") {
		return true
	}
	return false
}

func pathMatchesGitignore(repoRoot, relPath string) bool {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		return false
	}
	rel := filepath.ToSlash(relPath)
	base := filepath.Base(rel)
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if strings.HasSuffix(line, "/") {
			prefix := strings.TrimSuffix(line, "/")
			if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
				return true
			}
		} else if strings.Contains(line, "/") {
			if matched, _ := path.Match(line, rel); matched {
				return true
			}
		} else {
			if matched, _ := path.Match(line, base); matched {
				return true
			}
		}
	}
	return false
}

func isScanable(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// DiscoverFiles finds scannable files under repoRoot.
func DiscoverFiles(repoRoot string, paths []string, gitAware bool) ([]string, error) {
	if gitAware {
		relPaths, err := git.LsFiles(repoRoot, paths)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, rel := range relPaths {
			abs := filepath.Join(repoRoot, rel)
			if isScanable(abs) {
				out = append(out, rel)
			}
		}
		return out, nil
	}

	roots := paths
	if len(roots) == 0 {
		roots = []string{"."}
	}
	seen := map[string]bool{}
	var discovered []string
	for _, root := range roots {
		absRoot := filepath.Join(repoRoot, root)
		info, err := os.Stat(absRoot)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			rel, _ := filepath.Rel(repoRoot, absRoot)
			discovered = append(discovered, filepath.ToSlash(rel))
			continue
		}
		_ = filepath.WalkDir(absRoot, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if defaultIgnoredDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if isTransientArtifact(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(repoRoot, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			for _, part := range strings.Split(rel, "/") {
				if defaultIgnoredDirs[part] {
					return nil
				}
			}
			if pathMatchesGitignore(repoRoot, rel) {
				return nil
			}
			discovered = append(discovered, rel)
			return nil
		})
	}
	for _, p := range discovered {
		seen[p] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	// simple sort
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// ScanFile reads and classifies one file.
func ScanFile(repoRoot, relPath string) (*FileRecord, error) {
	if isTransientArtifact(filepath.Base(relPath)) {
		return nil, nil
	}
	abs := filepath.Join(repoRoot, relPath)
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil
	}
	binary := isBinary(data)
	size := len(data)
	rec := &FileRecord{
		Path:      relPath,
		Language:  languageFor(relPath),
		Kind:      kindFor(relPath, binary),
		SizeBytes: size,
	}
	if binary {
		rec.Kind = "binary"
		return rec, nil
	}
	lines := len(strings.Split(string(data), "\n"))
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		// split gives at least 1 for non-empty; line count is correct
	}
	rec.Lines = &lines
	rec.ContentHash = sha256Bytes(data)
	return rec, nil
}

// ScanInventory discovers and scans files.
func ScanInventory(repoRoot string, paths []string, gitAware bool) ([]FileRecord, error) {
	relPaths, err := DiscoverFiles(repoRoot, paths, gitAware)
	if err != nil {
		return nil, err
	}
	var records []FileRecord
	for _, rel := range relPaths {
		rec, err := ScanFile(repoRoot, rel)
		if err != nil || rec == nil {
			continue
		}
		records = append(records, *rec)
	}
	return records, nil
}
