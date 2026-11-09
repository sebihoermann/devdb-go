package export

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/domain/status"
	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
)

// detailCap is how many rows a view narrates one by one before it summarises
// the remainder. A ledger with four hundred open findings should not produce a
// four-hundred-line document; it should produce a report that says there are
// four hundred and shows the worst ones.
const detailCap = 8

// highSeverities are the severities that mean "a person should look at this".
var highSeverities = []string{"high", "critical"}

// overview is the slice of the ledger the summary narrates.
type overview struct {
	snap      status.Snapshot
	qual      status.QualitySignals
	items     []planning.PlanItem
	feedback  []feedback.Row
	findings  []review.Finding
	notes     []architecture.Note
	tasks     []tasks.Row
	reminders []reminders.Row
	blocked   []planning.BlockedItem
	last      string
}

// loadOverview gathers everything the summary needs in one pass.
func loadOverview(db *sql.DB, opts Options) (*overview, error) {
	o := &overview{}
	var err error
	o.snap, err = status.Build(db, opts.RepoRoot, opts.SchemaKind, opts.SchemaVersion)
	if err != nil {
		return nil, err
	}
	o.qual, err = status.Quality(db)
	if err != nil {
		return nil, err
	}
	o.items, err = planning.ListItems(db, planning.ItemFilter{})
	if err != nil {
		return nil, err
	}
	o.feedback, err = feedback.List(db, feedback.ListFilter{Status: "open"})
	if err != nil {
		return nil, err
	}
	o.findings, err = review.ListFindings(db, review.ListFilter{Status: "open"})
	if err != nil {
		return nil, err
	}
	o.notes, err = architecture.List(db, architecture.ListFilter{})
	if err != nil {
		return nil, err
	}
	o.tasks, err = tasks.List(db, "open", "", 0)
	if err != nil {
		return nil, err
	}
	o.reminders, err = reminders.List(db, "open", false, 0)
	if err != nil {
		return nil, err
	}
	o.blocked, err = planning.ListBlocked(db)
	if err != nil {
		return nil, err
	}
	o.last, err = lastActivity(db)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// lastActivity returns the newest timestamp across the tables that change
// when the project moves. It anchors the report to the ledger rather than to
// the wall clock: a committed report that said "last activity 2 days ago"
// would have to be rewritten every day to stay true.
func lastActivity(db *sql.DB) (string, error) {
	newest := ""
	for _, table := range []string{
		"feedback", "plan_items", "review_findings", "architecture_notes",
		"tasks", "reminders", "goals", "plans", "verification_runs",
	} {
		var ts sql.NullString
		// The table names are constants in this list, not input, so the
		// interpolation cannot carry an injection.
		if err := db.QueryRow("SELECT MAX(created_at) FROM " + table).Scan(&ts); err != nil {
			continue
		}
		if ts.Valid && ts.String > newest {
			newest = ts.String
		}
	}
	return newest, nil
}

// drawSummary renders the view that answers "what is going on".
func drawSummary(db *sql.DB, opts Options) ([]string, error) {
	o, err := loadOverview(db, opts)
	if err != nil {
		return nil, err
	}
	lines := summaryPosture(o)
	lines = append(lines, "")
	lines = append(lines, summaryScale(o)...)
	decisions := summaryDecisions(o)
	if len(decisions) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("**%s.**", decisionsHeading(len(decisions))))
		lines = append(lines, "")
		for _, d := range decisions {
			lines = append(lines, bullet("%s", d))
		}
	}
	return lines, nil
}

// summaryPosture opens the report with the state of play.
func summaryPosture(o *overview) []string {
	var b strings.Builder
	switch {
	case o.snap.InProgress > 0:
		b.WriteString("**Posture: active.** ")
	case o.snap.OpenItems > 0:
		b.WriteString("**Posture: planned.** ")
	default:
		b.WriteString("**Posture: idle.** ")
	}
	switch {
	case o.snap.InProgress > 0 && o.snap.InFlight != nil:
		b.WriteString(fmt.Sprintf("%s is in progress", o.snap.InFlight.Title))
		if o.last != "" {
			b.WriteString(fmt.Sprintf(", and the ledger was last written on %s", date(o.last)))
		}
		b.WriteString(".")
	case o.snap.InProgress > 0:
		b.WriteString(fmt.Sprintf("%s %s in progress",
			plural(o.snap.InProgress, "plan item is", "plan items are"), verb(o.snap.InProgress, "is", "are")))
		b.WriteString(".")
	case o.snap.OpenItems > 0:
		b.WriteString(fmt.Sprintf("Nothing is in progress, but %s planned and waiting to start",
			plural(o.snap.OpenItems, "plan item is", "plan items are")))
		if o.last != "" {
			b.WriteString(fmt.Sprintf("; the ledger was last written on %s", date(o.last)))
		}
		b.WriteString(".")
	default:
		b.WriteString("No plan item is planned or in progress")
		if o.last != "" {
			b.WriteString(fmt.Sprintf(", and the ledger was last written on %s", date(o.last)))
		}
		b.WriteString(".")
	}
	return []string{b.String()}
}

