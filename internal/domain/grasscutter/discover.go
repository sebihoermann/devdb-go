package grasscutter

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/git"
)

// Candidate is one heuristic detection result.
type Candidate struct {
	Principle      string `json:"principle"`
	FilePath       string `json:"file_path,omitempty"`
	LineStart      *int   `json:"line_start,omitempty"`
	LineEnd        *int   `json:"line_end,omitempty"`
	Title          string `json:"title"`
	Recommendation string `json:"recommendation"`
	Severity       string `json:"severity"`
	Confidence     string `json:"confidence"`
	Effort         string `json:"effort"`
}

var (
	allPrinciples = []string{"dead", "inlinable", "sprawl", "duplication", "staleness"}
	todoRE        = regexp.MustCompile(`\b(TODO|FIXME|XXX)\b`)
	severityRank  = map[string]int{"critical": 6, "high": 5, "med": 4, "low": 3, "info": 2}
)

// Discover scans indexed Python files and returns heuristic candidates.
func Discover(repoRoot string, db *sql.DB, scopePaths, principles []string) ([]Candidate, map[string]int, error) {
	wanted := map[string]bool{}
	if len(principles) == 0 {
		for _, p := range allPrinciples {
			wanted[p] = true
		}
	} else {
		for _, p := range principles {
			wanted[p] = true
		}
	}

	files, err := pythonFiles(db, scopePaths)
	if err != nil {
		return nil, nil, err
	}

	filesData := map[string]*pythonFile{}
	for _, path := range files {
		parsed, err := readPython(repoRoot, path)
		if err != nil {
			return nil, nil, err
		}
		if parsed != nil {
			filesData[path] = parsed
		}
	}

	counts := map[string]int{}
	var candidates []Candidate

	if wanted["dead"] || wanted["inlinable"] {
		defs := map[string][]funcDef{}
		refCounts := map[string]int{}
		for path, pf := range filesData {
			for _, node := range walkFunctions(pf.mod) {
				end := functionEndLine(node)
				defs[string(node.Name)] = append(defs[string(node.Name)], funcDef{
					path: path, name: string(node.Name),
					line: node.GetLineno(), endLine: end,
					bodySig: functionSignature(node), node: node,
				})
			}
			for name, n := range referenceCounts(pf.mod) {
				refCounts[name] += n
			}
		}
		for name, nodes := range defs {
			totalRefs := refCounts[name]
			path, node := nodes[0].path, nodes[0]
			if wanted["dead"] && totalRefs == 0 {
				if deadDefExempt(path, name) {
					continue
				}
				candidates = append(candidates, newCandidate(
					"dead", path, node.line, node.endLine,
					"remove dead "+name,
					"Delete the function or move it behind a clear use site.",
					"high", "medium", "small",
				))
				counts["dead"]++
			} else if wanted["inlinable"] && totalRefs == 1 {
				candidates = append(candidates, newCandidate(
					"inlinable", path, node.line, node.endLine,
					"inline once-used "+name,
					"Inline the function at its single call site if that keeps the code clearer.",
					"low", "medium", "small",
				))
				counts["inlinable"]++
			}
		}
	}

	if wanted["sprawl"] {
		for _, path := range files {
			var lines sql.NullInt64
			if err := db.QueryRow(`SELECT lines FROM repo_files WHERE path=?`, path).Scan(&lines); err != nil {
				return nil, nil, err
			}
			if lines.Valid && lines.Int64 > 500 {
				candidates = append(candidates, newCandidate(
					"sprawl", path, 0, 0,
					path+" is too large",
					"Split the file into smaller modules with narrower responsibilities.",
					"critical", "high", "large",
				))
				counts["sprawl"]++
			}
		}
	}

	if wanted["duplication"] {
		seen := map[string]funcDef{}
		for path, pf := range filesData {
			for _, node := range walkFunctions(pf.mod) {
				sig := functionBodyText(pf.text, node)
				fd := funcDef{
					path: path, name: string(node.Name),
					line: node.GetLineno(), endLine: functionEndLine(node),
					bodySig: sig, node: node,
				}
				if first, ok := seen[sig]; ok {
					if first.path != path {
						candidates = append(candidates, newCandidate(
							"duplication", first.path, first.line, first.endLine,
							"duplicate "+first.name,
							"Extract the shared body into one helper and call it from both places.",
							"med", "medium", "small",
						))
						counts["duplication"]++
						delete(seen, sig)
					}
				} else {
					seen[sig] = fd
				}
			}
		}
	}

	if wanted["staleness"] {
		blameCache := map[string]map[int]int{}
		for path, pf := range filesData {
			lines := strings.Split(pf.text, "\n")
			for idx, line := range lines {
				if !todoRE.MatchString(line) {
					continue
				}
				lineNo := idx + 1
				age := blameAgeDays(repoRoot, path, lineNo, blameCache)
				if age == nil || *age >= 90 {
					candidates = append(candidates, newCandidate(
						"staleness", path, lineNo, lineNo,
						"stale comment in "+path,
						"Either remove the stale comment or turn it into a tracked finding with a concrete owner.",
						"low", "medium", "small",
					))
					counts["staleness"]++
				}
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		si := severityRank[candidates[i].Severity]
		sj := severityRank[candidates[j].Severity]
		if si != sj {
			return si > sj
		}
		if candidates[i].Principle != candidates[j].Principle {
			return candidates[i].Principle > candidates[j].Principle
		}
		if candidates[i].FilePath != candidates[j].FilePath {
			return candidates[i].FilePath > candidates[j].FilePath
		}
		li, lj := 0, 0
		if candidates[i].LineStart != nil {
			li = *candidates[i].LineStart
		}
		if candidates[j].LineStart != nil {
			lj = *candidates[j].LineStart
		}
		return li > lj
	})

	return candidates, counts, nil
}

func pythonFiles(db *sql.DB, scopePaths []string) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM repo_files WHERE path LIKE '%.py' ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(scopePaths) == 0 || (len(scopePaths) == 1 && scopePaths[0] == ".") {
		return paths, nil
	}
	var filtered []string
	for _, path := range paths {
		if pathInScope(path, scopePaths) {
			filtered = append(filtered, path)
		}
	}
	return filtered, nil
}

