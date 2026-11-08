package cli

import (
	"strings"
	"testing"
)

func TestAddDuplicateGuard(t *testing.T) {
	e := newInProcessEnv(t)
	e.run("inventory", "scan")
	runID := e.run("review", "start", "--paths", ".")

	cases := []struct {
		name string
		kind string
		args []string
	}{
		{"feedback", "duplicate feedback", []string{"feedback", "add", "--role", "model", "guard note"}},
		{"goal", "duplicate goal", []string{"goal", "add", "Guard goal", "--kind", "goal"}},
		{"task", "duplicate task", []string{"task", "add", "Guard task"}},
		{"reminder", "duplicate reminder", []string{"reminder", "add", "Guard reminder", "--due", "2099-01-01T00:00:00Z"}},
		{"arch", "duplicate architecture note", []string{"arch", "add", "guard-topic", "--body", "Guard architecture body.", "--source", "main.go"}},
		{"review", "duplicate review finding", []string{"review", "finding", "--run", runID, "--file", "main.go", "--principle", "kiss", "--title", "Guard finding", "--recommendation", "fix it"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e.run(tc.args...)
			_, stderr, code := e.runAllow(tc.args...)
			if code != ExitInvalidValue {
				t.Fatalf("second add code=%d, want %d (stderr=%q)", code, ExitInvalidValue, stderr)
			}
			if !strings.Contains(stderr, tc.kind) || !strings.Contains(stderr, "already exists as") {
				t.Fatalf("duplicate warning missing from stderr=%q", stderr)
			}
			e.run(append(append([]string{}, tc.args...), "--allow-duplicate")...)
		})
	}
}

func TestDuplicateGuardNormalizesCaseAndWhitespace(t *testing.T) {
	e := newInProcessEnv(t)
	e.run("feedback", "add", "--role", "model", "Fix  the parser")
	_, stderr, code := e.runAllow("feedback", "add", "--role", "user", "fix the   parser")
	if code != ExitInvalidValue {
		t.Fatalf("code=%d, want %d (stderr=%q)", code, ExitInvalidValue, stderr)
	}
}

func TestDuplicateGuardIgnoresClosedRows(t *testing.T) {
	e := newInProcessEnv(t)

	feedbackID := e.run("feedback", "add", "--role", "model", "closed note")
	e.run("feedback", "close", feedbackID)
	e.run("feedback", "add", "--role", "model", "closed note")

	taskID := e.run("task", "add", "done task")
	e.run("task", "done", taskID)
	e.run("task", "add", "done task")

	reminderID := e.run("reminder", "add", "dismissed reminder")
	e.run("reminder", "dismiss", reminderID)
	e.run("reminder", "add", "dismissed reminder")

	goalID := e.run("goal", "add", "done goal")
	e.run("goal", "set", goalID, "done")
	e.run("goal", "add", "done goal")
}
