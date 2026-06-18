# Go Native Behavioral Parity Matrix

Reference for answering **“is behavior X ported?”** during the Python → Go cutover.
Snapshot: 2026-06-16. Source of truth for runtime behavior is `golang/`; this table tracks intent and gaps.

## Classification key

| Class | Meaning |
|-------|---------|
| **keep** | Same product behavior; only command grammar changed |
| **merge** | Multiple legacy verbs folded into one noun/verb command with flags |
| **redesign** | Deliberately different behavior or storage shape |
| **remove** | Intentionally not ported; rationale documented |

## Status key

| Status | Meaning |
|--------|---------|
| **done** | Usable in Go CLI for normal agent workflows |
| **partial** | Core path works; noted gaps remain |
| **not ported** | No Go equivalent by design |
| **N/A** | Go-only; no Python predecessor |

---

## Top-level reads

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `init` | `devdb init` | keep | done | Creates `.devdb/development.db`, runs Go migrations |
| `status` | `devdb status` | keep | done | Compact delivery snapshot; `--json` / `--verbose` |
| `quality` | `devdb quality` | keep | done | Trust signals; stale arch, findings, missed calls |
| `report` | `devdb report` | keep | done | Actionable overview; `--all` expands feedback section |
| `resume` | `devdb resume` | keep | done | Surfaces latest `in_progress` plan item |
| `doctor` / `doctor hygiene` | `devdb doctor` / `devdb doctor hygiene` | merge | done | Per-repo health; hub sync is `hub doctor` |
| `help` | `devdb help` | keep | done | Category list; legacy names suggest new grammar |

---

## Planning and workflow

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `create-plan` | `plan create` | merge | done | |
| `scaffold-plan --mode design\|implement` | `plan scaffold --mode design\|implement` | merge | done | Writes HTML artifact under repo `docs/` |
| `promote-plan` | `plan promote` | merge | done | Design → implement titles and acceptance text |
| `reconcile-plans` | `plan reconcile` | merge | done | Drift detection; `--apply` repairs |
| `add-milestone` | `plan milestone add` | merge | done | |
| `list-milestones` | `plan milestone list` | merge | done | |
| `set-status` (milestone) | `plan milestone status` | merge | done | |
| `add-plan-item` | `plan item add` | merge | done | Structured plans |
| `add-plan` (flat phase/step) | `plan item add --legacy --phase … --step …` | redesign | partial | Legacy rows readable; deprecation path only |
| `list-plan-items` | `plan item list` | merge | done | `--legacy` filter for flat items |
| `show-plan-item` | `plan item show` | merge | done | Acceptance prefixes for `meet` |
| `work-on` | `plan item start` | merge | done | Stdout: bare id; scope hints on stderr |
| `pause-on` | `plan item pause --note` | merge | done | `--note` required |
| `close-plan-item` | `plan item close --evidence` | merge | done | Rejects open acceptance criteria |
| `set-status` (item) | `plan item status` | merge | done | Cannot bypass acceptance closure for `done` |
| `add-acceptance` | `plan acceptance add` | merge | done | Auto-ordinal when omitted |
| `meet-acceptance` | `plan acceptance meet` | merge | done | |
| `backfill-acceptance` | `plan acceptance backfill` | merge | done | Markdown spec or stdin |
| `add-plan-file` | `plan file add` | merge | done | |
| `show-plan` | `plan show` | merge | done | Slug or id |
| `show-plan-tree` | `plan tree` | merge | done | |
| `list-plans` | `plan list` | merge | done | |
| `set-plan-status` | `plan status` | merge | done | |
| `link-source` | — | remove | not ported | `entity_links` / `project_links` tables unused in Go schema; link via plan item body or feedback context instead |

---

## Feedback, goals, and memory

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `feedback-user` | `feedback add --role user` | merge | done | |
| `feedback-model` | `feedback add --role model` | merge | done | |
| `feedback-codebase` | `feedback add --role codebase` | merge | done | |
| `close-feedback` | `feedback close` | merge | done | |
| `annotate` | `feedback annotate` | merge | done | |
| `show-feedback` | `feedback list` / `feedback show` | merge | done | Legacy name not registered |
| `add-goal` | `goal add` | merge | done | |
| `set-goal-status` | `goal set` | merge | done | `inactive` → `wontfix` on import |
| `add-feature` | `feature add` | merge | done | |
| `add-suggestion` | `feedback add --role model --category idea` | merge | done | `improvement_suggestions` table not ported |
| `import-feedback-md` | `feedback import markdown` | merge | done | |
| `import-branch-commits` | `feedback import commits` | merge | done | Writes `commit_archeology` |
| `log-session` | — | remove | not ported | Session notes covered by `plan item pause --note` and `status_log` |
| `add-tightening` / `close-tightening` | — | remove | not ported | `code_tightening` table skipped by importer |

---

## Architecture notes

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `add-arch-note` | `arch add` | merge | done | |
| `update-arch-note` | `arch update` | merge | done | |
| `verify-arch-note` | `arch verify ID` | merge | done | |
| `verify-all-arch-notes` | `arch verify all` / `arch verify --all` | merge | done | Bulk staleness check |
| `list-arch-notes` | `arch list` | merge | done | `--stale`, `--touching` |
| `arch-render` | `arch render` | merge | done | |

---

## Inventory and context

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `scan` | `inventory scan` | merge | done | Populates `repo_files` |
| `snapshot-loc` | `inventory loc` | redesign | partial | On-the-fly summary; no `loc_snapshots` persistence in Go |
| `context` | `inventory context` | merge | done | |
| `diff-since` | `inventory diff` | merge | done | |
| `suggest-cuts` | `inventory suggest-cuts` | merge | done | Opens `grass-cutter` tier review run; `--dry-run`, `--paths` |