// summaryScale states the size of the ledger in prose.
func summaryScale(o *overview) []string {
	var parts []string
	if n := len(o.feedback); n > 0 {
		parts = append(parts, fmt.Sprintf("%d open %s", n, noun(n, "feedback entry", "feedback entries")))
	}
	if n := len(o.findings); n > 0 {
		parts = append(parts, fmt.Sprintf("%d unresolved %s", n, noun(n, "review finding", "review findings")))
	}
	if n := len(o.tasks); n > 0 {
		parts = append(parts, fmt.Sprintf("%d open %s", n, noun(n, "task", "tasks")))
	}
	if n := len(o.reminders); n > 0 {
		parts = append(parts, fmt.Sprintf("%d open %s", n, noun(n, "reminder", "reminders")))
	}
	if len(parts) == 0 {
		return []string{"The ledger is empty — no plans, no feedback, no findings, nothing recorded yet."}
	}
	lines := []string{"The ledger holds " + joinAnd(parts) + "."}
	if n := countSeverity(o.feedback, highSeverities...); n > 0 {
		total := len(o.feedback)
		lines = append(lines, fmt.Sprintf("%d of the %d open %s %s high or critical",
			n, total, noun(total, "feedback entry", "feedback entries"), verb(n, "is", "are"))+".")
	}
	if n := countFindingSeverity(o.findings, highSeverities...); n > 0 {
		total := len(o.findings)
		lines = append(lines, fmt.Sprintf("%d of the %d unresolved %s %s high or critical",
			n, total, noun(total, "review finding", "review findings"), verb(n, "is", "are"))+".")
	}
	if n := countStale(o.notes); n > 0 {
		if n == 1 {
			lines = append(lines, "1 architecture note has drifted from the file it describes.")
		} else {
			lines = append(lines, fmt.Sprintf("%d architecture notes have drifted from the files they describe.", n))
		}
	}
	return lines
}

// summaryDecisions collects the things only a person can resolve.
func summaryDecisions(o *overview) []string {
	var out []string
	unfixed := 0
	for _, f := range o.feedback {
		if severityIn(f.Severity, highSeverities) && f.ProposedFix == "" {
			unfixed++
		}
	}
	if unfixed > 0 {
		out = append(out, fmt.Sprintf("%d high-severity feedback %s %s no proposed fix and no owner.",
			unfixed, noun(unfixed, "entry", "entries"), verb(unfixed, "carries", "carry")))
	}
	if n := len(o.findings); n > 0 {
		s := fmt.Sprintf("%s still open", plural(n, "review finding is", "review findings are"))
		if oldest := dateOf(findingTimestamps(o.findings)...); oldest != "" {
			s += fmt.Sprintf(", the oldest since %s", oldest)
		}
		out = append(out, s+".")
	}
	if n := len(o.blocked); n > 0 {
		s := fmt.Sprintf("%s blocked and waiting on something outside the project",
			plural(n, "plan item is", "plan items are"))
		if len(o.blocked) > 0 {
			s += fmt.Sprintf(": %s", o.blocked[0].PlanItem.Title)
		}
		out = append(out, s+".")
	}
	if o.snap.InProgress == 0 && o.snap.OpenItems > 0 {
		out = append(out, fmt.Sprintf("Nothing is in progress, so %s of queued work %s not moving.",
			plural(o.snap.OpenItems, "item", "items"), verb(o.snap.OpenItems, "is", "are")))
	}
	if n := countOverdue(o.reminders); n > 0 {
		out = append(out, fmt.Sprintf("%s past due.", plural(n, "reminder is", "reminders are")))
	}
	return out
}

