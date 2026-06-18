package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Commit is one entry from git log.
type Commit struct {
	SHA     string
	Author  string
	Date    string
	Subject string
	Body    string
}

// Log returns commit history for branch (newest first), up to limit entries.
func Log(dir, branch string, limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = 200
	}
	sep := "\x1f"
	end := "\x1e"
	fmtStr := strings.Join([]string{"%H", "%an", "%aI", "%s", "%b"}, sep) + end
	cmd := exec.Command("git", "-C", dir, "log", branch,
		fmt.Sprintf("-n%d", limit), fmt.Sprintf("--pretty=format:%s", fmtStr))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", branch, err)
	}
	var commits []Commit
	for _, chunk := range strings.Split(string(out), end) {
		chunk = strings.Trim(chunk, "\n")
		if chunk == "" {
			continue
		}
		parts := strings.Split(chunk, sep)
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, Commit{
			SHA:     parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Subject: parts[3],
			Body:    parts[4],
		})
	}
	return commits, nil
}
