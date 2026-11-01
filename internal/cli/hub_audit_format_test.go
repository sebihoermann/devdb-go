package cli

import (
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
)

// auditFixture builds a report with two projects in high_feedback: noisy
// (12 rows) and quiet (2 rows).
func auditFixture() hub.AuditReport {
	report := hub.AuditReport{
		CollectedAt:       "2026-11-01T00:00:00Z",
		Mode:              "live",
		SeverityThreshold: "high",
		Sections:          map[string]hub.AuditSection{},
	}
	var rows []map[string]any
	for i := 0; i < 12; i++ {
		rows = append(rows, map[string]any{
			"project": "noisy", "severity": "high", "category": "correctness",
			"note": "noisy row", "id_prefix": "aaaa1111",
		})
	}
	for i := 0; i < 2; i++ {
		rows = append(rows, map[string]any{
			"project": "quiet", "severity": "critical", "category": "docs",
			"note": "quiet row", "id_prefix": "bbbb2222",
		})
	}
	report.Sections["high_feedback"] = hub.AuditSection{Rows: rows}
	return report
}

// TestFormatAuditGroupedAndCapped covers feedback 143bcc9f: the 69-row
// fleet audit printed as one uncapped wall of text. Human output must
// group rows per project (header carries the full count), cap each group
// at the limit with a "...and K more" line, and expand fully when the
// limit is 0 (global --all).
func TestFormatAuditGroupedAndCapped(t *testing.T) {
	report := auditFixture()

	capped := strings.Join(formatAudit(report, 10), "\n")

	// Section header still reports the true total (14 = 12 noisy + 2 quiet).
	if !strings.Contains(capped, "high feedback (14)") {
		t.Fatalf("section total missing: %s", capped)
	}
	// Per-project group headers with their own counts.
	if !strings.Contains(capped, "noisy (12)") || !strings.Contains(capped, "quiet (2)") {
		t.Fatalf("group headers missing: %s", capped)
	}
	// Cap applied to the noisy group only, with the remainder announced.
	if !strings.Contains(capped, "...and 2 more") {
		t.Fatalf("cap overflow line missing: %s", capped)
	}
	if strings.Contains(capped, "...and") && strings.Contains(capped, "quiet") {
		// quiet (2 rows) must not be capped — check its rows all rendered
		if strings.Count(capped, "quiet row") != 2 {
			t.Fatalf("quiet group should render all 2 rows uncapped: %s", capped)
		}
	}
	if strings.Count(capped, "noisy row") != 10 {
		t.Fatalf("noisy group should render exactly 10 capped rows, got %d", strings.Count(capped, "noisy row"))
	}
	// Row bodies no longer repeat the project name (the group header does).
	if strings.Contains(capped, "noisy  [high]") {
		t.Fatalf("row body still carries the project prefix: %s", capped)
	}

	// limit 0 (--all) expands everything, no overflow lines.
	expanded := strings.Join(formatAudit(report, 0), "\n")
	if strings.Count(expanded, "noisy row") != 12 {
		t.Fatalf("--all should render all 12 noisy rows, got %d", strings.Count(expanded, "noisy row"))
	}
	if strings.Contains(expanded, "...and") {
		t.Fatalf("--all output must not contain overflow lines: %s", expanded)
	}

	// Empty section still renders "(none)".
	empty := hub.AuditReport{Sections: map[string]hub.AuditSection{}}
	out := strings.Join(formatAudit(empty, 10), "\n")
	if !strings.Contains(out, "(none)") {
		t.Fatalf("empty section should render (none): %s", out)
	}
}