// decisionsHeading titles the decision list.
func decisionsHeading(n int) string {
	if n == 1 {
		return "One thing needs a decision"
	}
	return fmt.Sprintf("%s need a decision", plural(n, "thing", "things"))
}

// drawDelivery renders what is being worked on and how far along it is.
func drawDelivery(db *sql.DB, opts Options) ([]string, error) {
	plans, err := planning.ListPlans(db)
	if err != nil {
		return nil, err
	}
	items, err := planning.ListItems(db, planning.ItemFilter{})
	if err != nil {
		return nil, err
	}
	blocked, err := planning.ListBlocked(db)
	if err != nil {
		return nil, err
	}
	items = sortItemsByBuildOrder(items)
	blockedItems := map[string]bool{}
	for _, b := range blocked {
		blockedItems[b.PlanItem.ID] = true
	}

	var lines []string
	var active []planning.Plan
	for _, p := range plans {
		if p.Status == "active" {
			active = append(active, p)
		}
	}
	if len(active) == 0 && len(items) == 0 {
		return []string{"No plans are recorded in this ledger."}, nil
	}
	for _, p := range active {
		milestones, err := planning.ListMilestones(db, p.ID)
		if err != nil {
			return nil, err
		}
		planItems := itemsForPlan(items, p.ID)
		lines = append(lines, fmt.Sprintf("### %s", p.Title), "")
		lines = append(lines, fmt.Sprintf("Plan `%s` is %s. %s.", p.Slug, p.Status, stateSummary(planItems)), "")
		if len(milestones) == 0 {
			lines = append(lines, "This plan has no milestones, so its items are listed directly.", "")
			lines = append(lines, itemBullets(planItems, blockedItems, opts)...)
			lines = append(lines, "")
			continue
		}
		for _, m := range milestones {
			mItems := itemsForMilestone(planItems, m.ID)
			lines = append(lines, fmt.Sprintf("#### M%d — %s", m.Number, m.Title), "")
			lines = append(lines, stateSummary(mItems)+".", "")
			lines = append(lines, itemBullets(mItems, blockedItems, opts)...)
			lines = append(lines, "")
		}
		unassigned := itemsWithoutMilestone(planItems)
		if len(unassigned) > 0 {
			lines = append(lines, "#### Items not placed in a milestone", "")
			lines = append(lines, fmt.Sprintf("%s belong to this plan but to no milestone.",
				plural(len(unassigned), "item", "items")), "")
			lines = append(lines, itemBullets(unassigned, blockedItems, opts)...)
			lines = append(lines, "")
		}
	}
	if len(blocked) > 0 {
		lines = append(lines, "#### Blocked", "")
		for _, b := range blocked {
			lines = append(lines, bullet("**%s** — %s", b.PlanItem.Title, truncate(sanitize(b.Note), 200)))
		}
		lines = append(lines, "")
	}
	legacy := itemsWithoutPlan(items)
	if len(legacy) > 0 {
		lines = append(lines, "#### Unassigned items", "")
		lines = append(lines, fmt.Sprintf("%s do not belong to a plan.", plural(len(legacy), "item", "items")), "")
		lines = append(lines, itemBullets(legacy, blockedItems, opts)...)
	}
	return lines, nil
}

// stateSummary describes a set of plan items in one clause, omitting the
// counts that are zero so the sentence says something.
func stateSummary(items []planning.PlanItem) string {
	done, active, planned := countItemStates(items)
	if len(items) == 0 {
		return "It has no plan items yet"
	}
	var parts []string
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d %s done", done, verb(done, "is", "are")))
	}
	if active > 0 {
		parts = append(parts, fmt.Sprintf("%d %s in progress", active, verb(active, "is", "are")))
	}
	if planned > 0 {
		parts = append(parts, fmt.Sprintf("%d %s not started", planned, verb(planned, "is", "are")))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("It carries %s", plural(len(items), "plan item", "plan items"))
	}
	return "Of " + plural(len(items), "item", "items") + ", " + joinAnd(parts)
}

// itemBullets renders one bullet per plan item, capped at the detail limit.
func itemBullets(items []planning.PlanItem, blocked map[string]bool, opts Options) []string {
	cap := opts.limit(detailCap)
	shown := items
	if cap > 0 && len(shown) > cap {
		shown = shown[:cap]
	}
	var lines []string
	for _, it := range shown {
		mark := "○"
		switch it.Status {
		case "done":
			mark = "●"
		case "in_progress":
			mark = "◐"
		}
		line := fmt.Sprintf("%s **%s** — %s", mark, it.Title, itemStatusWord(it.Status))
		if blocked[it.ID] {
			line += " _(blocked)_"
		}
		lines = append(lines, "- "+line)
	}
	if cap > 0 && len(items) > cap {
		lines = append(lines, fmt.Sprintf("- _… and %s not shown. Re-run with `--all` for the full list._",
			plural(len(items)-cap, "item", "items")))
	}
	return lines
}

