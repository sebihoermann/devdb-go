# Plan: `devdb hub audit` — one-command cross-project open issues + plans

**Status:** design — pending approval
**Repo:** devdb-go (main, clean working tree)
**Last updated:** 2026-06-19

## Context

The session-start ritual is documented per-project
(`devdb resume`, `devdb status`, `devdb quality`, `devdb report`).
The cross-project equivalent — needed to triage across the 8
projects currently in `~/.devdb-projects` — currently requires:

- `hub list`
- `hub dashboard --view work` (cached snapshot)
- `hub across open-debt` (high-severity feedback + findings only)
- one `plan item list --status planned` per project
- one `plan item list --status in_progress` per project
- one `review list --status open --severity high` per project
- one `reminder list --overdue` per project
- one `arch list --stale` per project

A single session-start audit run today takes ~30 commands.
This document proposes one verb that replaces all of them.

## Existing surface

| Verb | Read mode | Scope | Limitation |
|------|-----------|-------|------------|
| `hub dashboard --view {summary\|work\|delivery\|quality}` | cached snapshot | per-project counts | needs `hub sync` first; no plans or reminders rows |
| `hub across <query>` | live federation read | flat row dump | only `code-hygiene-cross`, `similar-feedback`, `open-debt`; `open-debt` is high/critical only, no plans/arch/reminders/blocked |
| `devdb report` | per-project | top 5 open feedback + quality + status | not cross-project |

`internal/domain/hub/snapshot.go::CollectSnapshot` already computes
every per-project count we need: `open_high_feedback`,
`open_high_findings`, `stale_arch_notes`, `overdue_reminders`,
`in_progress_items`, `blocked_reason`, `latest_verification_freshness`.

`internal/domain/hub/across.go` is the cleanest existing pattern to
mirror: registry walk → per-project open → row aggregation.

## Design

### Command: `devdb hub audit`

Top-level under `hub`, alongside `dashboard` / `across` / `doctor`.
**Live federation read by default**, matching `hub across`.
`--cached` falls back to the hub snapshot for parity with `dashboard`.

### Sections

1. **high feedback** — severity ≥ threshold, role any, with
   project / severity / category / id prefix / note (≤ 72 chars)
2. **high review findings** — same shape, from `review_findings`
3. **stale architecture notes** — topic + project
4. **overdue reminders** — title + due date + project
5. **in-progress plan items** — title + project (cross-project resume)
6. **blocked** — items whose `status_log.note` matches
   `blocked|blocker|waiting|stuck|cannot proceed|can't proceed`
7. **planned per project** — `<alias>  count=N  next="<M-step 1 title>"`
   (one line per project, replaces 8× `plan item list --status planned`)
8. **stale verification** — projects whose latest verification is no
   longer fresh

Empty sections are silently omitted in human output; in JSON they
appear as empty arrays.

### Output

**Human** — section headers + compact lines (matches `devdb report`
house style). Example:

```
# audit · 2026-06-19 · 8 projects

high feedback (1)
  MostActive  [high] Yahoo Finance scraper hangs on GDPR consent overlay …  [8881e326]

high review findings
  (none)

stale architecture notes (1)
  workspace · host-firewall-ssh-layering

overdue reminders
  (none)

in-progress plan items
  (none)

blocked
  (none)

planned per project
  workspace    N=26  next="Add src/devdb/project_config.py (tomllib + minimal yaml parser)"
  mail-server  N=36  next="Sebastián: pick the domain name"
  trips        N=11  next="Confirm dog weight + check against Iberia brachycephalic banned list"

stale verification
  (none)
```

**JSON:**

```json
{
  "collected_at": "2026-06-19T19:46:14Z",
  "registry": "/root/.devdb-projects",
  "mode": "live",
  "severity_threshold": "high",
  "sections": {
    "high_feedback": [{"project": "MostActive", "id": "8881e326…", "severity": "high", "category": "correctness", "note": "Yahoo Finance scraper hangs…"}],
    "high_findings": [],
    "stale_arch": [{"project": "workspace", "topic": "host-firewall-ssh-layering"}],
    "overdue_reminders": [],
    "in_progress": [],
    "blocked": [],
    "planned_per_project": [{"project": "workspace", "count": 26, "next": "Add src/devdb/project_config.py…"}],
    "stale_verification": []
  },
  "by_project": {
    "MostActive": {"open_high_feedback": 1, "open_plan_items": 0, "in_progress": 0, "stale_arch": 0, "overdue_reminders": 0},
    "workspace":   {"open_high_feedback": 0, "open_plan_items": 26, "in_progress": 0, "stale_arch": 1, "overdue_reminders": 0}
  }
}
```

### Flags

- `--json` (global)
- `--severity {info|low|med|medium|high|critical}` — default `high`
  (matches existing `open-debt`); lower threshold widens sections 1–2