func pathInScope(path string, scope []string) bool {
	path = filepath.ToSlash(path)
	for _, item := range scope {
		item = strings.TrimSuffix(filepath.ToSlash(item), "/")
		if item == "." || item == "" {
			return true
		}
		if path == item || strings.HasPrefix(path, item+"/") {
			return true
		}
	}
	return false
}

func blameAgeDays(repoRoot, path string, lineNo int, cache map[string]map[int]int) *int {
	if _, ok := cache[path]; !ok {
		cache[path] = git.BlameLineAges(repoRoot, path)
	}
	age, ok := cache[path][lineNo]
	if !ok {
		return nil
	}
	return &age
}

func newCandidate(principle, filePath string, lineStart, lineEnd int, title, recommendation, severity, confidence, effort string) Candidate {
	c := Candidate{
		Principle: principle, FilePath: filePath,
		Title: title, Recommendation: recommendation,
		Severity: severity, Confidence: confidence, Effort: effort,
	}
	if lineStart > 0 {
		c.LineStart = &lineStart
	}
	if lineEnd > 0 {
		c.LineEnd = &lineEnd
	}
	return c
}

// capCandidates limits findings per file before persistence.
func capCandidates(candidates []Candidate, cap int) []Candidate {
	perFile := map[string]int{}
	var kept []Candidate
	for _, c := range candidates {
		key := c.FilePath
		if perFile[key] >= cap {
			continue
		}
		perFile[key]++
		kept = append(kept, c)
	}
	return kept
}

func formatSummary(candidates []Candidate, counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	return fmt.Sprintf("found %d candidates: %s", len(candidates), strings.Join(parts, " "))
}