// itemStatusWord renders a plan item status as prose.
func itemStatusWord(status string) string {
	switch status {
	case "done":
		return "done"
	case "in_progress":
		return "in progress"
	case "wontfix":
		return "will not be done"
	default:
		return "not started"
	}
}

// drawQuality renders the trust signals: findings, drifted notes, and
// whether the last verification still covers the tree.
func drawQuality(db *sql.DB, opts Options) ([]string, error) {
	findings, err := review.ListFindings(db, review.ListFilter{Status: "open"})
	if err != nil {
		return nil, err
	}
	findings = sortFindingsBySeverity(findings)
	notes, err := architecture.List(db, architecture.ListFilter{})
	if err != nil {
		return nil, err
	}
	runs, err := verification.List(db, opts.limit(detailCap))
	if err != nil {
		return nil, err
	}
	lines := labelled("Review findings", qualityFindings(findings, opts))
	lines = append(lines, "")
	lines = append(lines, labelled("Architecture notes", qualityNotes(notes))...)
	lines = append(lines, "")
	lines = append(lines, labelled("Verification", qualityVerification(db, runs, opts))...)
	return lines, nil
}

// qualityFindings narrates the open review findings.
func qualityFindings(findings []review.Finding, opts Options) []string {
	if len(findings) == 0 {
		return []string{"No review findings are open."}
	}
	high := countFindingSeverity(findings, highSeverities...)
	s := fmt.Sprintf("%s open", plural(len(findings), "review finding is", "review findings are"))
	if high > 0 {
		s += ", " + sharePhrase(high, len(findings), "high or critical")
	}
	s += "."
	if oldest := dateOf(findingTimestamps(findings)...); oldest != "" {
		s += fmt.Sprintf(" The oldest has been open since %s.", oldest)
	}
	lines := []string{s, ""}
	cap := opts.limit(detailCap)
	shown := findings
	if cap > 0 && len(shown) > cap {
		shown = shown[:cap]
	}
	for _, f := range shown {
		where := f.FilePath
		if where == "" {
			where = f.Principle
		}
		lines = append(lines, fmt.Sprintf("### %s · %s · %s", id8(f.ID), severityLabel(f.Severity), where), "")
		lines = append(lines, f.Title, "")
		lines = append(lines, quote(f.Recommendation, opts.maxChars())...)
		lines = append(lines, "", fmt.Sprintf("_Recorded %s · effort %s · confidence %s_", date(f.CreatedAt), f.Effort, f.Confidence), "")
	}
	if cap > 0 && len(findings) > cap {
		lines = append(lines, fmt.Sprintf("_… and %s not shown. Re-run with `--all` for the full list._",
			plural(len(findings)-cap, "finding", "findings")))
	}
	return lines
}

// qualityNotes narrates the architecture notes and which of them drifted.
func qualityNotes(notes []architecture.Note) []string {
	active := activeNotes(notes)
	if len(active) == 0 {
		return []string{"No architecture notes are recorded."}
	}
	stale := countStale(active)
	s := fmt.Sprintf("%s recorded", plural(len(active), "architecture note is", "architecture notes are"))
	if stale > 0 {
		s += ", and " + driftPhrase(stale, len(active))
	} else if len(active) == 1 {
		s += ", and it still matches the file it describes"
	} else {
		s += ", and all of them still match the files they describe"
	}
	s += "."
	lines := []string{s}
	if stale > 0 {
		var names []string
		for _, n := range active {
			if n.Stale {
				names = append(names, "`"+n.Topic+"`")
			}
		}
		lines = append(lines, "", "The drifted notes are "+joinAnd(names)+". An agent reading them today is reading a description of code that has since changed.")
	}
	return lines
}

