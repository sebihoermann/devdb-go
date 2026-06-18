package cli

import (
	"path/filepath"
	"testing"
)

func TestCoverageSweepJSONReads(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	reads := [][]string{
		{"status"}, {"quality"}, {"report"}, {"resume"}, {"doctor"},
		{"feedback", "list"}, {"goal", "list"}, {"feature", "list"},
		{"task", "list"}, {"reminder", "list"}, {"plan", "list"},
		{"arch", "list"}, {"review", "list"}, {"inventory", "loc"},
		{"analytics", "missed"}, {"analytics", "summary"},
		{"archive", "list"}, {"approval", "list"},
	}
	for _, args := range reads {
		e.runOut(append([]string{"--json"}, args...)...)
		e.runOut(append([]string{"--json", "--verbose"}, args...)...)
	}
}

func TestCoverageSweepWriteJSON(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")

	writes := [][]string{
		{"feedback", "add", "--role", "model", "sweep"},
		{"goal", "add", "G", "--kind", "goal"},
		{"feature", "add", "F"},
		{"task", "add", "T"},
		{"reminder", "add", "R", "--due", "2099-01-01T00:00:00Z"},
		{"plan", "create", "P", "--slug", "sweep-plan"},
		{"arch", "add", "sweep-topic", "--body", "note body here.", "--source", "main.go"},
	}
	for _, args := range writes {
		e.run(append([]string{"--json"}, args...)...)
	}
	planID := e.run("plan", "create", "P2", "--slug", "sweep2")
	msID := e.run("plan", "milestone", "add", "M1", "--plan", planID)
	itemID := e.run("plan", "item", "add", "I", "--plan", planID, "--milestone", msID)
	e.run("--json", "plan", "item", "start", itemID)
	e.run("--json", "plan", "item", "pause", itemID, "--note", "ctx")
	e.run("--json", "plan", "item", "close", itemID, "--evidence", "done")
}

func TestCoverageSweepQualityVerify(t *testing.T) {
	e := newInProcessEnv(t)
	initGitRepo(t, e.repo)
	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", ".")
	findingID := e.run("review", "finding", "--run", runID, "--file", "main.go", "--principle", "kiss",
		"--title", "sweep", "--recommendation", "fix", "--severity", "low")
	e.run("--json", "review", "resolve", findingID, "--evidence", "ok")
	e.run("--json", "review", "finish", runID, "--summary", "done")
	e.run("--json", "review", "report", runID)
	e.run("--json", "review", "principles", "--tier", "extended")

	vRun := e.run("verify", "record", "go test ./...", "--scope", ".", "--status", "passed",
		"--finished", "--git-sha", "abc", "--exit-code", "0", "--notes", "ok")
	for _, view := range []string{"summary", "inputs", "failures"} {
		e.runOut("--json", "verify", "show", vRun, "--view", view)
	}
	e.runOut("--json", "verify", "query", "--command", "go test ./...", "--scope", ".")
}

func TestCoverageSweepHubAndHygiene(t *testing.T) {
	e := newInProcessEnv(t)
	dir := t.TempDir()
	metaDB := filepath.Join(dir, "meta.db")
	registry := filepath.Join(dir, "reg.json")
	hub := hubBase(t, metaDB, registry)
	runCLIOut(t, append(hub, "register", e.repo, "--alias", "sweep")...)
	runCLIOut(t, append(hub, "--json", "sync")...)
	runCLIOut(t, append(hub, "--json", "list", "--refresh")...)
	runCLIOut(t, append(hub, "--json", "dashboard", "--view", "quality")...)
	runCLIOut(t, append(hub, "--json", "doctor")...)
	runCLIOut(t, append(hub, "--json", "across", "open-debt")...)

	e.runOut("--json", "archive", "gc", "--dry-run")
	e.runOut("--json", "archive", "run", "--dry-run", "--yes")
	e.runOut("--json", "doctor", "hygiene")
}