- `--kind <list>` — repeatable or comma-separated; subset of
  `feedback,findings,stale_arch,overdue,in_progress,blocked,planned,verification`
- `--project <alias>` — repeatable; narrows registry walk
- `--cached` — read from `~/.devdb/metadata.db` snapshot instead of
  live federation read
- `--include-archived` — default off; surfaces archived feedback rows

## Implementation

### New files

- `internal/domain/hub/audit.go` — `AuditOptions`, `AuditReport`,
  `AuditSection` types; `Audit(opts) (AuditReport, error)`. Mirrors
  the `Across` shape. Per-project live reader reuses the SQL
  fragments already in `snapshot.go` for `open_high_feedback`,
  `stale_arch_notes`, `overdue_reminders`, `blocked_reason`.
- `internal/domain/hub/audit_test.go` — table-driven:
  - empty registry → empty report, exit 0
  - one project with one of each kind → all 8 sections populated
  - `--project` filter narrows correctly
  - `--severity high` vs `--severity medium` differ
  - corrupt `.devdb/development.db` skipped silently (parity with `across`)
  - missing `.devdb/development.db` skipped silently
  - `--cached` returns from hub snapshot
  - JSON shape stable

### Modified files

- `internal/cli/commands_hub.go` — add `cmdHubAudit(openCtx)`;
  register in the `cmdHub` slice (line 22–31) alongside
  `cmdHubAcross` / `cmdHubDashboard`. New `formatAudit` human
  renderer mirrors `formatDashboard` (line 240).
- `skills/devdb/SKILL.md` — add `hub audit` to the cheatsheet
  (line ~362) and to the "Other useful reads" block (line ~351).
  One-line edits each.
- `skills/devdb/SPEC.md` — append a short subsection under M7
  federation noting the new verb and its flag contract.
- `CLAUDE.md` — one-line add to "Top 5 verbs" section: `hub audit`
  for cross-project open issues.

### Test plan

- `go test ./internal/domain/hub/...` — new `audit_test.go` plus
  existing suite unchanged
- `go test ./internal/cli/...` — golden-file snapshot for human
  output (`UPDATE_GOLDEN=1 go test ./internal/cli -run TestHubAudit`)
- Coverage on `audit.go` ≥ current hub-package average

## Dogfood plan (lives in `devdb-go/.devdb/development.db`)

Create plan `devdb-go · hub-audit`, milestone M1, items:

1. **Design** — `AuditOptions` / `AuditReport` / `AuditSection`
   structs landed in `audit.go`; JSON sample written into SPEC.md.
   Acceptance: `go build ./...` succeeds; types compile.
2. **Implement** — `audit.go` + `cmdHubAudit` + `cmdHub` slice
   registration. Acceptance: `devdb hub audit --help` shows the verb;
   `devdb hub audit` against current 8-project registry returns the
   snapshot above; exit 0 on empty registry.
3. **Tests** — `audit_test.go` covers each section + skip cases.
   Acceptance: `go test ./...` green; new file ≥ 90% line coverage.
4. **Docs** — SKILL.md + SPEC.md + CLAUDE.md updates. Acceptance:
   `devdb help hub audit` short-help is one line; cheatsheet entry
   present; SPEC section present.
5. **Verification** — `go build -o devdb ./cmd/devdb` +
   `go test ./...` + run `devdb hub audit --json` against current
   registry, capture output, record with `verify record`.
   Acceptance: `verify record "go test ./..." --scope . --status passed --exit-code 0 --finished`.

Per `devdb-go/CLAUDE.md`: bracket execution with
`devdb plan item start <id>` / `devdb plan item pause <id>`.

## Out of scope (deliberately not now)

- TTL cache of audit results — live read is ~40 SQL statements
  across 8 projects, acceptable for session-start cadence
- `project_links` cross-project relations table — already SPEC'd
- `hub across-sql` ad-hoc federated SQL — not the gap
- Replacing `hub dashboard` — cached snapshot has its own use
- Removing `hub across open-debt` — keep both for one release, mark
  deprecated in SKILL.md, remove after adoption

## Risks

- **Live vs cached semantics:** defaulting to live matches `across`,
  differs from `dashboard`. Docs must say "live by default; use
  `--cached` for dashboard parity".
- **Severity threshold:** default `high` keeps noise down; lowering
  to `medium` doubles typical row count in this workspace.
- **Legacy Python schemas:** must use the same `DetectSchema` skip
  that `CollectSnapshot` uses, otherwise schema-version errors will
  abort the read.

## Open questions resolved in this draft

1. Default mode — **live** (matches `hub across`, matches session-start intent)
2. Default severity — **high** (matches existing `open-debt`)
3. Name — **`hub audit`** (distinct from `dashboard` / `doctor` / `across`)
4. `open-debt` deprecation — **defer** (keep both for one release)