// qualityVerification reports whether the last recorded verification still
// covers the current tree.
func qualityVerification(db *sql.DB, runs []verification.Run, opts Options) []string {
	if len(runs) == 0 {
		return []string{"No verification run has been recorded, so nothing in this ledger is known to pass."}
	}
	lines := []string{fmt.Sprintf("%s recorded.",
		plural(len(runs), "verification run is", "verification runs are")), ""}
	for _, r := range runs {
		fresh, reason := verification.EvaluateFreshness(db, r.ID)
		status := r.Status
		if status == "passed" && !fresh {
			status = "passed but no longer current"
		}
		line := fmt.Sprintf("`%s` (scope `%s`) %s", r.Command, r.Scope, status)
		if r.FinishedAt != "" {
			line += fmt.Sprintf(", finished %s", date(r.FinishedAt))
		}
		if !fresh && reason != "" && reason != "no_inputs_stored" {
			line += fmt.Sprintf(" — %s, so the result no longer covers the current tree", freshnessPhrase(reason))
		}
		lines = append(lines, bullet("%s", line))
	}
	return lines
}

// freshnessPhrase turns a freshness reason code into prose.
func freshnessPhrase(reason string) string {
	switch reason {
	case "no_inputs_stored":
		return "no inputs were recorded for it"
	case "run_missing", "run_lookup_failed":
		return "the run could not be read back"
	default:
		return "its inputs have changed since"
	}
}

// drawWork renders the items that wait on a person rather than on an agent.
func drawWork(db *sql.DB, opts Options) ([]string, error) {
	tsk, err := tasks.List(db, "open", "", 0)
	if err != nil {
		return nil, err
	}
	rems, err := reminders.List(db, "open", false, 0)
	if err != nil {
		return nil, err
	}
	lines := labelled("Tasks", workTasks(tsk, opts))
	lines = append(lines, "")
	lines = append(lines, labelled("Reminders", workReminders(rems, opts))...)
	return lines, nil
}

// workTasks narrates the open tasks.
func workTasks(tsk []tasks.Row, opts Options) []string {
	if len(tsk) == 0 {
		return []string{"No tasks are open."}
	}
	high := 0
	for _, t := range tsk {
		if t.Priority == "high" || t.Priority == "critical" {
			high++
		}
	}
	s := fmt.Sprintf("%s open", plural(len(tsk), "task is", "tasks are"))
	if high > 0 {
		s += ", " + sharePhrase(high, len(tsk), "high priority")
	}
	s += "."
	lines := []string{s, ""}
	cap := opts.limit(detailCap)
	shown := tsk
	if cap > 0 && len(shown) > cap {
		shown = shown[:cap]
	}
	for _, t := range shown {
		line := fmt.Sprintf("**%s** — priority %s", t.Title, t.Priority)
		if t.DueAt != "" {
			line += fmt.Sprintf(", due %s", date(t.DueAt))
		}
		lines = append(lines, bullet("%s", line))
	}
	if cap > 0 && len(tsk) > cap {
		lines = append(lines, fmt.Sprintf("_… and %s not shown. Re-run with `--all` for the full list._",
			plural(len(tsk)-cap, "task", "tasks")))
	}
	return lines
}

// workReminders narrates the open reminders.
func workReminders(rems []reminders.Row, opts Options) []string {
	if len(rems) == 0 {
		return []string{"No reminders are open."}
	}
	overdue := countOverdue(rems)
	s := fmt.Sprintf("%s open", plural(len(rems), "reminder is", "reminders are"))
	if overdue > 0 {
		s += ", " + sharePhrase(overdue, len(rems), "past due")
	}
	s += "."
	lines := []string{s, ""}
	cap := opts.limit(detailCap)
	shown := rems
	if cap > 0 && len(shown) > cap {
		shown = shown[:cap]
	}
	for _, r := range shown {
		line := fmt.Sprintf("**%s**", r.Title)
		if r.DueAt != "" {
			line += fmt.Sprintf(", due %s", date(r.DueAt))
		}
		if r.FilePath != "" {
			line += fmt.Sprintf(" (`%s`)", r.FilePath)
		}
		lines = append(lines, bullet("%s", line))
	}
	if cap > 0 && len(rems) > cap {
		lines = append(lines, fmt.Sprintf("_… and %s not shown. Re-run with `--all` for the full list._",
			plural(len(rems)-cap, "reminder", "reminders")))
	}
	return lines
}

