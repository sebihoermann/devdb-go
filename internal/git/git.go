package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// HeadSHA returns the current commit SHA or empty string when not in a git repo.
func HeadSHA(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Branch returns the current branch name.
func Branch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// AheadBehind returns commits ahead/behind upstream when tracking is configured.
func AheadBehind(dir string) (ahead, behind int) {
	cmd := exec.Command("git", "-C", dir, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0
	}
	fmt.Sscanf(parts[0], "%d", &behind)
	fmt.Sscanf(parts[1], "%d", &ahead)
	return ahead, behind
}

// IsDirty reports whether the worktree has uncommitted changes.
func IsDirty(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// LsFiles returns tracked file paths relative to dir (git ls-files).
func LsFiles(dir string, paths []string) ([]string, error) {
	args := []string{"-C", dir, "ls-files", "-z"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	raw := strings.Split(string(out), "\x00")
	var files []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			files = append(files, p)
		}
	}
	return files, nil
}

// BlameLineAges maps 1-based line numbers to author age in days from git blame porcelain output.
func BlameLineAges(dir, path string) map[int]int {
	cmd := exec.Command("git", "-C", dir, "blame", "--line-porcelain", path)
	out, err := cmd.Output()
	if err != nil {
		return map[int]int{}
	}
	now := time.Now().UTC().Unix()
	ages := map[int]int{}
	var currentTS int64
	lineNo := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "author-time ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				currentTS, _ = strconv.ParseInt(fields[1], 10, 64)
			} else {
				currentTS = 0
			}
			continue
		}
		if strings.HasPrefix(line, "\t") {
			lineNo++
			if currentTS > 0 {
				ages[lineNo] = int((now - currentTS) / 86400)
			}
		}
	}
	return ages
}

// DiffNameOnly returns paths changed between ref and the working tree.
func DiffNameOnly(dir, ref string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir, "diff", "--name-only", ref, "--")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}