---

## Review and verification

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `review-start` | `review start` | merge | done | |
| `review-add-finding` | `review finding` | merge | done | |
| `review-finish` | `review finish` | merge | done | |
| `review-list` | `review list` | merge | done | `--all` expands cap |
| `review-resolve` | `review resolve` | merge | done | |
| `review-report` | `review report` | merge | done | |
| `review-principles` | `review principles` | merge | done | |
| `review-import` | `review import` | merge | done | JSONL batch |
| `record-verification-run` | `verify record` | merge | done | Auto-collects inputs when omitted |
| `query-verification` | `verify query` | merge | done | |
| `show-verification-run` | `verify show` | merge | done | |
| `dismiss-verification` | `verify dismiss` | merge | done | |
| `list-verification-runs` | — | remove | not ported | Use `verify query` / `verify show`; no list verb in new grammar |

---

## Tasks, approvals, reminders

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `add-task` | `task add` | merge | done | |
| `list-tasks` | `task list` | merge | done | |
| `complete-task` | `task done` | merge | done | |
| `set-task-status` | `task status` | merge | done | |
| `request-approval` | `approval request` | merge | done | |
| `approve` / `reject` / `withdraw-approval` | `approval approve` / `reject` / `withdraw` | merge | done | |
| `list-pending-approvals` | `approval list` | merge | done | |
| `approval-log` | `approval log` | merge | done | |
| `add-reminder` | `reminder add` | merge | done | |
| `list-reminders` | `reminder list` | merge | done | `--all` expands cap |
| `show-reminder` | `reminder show` | merge | done | |
| `dismiss-reminder` | `reminder dismiss` | merge | done | |
| `snooze-reminder` / `unsnooze-reminder` | `reminder snooze` / `unsnooze` | merge | done | |

---

## Hygiene, archive, analytics

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `gc` | `archive gc` | merge | done | Stale open feedback + missing-file findings |
| `archive` | `archive run` | merge | done | |
| `restore-list` | `archive list` | merge | done | |
| `restore` | `archive restore` | merge | done | |
| `list-missed-calls` | `analytics missed` | merge | done | |
| `missed-calls-summary` | `analytics summary` | merge | done | |

---

## Hub and federation

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `register` | `hub register` | merge | done | |
| `list-projects` | `hub list` | merge | done | |
| `hub-sync` | `hub sync` | merge | done | `--watch` supported |
| `doctor-sync` | `hub doctor` | merge | done | |
| `hub-dashboard` | `hub dashboard` | merge | done | `--view summary\|work\|delivery\|quality` |
| `hub-project` | `hub project` | merge | done | |
| `across` | `hub across` | merge | done | Federated queries hit project DBs directly |
| `hub-work` / `hub-status` / `hub-quality` | `hub dashboard --view …` | remove | not ported | No silent aliases; `errors.go` suggests new shape |

---

## Import and cutover

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| (N/A) | `import python-db` | N/A | done | One-time migration; not a runtime compatibility layer |
| Python `devdb` console script | Go binary in `golang/` | redesign | partial | M14 cutover: docs/skill still reference Python verbs |

---

## Raw table access

| Python behavior | Go command | Class | Status | Notes |
|-----------------|------------|-------|--------|-------|
| `list TABLE` | `list TABLE` | keep | done | Allowed tables documented in `list --help` |
| `show TABLE ID` | `show TABLE ID` | keep | done | |

---

## Python-only storage (intentionally skipped)

| Table / feature | Class | Status | Rationale |
|-----------------|-------|--------|-----------|
| `code_tightening` | remove | not ported | Low production use; feedback + review cover convention debt |
| `improvement_suggestions` | remove | not ported | Merged into `feedback` with `category=idea` |
| `loc_snapshots` | remove | partial | Historical LoC trends dropped; `inventory loc` is live-only |
| `agent_documents` | remove | not ported | Experimental; no Go consumer |
| `anchors` | remove | not ported | Unused in production dogfood |
| `entity_links` / `project_links` | remove | not ported | `link-source` not ported; plan scope + feedback context suffice |

---

## Output contract parity

| Contract | Python | Go | Status |
|----------|--------|-----|--------|
| Write stdout (human) | Bare entity id | Bare entity id | done |
| Write stdout (`--json`) | Mixed / inconsistent | `{"id":…}` + stable metadata | done |
| Write detail / hints | Often mixed into stdout | stderr via `Writer.Hint` | redesign | Stricter in Go |
| Read `--json` | Partial | All data commands | done | See `m12_test.go` JSON contract table |
| Global `--all` on lists | Per-command | Wired on list reads + `report` | done | M12 |
| Unknown command telemetry | `missed_cli_calls` | `missed_cli_calls` + suggestion | done | |

---

## Golden output fixtures

Human and JSON reference outputs for core reads and one write per domain live in:

- `golang/internal/cli/testdata/golden/` — file-backed expected output
- `golang/internal/cli/golden_test.go` — normalizes volatile ids/git before compare

Commands covered: `status`, `report`, `resume`, `plan item start` (work-on equivalent), and domain write verbs.

---

## Sign-off

| Artifact | Location | Reviewed |
|----------|----------|----------|
| Parity matrix (this file) | `docs/go-native-parity-matrix.md` | 2026-06-16 |
| Schema + importer mapping | `docs/go-native-schema-importer-mapping.md` | 2026-06-16 |
| Golden fixtures | `golang/internal/cli/testdata/golden/` | 2026-06-16 |

M1 exit criteria are satisfied retroactively: command grammar is frozen in the implementation plan HTML, behavioral parity is tabulated here, legacy features are classified, and golden examples enforce the output contract for primary agent entry points.