// drawArchitecture narrates the recorded architecture knowledge.
func drawArchitecture(db *sql.DB, opts Options) ([]string, error) {
	notes, err := architecture.List(db, architecture.ListFilter{})
	if err != nil {
		return nil, err
	}
	active := activeNotes(notes)
	if len(active) == 0 {
		return []string{"No architecture notes are recorded, so the project carries no written explanation of how it works."}, nil
	}
	stale := countStale(active)
	s := fmt.Sprintf("%s recorded", plural(len(active), "architecture note is", "architecture notes are"))
	if stale > 0 {
		s += ", and " + driftPhrase(stale, len(active))
	} else if len(active) == 1 {
		s += ", and it still matches the file it describes"
	} else {
		s += ", and all of them still match the files they describe"
	}
	s += "."
	lines := []string{s, ""}
	cap := opts.limit(detailCap)
	shown := active
	if cap > 0 && len(shown) > cap {
		shown = shown[:cap]
	}
	for _, n := range shown {
		lines = append(lines, fmt.Sprintf("### %s", n.Topic), "")
		lines = append(lines, quote(n.Body, opts.maxChars())...)
		lines = append(lines, "")
		meta := fmt.Sprintf("_Verified %s · confidence %s_", date(n.LastVerifiedAt), n.Confidence)
		if len(n.SourcePaths) > 0 {
			meta = fmt.Sprintf("_Verified %s · confidence %s · sources: %s_",
				date(n.LastVerifiedAt), n.Confidence, strings.Join(n.SourcePaths, ", "))
		}
		if n.Stale {
			meta += "\n\n_The files it names have changed since. Run `devdb arch verify` to refresh it._"
		}
		lines = append(lines, meta, "")
	}
	if cap > 0 && len(active) > cap {
		lines = append(lines, fmt.Sprintf("_… and %s not shown. Re-run with `--all` for the full list._",
			plural(len(active)-cap, "note", "notes")))
	}
	return lines, nil
}

// drawFeedback narrates the open feedback.
func drawFeedback(db *sql.DB, opts Options) ([]string, error) {
	rows, err := feedback.List(db, feedback.ListFilter{Status: "open"})
	if err != nil {
		return nil, err
	}
	rows = sortFeedbackBySeverity(rows)
	if len(rows) == 0 {
		return []string{"No feedback is open."}, nil
	}
	high := countSeverity(rows, highSeverities...)
	s := fmt.Sprintf("%s open", plural(len(rows), "feedback entry is", "feedback entries are"))
	if high > 0 {
		s += ", " + sharePhrase(high, len(rows), "high or critical")
	}
	s += "."
	byCategory := map[string]int{}
	for _, r := range rows {
		if r.Category != "" {
			byCategory[r.Category]++
		}
	}
	if len(byCategory) > 0 {
		var names []string
		for _, c := range sortedCategories(byCategory) {
			names = append(names, "`"+c+"`")
		}
		if len(rows) == 1 {
			s += " It is filed under " + joinAnd(names) + "."
		} else {
			var cats []string
			for _, c := range sortedCategories(byCategory) {
				cats = append(cats, fmt.Sprintf("%d under `%s`", byCategory[c], c))
			}
			s += " By category: " + joinAnd(cats) + "."
		}
	}
	lines := []string{s, ""}
	cap := opts.limit(detailCap)
	shown := rows
	if cap > 0 && len(shown) > cap {
		shown = shown[:cap]
	}
	for _, r := range shown {
		meta := []string{id8(r.ID), severityLabel(r.Severity), r.Role}
		if r.Category != "" {
			meta = append(meta, r.Category)
		}
		lines = append(lines, fmt.Sprintf("### %s", strings.Join(meta, " · ")), "")
		lines = append(lines, quote(r.Note, opts.maxChars())...)
		if r.Context != "" {
			lines = append(lines, "", fmt.Sprintf("_Context: %s_", truncate(sanitize(r.Context), 200)))
		}
		if r.ProposedFix != "" {
			lines = append(lines, "", fmt.Sprintf("_Proposed fix: %s_", truncate(sanitize(r.ProposedFix), 200)))
		}
		lines = append(lines, "", fmt.Sprintf("_Logged %s_", date(r.CreatedAt)), "")
	}
	if cap > 0 && len(rows) > cap {
		lines = append(lines, fmt.Sprintf("_… and %s not shown. Re-run with `--all` for the full list._",
			plural(len(rows)-cap, "entry", "entries")))
	}
	return lines, nil
}

// labelled prefixes a paragraph with a bold label, so a view that holds
// several unrelated subjects reads as a list of subjects rather than a run of
// unlabelled paragraphs.
func labelled(name string, body []string) []string {
	if len(body) == 0 {
		return nil
	}
	out := make([]string, 0, len(body))
	out = append(out, "**"+name+".** "+body[0])
	out = append(out, body[1:]...)
	return out
}

// sortFeedbackBySeverity puts the entries a person must act on first.
// The ledger hands them back newest-first, which is the right order for a
// chronological read and the wrong order for a briefing: an administrator
// opening the report wants the high-severity item at the top, not whichever
// one happened to be written last.
func sortFeedbackBySeverity(rows []feedback.Row) []feedback.Row {
	out := make([]feedback.Row, len(rows))
	copy(out, rows)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out
}

// sortFindingsBySeverity puts the findings a person must act on first.
func sortFindingsBySeverity(findings []review.Finding) []review.Finding {
	out := make([]review.Finding, len(findings))
	copy(out, findings)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out
}

// activeNotes returns the architecture notes that are still active.
func activeNotes(notes []architecture.Note) []architecture.Note {
	var out []architecture.Note
	for _, n := range notes {
		if n.Status == "active" {
			out = append(out, n)
		}
	}
	return out
}

// sortItemsByBuildOrder puts plan items in the order they were written.
// The storage layer hands them back newest-first, which is the right order
// for a ledger read and the wrong order for a delivery narrative: a reader
// wants to walk the plan from its first item to its last.
func sortItemsByBuildOrder(items []planning.PlanItem) []planning.PlanItem {
	out := make([]planning.PlanItem, len(items))
	copy(out, items)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// itemsForPlan returns the items belonging to a plan.
func itemsForPlan(items []planning.PlanItem, planID string) []planning.PlanItem {
	var out []planning.PlanItem
	for _, it := range items {
		if it.PlanID == planID {
			out = append(out, it)
		}
	}
	return out
}

// itemsForMilestone returns the items belonging to a milestone.
func itemsForMilestone(items []planning.PlanItem, milestoneID string) []planning.PlanItem {
	var out []planning.PlanItem
	for _, it := range items {
		if it.MilestoneID == milestoneID {
			out = append(out, it)
		}
	}
	return out
}

// itemsWithoutMilestone returns the items that belong to a plan but to no
// milestone. Without this they would appear in no section at all: the
// milestone loop only walks items it can match, so an item with no
// milestone_id would silently disappear from the report.
func itemsWithoutMilestone(items []planning.PlanItem) []planning.PlanItem {
	var out []planning.PlanItem
	for _, it := range items {
		if it.MilestoneID == "" {
			out = append(out, it)
		}
	}
	return out
}

// itemsWithoutPlan returns the items that belong to no plan.
func itemsWithoutPlan(items []planning.PlanItem) []planning.PlanItem {
	var out []planning.PlanItem
	for _, it := range items {
		if it.PlanID == "" {
			out = append(out, it)
		}
	}
	return out
}

// countItemStates counts the items by status.
func countItemStates(items []planning.PlanItem) (done, active, planned int) {
	for _, it := range items {
		switch it.Status {
		case "done":
			done++
		case "in_progress":
			active++
		default:
			planned++
		}
	}
	return
}

// countSeverity counts the rows at the given severities.
func countSeverity(rows []feedback.Row, severities ...string) int {
	n := 0
	for _, r := range rows {
		if severityIn(r.Severity, severities) {
			n++
		}
	}
	return n
}

// countFindingSeverity counts findings at the given severities.
func countFindingSeverity(findings []review.Finding, severities ...string) int {
	n := 0
	for _, f := range findings {
		if severityIn(f.Severity, severities) {
			n++
		}
	}
	return n
}

// countStale counts the architecture notes that drifted.
func countStale(notes []architecture.Note) int {
	n := 0
	for _, x := range notes {
		if x.Stale {
			n++
		}
	}
	return n
}

// countOverdue counts the reminders whose due date has passed.
func countOverdue(rems []reminders.Row) int {
	now := nowUTC()
	n := 0
	for _, r := range rems {
		if r.DueAt != "" && before(r.DueAt, now) {
			n++
		}
	}
	return n
}

// findingTimestamps returns every finding's created_at, so dateOf can pick
// the earliest.
func findingTimestamps(findings []review.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.CreatedAt)
	}
	return out
}

// sortedCategories returns the category names in a stable order.
func sortedCategories(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// capitalizeFirst upper-cases the first letter of a sentence.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
