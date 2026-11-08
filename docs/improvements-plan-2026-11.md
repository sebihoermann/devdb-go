# devdb-go improvements — implementation plan (2026-11)

Implementation-ready companion to the usage analysis that produced findings F1–F9.
The analysis lives in the `devdb-go-improvements` project ledger as the discussion
record; this document is the executable plan. Nothing here is speculative: every
item names the files to touch and an acceptance check that can be run.

## How to read an item

Each item has:

- **Feedback** — the ledger finding it closes (short id, so it can be resolved).
- **Why** — the observed defect, not a restatement of the title.
- **Files** — the exact paths that change. Roles: `create`, `modify`.
- **Steps** — the ordered change.
- **Acceptance** — a command whose output proves it. A criterion is only met
  when its command was run and its output pasted as evidence.
- **Effort** — implement + test time for one experienced Go dev.

## Baseline (must be true before item 1)

> **Superseded — re-verified 2026-11-08 after M1–M6 landed.** The table below was
> the state *before* item 1. It is kept because it is the reason 1.1 came first:
> a red baseline hides new regressions. The current state follows it.

**Original baseline, verified on `main`, 2026-11-08 (pre-M1):**

| Check | Result (then) | Result now |
|---|---|---|
| `go build ./...` | clean | clean |
| `go test ./...` | **FAIL** on `internal/domain/hub` (2 tests) | **0 failures**, all packages ok |
| packages passing | 27 | 28 |
| `devdb --version` | `unknown flag: --version`, rc=1 | works (`devdb version dev`) |
| `devdb feedback list` flags | `--status` only | `--status/--category/--severity/--role/--context/--limit` |
| `devdb plan acceptance list` | prints help, rc=0 | lists criteria (`--plan/--plan-item/--open`) |
| `devdb archive list` | exists, `--table/--since/--until/--limit` | unchanged, plus `archive show` |
| `cmd/devdb-openclaw` tests | `[no test files]` | has test files |
| Host-env dependence | hub tests fail where `/etc/devdb/config.json` exists | hermetic — `ConfigSearchPathsForTest` + pinned `DEVDB_CONFIG` |

**The baseline itself is broken, so item 1.1 fixes it first.** Every other item's
acceptance writes a *new* green line into a suite that already has a red one;
without 1.1 a regression is indistinguishable from the pre-existing failure.

Host-level note for the fixing dev: the two hub failures only reproduce where a
system-wide devdb config file exists. A machine without one sees green and will
not reproduce the report — that is the bug, not a mis-report. See 1.1.

**A second host-env leak of the same class was found while closing the findings
and fixed in `b26d63d`:** an ambient `DEVDB_MODEL_ID` broke two M13 golden
fixtures that carry `"model_id": "unknown"`. `internal/cli`'s `TestMain` now
unsets it, so `go test ./...` is green on a host that exports the variable.

---

## Milestone M1 — Make the suite trustworthy

Payoff: every later item gets a real regression gate.

### 1.1 Hermetic hub tests (feedback `34656eb3`)

**Why.** `hub.configSearchPaths()` ends with the system config path. The two
default-path tests assert the `$HOME/.devdb/metadata.db` default but let that
search run, so on any machine with a system config the cached path wins and both
assertions fail. `go test ./...` is red before anyone changes a line.

**Files**
- `internal/domain/hub/paths_test.go` — `modify`
- `internal/domain/hub/service_test.go` — `modify`
- `internal/domain/hub/paths.go` — `modify` (only to extract the search list)
- `internal/domain/hub/main_test.go` — `create`

**Steps**
1. Add `main_test.go` with a `TestMain` that sets `DEVDB_CONFIG` to a path under
   `t.TempDir()`-equivalent (a `os.MkdirTemp` cleaned in `defer`) so no real
   config participates. Restore the old env value afterwards.
2. In `paths.go`, export a test-only seam `ConfigSearchPathsForTest(xdg, home, sys string) []string`
   returning the candidate list for supplied roots instead of reading env/`os.UserHomeDir`.
   Keep the production `configSearchPaths()` delegating to it.
3. Rewrite the two failing tests to assert against that seam with explicit roots,
   not the ambient environment.
4. Add one test that the system-config path *is* in the list and is lowest priority.

**Acceptance**
```
go test ./internal/domain/hub/ -count=1        → ok
DEVDB_CONFIG= go test ./... -count=1           → no FAIL lines, ≥27 ok
```
Also: temporarily create a config file at the system path in a container and
confirm the suite stays green (proves the leak is closed, not masked).

**Effort** 45 min.

### 1.2 Stop test invocations polluting `missed_cli_calls` (feedback `2720b1b9`)

**Why.** In-process CLI tests drive `cli.Execute()` with a temp `--db`, so every
intentional failure is recorded as a real miss through `recordMissedFromError`.
The result: 956 of 1068 telemetry rows carry `cwd=<repo>/internal/cli`, top
"missed command" is the fixture `scaffold-plan Title`, and `analytics summary`
reports a 7-day miss count that measures the test suite. Telemetry is the input
to every DX decision, so as it stands the input is noise.

**Files**
- `internal/domain/analytics/missed.go` — `modify`
- `internal/cli/commands.go` — `modify`
- `internal/cli/main_test.go` — `create`
- `internal/domain/analytics/missed_test.go` — `modify`

**Steps**
1. Add `analytics.RecordMissedCall` guard: return early when the resolved `cwd`
   is inside a Go source tree (`go.mod` at or above cwd) **and** the process is a
   test binary. Detect the test binary via a package-level
   `var Enabled = true` that `main_test.go` sets to `false` — do not sniff
   `os.Args[0]`, which is fragile under `go test -c`.
2. Belt and braces: in `commands.go`, skip `recordMissedFromError` when
   `analytics.Enabled == false`.
3. Cover both paths in `missed_test.go` (enabled writes, disabled does not).
4. Do **not** delete existing rows in this item — 1.3 is the cleanup.

**Acceptance**
```
go test ./internal/cli/ ./internal/domain/analytics/ -count=1   → ok
python3 - <<'PY'   # count rows created during the test run
# snapshot count, run go test ./..., snapshot again → delta must be 0
PY
devdb analytics summary                                         → 7d misses drop after 1.3
```

**Effort** 1 h.

### 1.3 Purge test-origin telemetry and add an origin column (feedback `2720b1b9`)

**Why.** Even with 1.2 in place, ~1,000 historical rows keep the DX signal
unusable for months. They are identifiable but not labelled, so every future
query has to re-derive the filter.

**Files**
- `internal/migrate/*.go` — `modify` (new migration)
- `internal/domain/analytics/missed.go` — `modify`
- `internal/cli/commands.go` — `modify`

**Steps**
1. Migration: `ALTER TABLE missed_cli_calls ADD COLUMN origin TEXT NOT NULL DEFAULT 'cli'`
   with values `cli | test | probe`. Follow the existing migration numbering.
2. `RecordMissedCall` takes the origin from the same switch as 1.2.
3. One-shot backfill in the same migration: rows whose `cwd` matches
   `%/internal/cli` or `%/internal/%` and whose `normalized_command` matches a
   known fixture (`scaffold-plan Title`, `--plan no-such-plan`, `/tmp/Test*`) are
   marked `origin='test'`.
4. `analytics missed` and `analytics summary` default to `WHERE origin='cli'`,
   with `--all-origins` to include the rest.

**Acceptance**
```
devdb analytics summary                       → 7d total excludes origin='test'
devdb analytics missed --all-origins --limit 5 → still shows the old rows
devdb analytics summary                       → top command is no longer a test fixture
```

**Effort** 1 h 15 m.

---

## Milestone M2 — CLI surface correctness

Payoff: agents stop guessing flags; failures become loud.

### 2.1 `feedback list` filters (feedback `a18974ce`)

**Why.** The table carries `category`, `severity`, `context`, `role`; the verb
exposes only `--status`. Every real retrospective query ("high dx items",
"everything about file X", "last 5") is a raw SQL detour.

**Files**
- `internal/cli/commands.go` — `modify` (`cmdFeedbackList`)
- `internal/domain/feedback/service.go` — `modify` (`List` gains a filter struct)
- `internal/cli/feedback_list_filters_test.go` — `create`

**Steps**
1. Replace `List(db, status string, limit int)` with
   `List(db *sql.DB, f ListFilter) ([]Row, error)` where `ListFilter` has
   `Status, Category, Severity, Context, Role string; Limit int`. Keep a thin
   `ListByStatus` wrapper so the existing callers and tests compile unchanged.
2. Build the WHERE clause by appending `AND col = ?` per non-empty field; keep
   the `all` sentinel meaning "no status filter".
3. Flags on the verb: `--category`, `--severity`, `--context` (substring match,
   `LIKE %v%`), `--role`, plus existing `--status`; `--limit` comes from the
   global list-limit machinery already used by `effectiveListLimit`.
4. Validate `--severity` and `--role` through the existing `validateEnum`
   helper so a typo is `ExitUsage`, not an empty result.
5. Add `--limit` documentation to `devdb help feedback list --examples`.

**Acceptance**
```
devdb feedback list --severity high --category dx --limit 5 --json   → ≤5 rows, all match
devdb feedback list --severity high --category dx --limit 5           → same rows, human mode
devdb feedback list --severity nope                                   → rc=2, names allowed values
go test ./internal/cli/ -run FeedbackList -count=1                    → ok
```

**Effort** 1 h 30 m.

### 2.2 `plan acceptance list` + fail-closed unknown flags (feedback `cfe4f72a`)

**Why.** `plan acceptance` has `add|backfill|meet` and no read verb, so the only
way to see criteria is `plan item show`. Worse, `devdb plan acceptance list` —
a verb that does not exist — prints the parent help and **exits 0**, so an agent
piping it to `jq` gets a silent empty result. A nonexistent subcommand must never
be success. (`--plan-item` and `--bogus` already exit 1; the help-fallback path
is the hole.)

**Files**
- `internal/cli/commands_planning.go` — `modify`
- `internal/domain/planning/service.go` — `modify` (add `ListAcceptance`)
- `internal/cli/plan_acceptance_list_test.go` — `create`
- `internal/cli/commands.go` — `modify` (help-fallback exit code)

**Steps**
1. `ListAcceptance(db, filter)` returning criteria joined to their item and plan:
   `SELECT pa.id, pa.plan_item_id, pi.title, pa.ordinal, pa.criterion, pa.status,
   COALESCE(pa.evidence,'') FROM plan_item_acceptance pa JOIN plan_items pi ...`
   Filters: `--plan-item <id|prefix>`, `--plan <slug|id>`, `--status open|met|all`.
2. Reuse `planning.ResolvePlanID` and the existing id-prefix resolver so an
   unknown ref fails closed with "not found" (same rule as the LIST-verb fix).
3. Root-cause the exit-0: a parent command whose `RunE` is the help printer
   returns nil for an unknown subcommand. Set `RunE` on `plan acceptance` to
   return `unknownCommandError(...)` when args are non-empty, and apply the same
   guard to every group command that has subcommands (audit the tree: `plan`,
   `feedback`, `review`, `hub`, `archive`, `analytics`, `inventory`, `task`).
4. Test matrix: for each group command, `X nosuchverb` must exit non-zero.

**Acceptance**
```
devdb plan acceptance list --plan-item <id>          → criteria rows, rc=0
devdb plan acceptance list --plan no-such-plan       → rc=3, "plan not found"
devdb plan acceptance list                           → rc!=0 (unknown subcommand)
devdb plan nosuchverb                                → rc=2
go test ./internal/cli/ -run 'AcceptanceList|UnknownSubcommand' -count=1 → ok
```

**Effort** 2 h.

### 2.3 `--version`, human empty lists, role enum (feedback `c69967c6`)

**Why.** Three small friction points, all verified: `devdb --version` is
`unknown flag` rc=1; every empty list prints `[]` in human mode while
`review list` prints "no findings"; `feedback add --role agent` leaks
`constraint failed: CHECK constraint failed: role IN (...)` with rc=1 instead of
the friendly `--role is required` used when the flag is absent.

**Files**
- `internal/cli/commands.go` — `modify` (version var, role validation)
- `internal/output/writer.go` — `modify` (human empty phrasing)
- `internal/cli/version_test.go`, `internal/output/writer_test.go` — `create`/`modify`
- `.github/workflows/ci.yml` — `modify` (inject version at build)

**Steps**
1. `var Version = "dev"` in package `cli`; `root.Version = Version`; add
   `--version` to the root command (cobra provides the flag once `Version` is
   set, printing `devdb version X`).
2. Build injection: `go build -ldflags "-X .../internal/cli.Version=$(git describe --tags --always --dirty)"`
   in CI and in the installer script.
3. Human empty output: add `output.EmptyText` alongside `HumanLines`; list verbs
   pass their own phrase ("no feedback", "no findings", "no plans"). Default to
   `[]` only where no phrase is set, so the JSON contract is untouched.
4. Validate `--role` with `validateEnum` **before** the insert, so a bad value
   returns `ExitUsage` and the same message shape as a missing one.
5. Assert the JSON path still emits `[]` for empty results (existing test
   `TestPrintDataJSONNilSliceEmitsEmptyArray` must stay green).

**Acceptance**
```
devdb --version                  → "devdb version <something>", rc=0
devdb feedback list              → "no feedback", not "[]"
devdb feedback list --json       → [], rc=0
devdb feedback add x --role agent→ rc=2, "--role: invalid value ... allowed: user, model, codebase"
```

**Effort** 1 h.

---

## Milestone M3 — Data model and hub

### 3.1 Archive-aware reads (feedback `4da5b607`)

**Why.** Archive is now the bulk of the data (3,477 archived rows vs 84 live
feedback in the largest project). `devdb archive list` exists, but every other
read path — `hub audit`, `hub dashboard`, checkers — queries live tables only, so
a mostly-archived project reads as clean and a closed finding reads as vanished.

**Files**
- `internal/domain/hub/audit.go` — `modify`
- `internal/domain/hub/snapshot.go` — `modify`
- `internal/domain/archive/service.go` — `modify` (add `Union` helper)
- `internal/domain/hub/audit_archived_test.go` — `create`

**Steps**
1. Add `archive.UnionCounts(db) (map[string]int, error)` — per-`source_table`
   counts from `archive_entries`, one query, used by both audit and snapshot.
2. Extend `AuditReport` with `archived` counts per project and render them in
   `formatAudit` as `live=N archived=M`.
3. Snapshot: carry `archived_count` per project so `--cached` mode keeps parity.
4. Do **not** change default filtering of high-feedback sections in this item —
   that is 3.2's call, and changing it here would flood every dashboard.

**Acceptance**
```
devdb hub audit --json   → every project entry has live and archived counts
devdb hub audit          → human output shows "archived=3477" for the largest project
go test ./internal/domain/hub/ -count=1 → ok
```

**Effort** 2 h.

### 3.2 Reach archived rows from the plan tree (feedback `4da5b607`, part 2)

**Why.** `plan item show` on an archived item returns "no match for id prefix"
even though the row exists. The only read path is `archive list --table plan_items`
plus manual JSON parsing.

**Files**
- `internal/domain/archive/service.go` — `modify`
- `internal/cli/commands_hygiene.go` — `modify`
- `internal/domain/archive/union_read_test.go` — `create`

**Steps**
1. `archive.GetByID(db, idPrefix) (Entry, error)` scanning `payload_json`.
2. `devdb archive show ID` — resolves a live row first, then archive, and prints
   a `source: archive (<reason>, <date>)` header when it came from archive.
3. Teach `plan item show` to fall back to `archive.GetByID` and print
   `[archived]` in the header plus a pointer to `devdb archive restore`.
4. Document the fallback in `skills/devdb/SKILL.md` so agents stop writing
   `payload_json LIKE '%id%'` queries.

**Acceptance**
```
devdb archive show <archived-item-id>   → the row, header says archived
devdb plan item show <archived-id>      → row + "[archived]" + restore hint
devdb plan item show <live-id>          → unchanged output
```
with the archived id taken from `devdb archive list --table plan_items --json`.

**Effort** 2 h 30 m.

### 3.3 Auto-register ledgers in the hygiene run (feedback `845ca1a7`)

**Why.** `hub doctor --scan` correctly flags unregistered ledgers — 7 under one
project root on this box — and nothing acts on it, so drift returns after every
new clone. Detection without repair is a permanent chore.

**Files**
- `internal/cli/commands_hub.go` — `modify` (`register --auto` gains `--dry-run`)
- `scripts/` or the operator's hygiene script — `modify` (call the verb)
- `internal/domain/hub/auto_register_test.go` — `modify`

**Steps**
1. `hub register --auto --scope DIR` already exists; add `--dry-run` and make it
   idempotent for an already-registered path (no duplicate alias, no error).
2. Alias derivation: directory basename; on collision append `-2`, `-3`; never
   silently overwrite an existing alias pointing elsewhere.
3. Wire into the weekly hygiene script **before** the archive pass, so newly
   found ledgers are archived in the same run; log the count registered.
4. Skip and report directories that are not writable by the running user rather
   than failing the whole run (mirror the existing `SKIPPED (ledger not writable)`
   behaviour).

**Acceptance**
```
devdb hub doctor --scan <dir>        → N unregistered
devdb hub register --auto --scope <dir> --dry-run   → same N, no writes
devdb hub register --auto --scope <dir>             → 0 unregistered after
devdb hub doctor --scan <dir>        → "no unregistered .devdb repos found"
```

**Effort** 2 h.

---

## Milestone M4 — Harness integration

### 4.1 Decide `devdb-openclaw` (feedback `ec923264`)

**Why.** The package ships a real 7-verb CLI (`version, list, links, sync,
friction scan, doctor, schedule`). `cmd/devdb-openclaw` is a 4-line `main` with
**no test files**, the binary is not installed anywhere, and `schedule` writes two
daily cron jobs (`17 3 * * *`, `13 6 * * *`) that shell out to that missing
binary — running it on a host creates two silently failing jobs every day.

**Decision to record (pick one, in the item close note):**
- **Ship it.** Build + install + a `doctor` gate before `schedule` may run.
- **Trim it.** Remove the `schedule` verb so the package cannot create broken
  crons, and keep the rest as a library.

Recommendation: **trim first, ship later.** `schedule` is the only verb with a
side effect outside the ledger, and it is the one driving the broken-cron risk.
The `friction scan` verb is the valuable part (it greps session notes and writes
deduplicated feedback), and it needs no cron inside devdb — an operator-level
scheduler already exists on this box and is the right owner of the timing.

**Files**
- `internal/integrations/openclaw/command.go` — `modify`
- `internal/integrations/openclaw/schedule.go` — `modify`/`delete`
- `internal/integrations/openclaw/schedule_test.go` — `modify`
- `cmd/devdb-openclaw/main.go` — `modify` (only if shipping)

**Steps (trim)**
1. Remove `schedule` from the verb table; keep `schedule.go` behind a build tag
   or delete it with its test.
2. `friction scan` gains `--dry-run` and `--since`, and writes through
   `feedback.Add` (so `model_id` is stamped) instead of raw SQL.
3. Add a package-level test that `Execute()` with `schedule` returns an unknown
   verb, so the decision cannot silently regress.

**Acceptance**
```
devdb-openclaw schedule            → rc!=0, unknown verb
devdb-openclaw friction scan --dry-run → proposed rows, no writes
go test ./internal/integrations/openclaw/ -count=1 → ok
go vet ./cmd/devdb-openclaw/       → clean
```

**Effort** 1 h 30 m.

### 4.2 Retire raw-SQL guidance in the session-artifacts skill (feedback `1c6c5967`)

**Why.** The skill instructs agents to insert feedback/feature rows with raw
`sqlite3` because "the CLI has no command to insert raw rows" — false since
`feedback add` and `feature add` existed. Raw inserts bypass `DEVDB_MODEL_ID`, so
89% of fleet feedback rows carry `model_id='unknown'`, which hollows out the audit
trail.

**Files**
- the `devdb-log-session-artifacts` skill (operator-owned, outside this repo) — `modify`
- `skills/devdb/SKILL.md` — `modify` (cross-reference)
- `internal/domain/feedback/service.go` — `modify` (only if a gap appears)

**Steps**
1. Replace the raw-SQL section with `devdb feedback add` / `devdb feature add`
   invocations, including the `--context` flag that carries the file/section.
2. Where a batch insert is genuinely needed, add `devdb feedback import` usage
   rather than a Python snippet.
3. Add a line to `skills/devdb/SKILL.md`: any write path that bypasses the CLI
   loses `model_id`, so it is not allowed for agent-authored rows.
4. Verification loop: after the edit, make one real row through the documented
   path and confirm `model_id` is not `unknown`.

**Acceptance**
```
devdb feedback list --json | jq '.[0].model_id'    → a real model id, not "unknown"
grep -c 'sqlite3.connect' <skill file>             → 0 in the workflow section
```

**Effort** 45 min.

---

## Milestone M5 — Reconcile vs archive (new, feedback `4f7fc1a7`)

This is the only **high-severity correctness** defect in the set and it was
missing from the original backlog.

### 5.1 Reconcile must count archived children (feedback `4f7fc1a7`)

**Why.** `milestoneStatusFromChildren` runs
`SELECT status, COUNT(*) FROM plan_items WHERE milestone_id=? GROUP BY status`.
After the hygiene run archives a completed plan's items, that query returns zero
rows, so reconcile computes no expected status and leaves the milestone — and the
plan — stuck `active` with zero live items. Observed consequence: a plan that was
archive-to-completion cannot be reconciled to `done`; the workaround was a direct
`UPDATE milestones SET status='done'`, which is exactly the state reconcile
exists to prevent.

**Reproduction (write this into the item close note):**
1. Create a plan with one milestone and one item, two acceptance criteria.
2. Meet both, `plan item close`.
3. `devdb archive run --yes` (moves the item and its children out of live tables).
4. `devdb plan reconcile --plan <slug>` → reports **no drift** while
   `plan list` still shows `active`.

**Files**
- `internal/domain/planning/reconcile.go` — `modify`
- `internal/domain/planning/reconcile_archive_test.go` — `create`

**Steps**
1. In `milestoneStatusFromChildren`, when the live query returns `total == 0`,
   run a second query over archived children:
   `SELECT json_extract(payload_json,'$.status'), COUNT(*) FROM archive_entries
    WHERE source_table='plan_items'
      AND json_extract(payload_json,'$.milestone_id')=? GROUP BY 1`
   (JSON1 is available — `modernc.org/sqlite` is built with it; verified by
   `json_extract` against a production ledger).
2. Merge live + archived counts before deriving the status, so a partially
   archived milestone still resolves correctly.
3. Same treatment in `planStatusFromMilestones` for milestones that were
   themselves archived to completion.
4. Keep the existing "parent plan is done → milestone done" shortcut; it becomes
   a fallback, not the only path.
5. Guard: if a milestone genuinely has no children live or archived, return ""
   as today — do not invent `done`.

**Acceptance**
```
go test ./internal/domain/planning/ -run Reconcile -count=1   → ok
# reproduction above, step 4 then:
devdb plan reconcile --plan <slug> --apply                    → reports the drift, fixes it
devdb plan list --json | jq '.[] | select(.slug=="<slug>") .status'  → "done"
# non-regression:
devdb plan reconcile --plan <fully-live-plan> --apply         → "no plan-tree drift detected"
```

**Effort** 2 h.

---

## Milestone M6 — Write-path reliability

### 6.1 Extend the SQLITE_BUSY retry to acceptance-meet (feedback `a0033eeb`)

**Why.** `internal/storage/db.go` already sets `MaxOpenConns(1)`, `busy_timeout=5000`
and a jittered bounded retry covering both Open PRAGMA setup and `WithTx` commit,
with `TestWithTxRetriesBusyTransaction` and `TestJitterBusyDelaySpreads` proving it,
and the ledger stress script verified 24/24 parallel writers with 0 BUSY. The one
unclosed sibling is **parallel `plan acceptance meet` from separate processes**,
which still surfaces `SQLITE_BUSY` and is mitigated only by a rule in the skill
("run meets sequentially, not in parallel"). A documentation rule is the wrong
control for a write path the retry already fixes everywhere else — the rule will
be forgotten, the retry will not.

**Files**
- `internal/storage/db.go` — `modify` (export the retrying tx helper if not already)
- `internal/domain/planning/service.go` — `modify` (meet goes through it)
- `internal/storage/acceptance_busy_test.go` — `create`
- `skills/devdb/SKILL.md` — `modify` (drop the sequential rule)
- `scripts/concurrency/stress_ledger.py` — `touched` (standing proof)

**Steps**
1. Route the acceptance-meet write through the same retrying transaction helper
   used by `WithTx` instead of a bare `Exec`.
2. Regression test: N ≥ 8 goroutines with their own connections meet criteria on
   one ledger concurrently; assert zero `SQLITE_BUSY` and all criteria met.
3. Only once that test is green, remove the sequential-discipline rule from the
   skill. Removing it first would trade a known-safe slow path for a silent race.
4. Re-run the concurrency stress script as the standing proof.

**Acceptance**
```
go test ./internal/storage/ -count=1                     → ok
go test ./internal/storage/ -run AcceptanceBusy -count=3 → ok, no BUSY errors
grep -c 'sequentially' skills/devdb/SKILL.md            → 0 in the meet section
python3 scripts/concurrency/stress_ledger.py            → full success, 0 BUSY
```

**Effort** 1 h 30 m. **Depends on** 1.1 (needs a green baseline to read the result).

---

## Sequencing, dependencies, effort

| # | Item | Depends on | Effort |
|---|---|---|---|
| 1 | 1.1 hermetic hub tests | — | 45 m |
| 2 | 1.2 telemetry test guard | — | 1 h |
| 3 | 1.3 telemetry origin column | 1.2 | 1 h 15 m |
| 4 | 2.3 version / empty / role | 1.1 | 1 h |
| 5 | 2.1 feedback filters | — | 1 h 30 m |
| 6 | 2.2 acceptance list + fail-closed | — | 2 h |
| 7 | 5.1 reconcile vs archive | 1.1 | 2 h |
| 8 | 3.1 archive-aware audit | — | 2 h |
| 9 | 3.2 archive read path | 3.1 | 2 h 30 m |
| 10 | 3.3 auto-register in hygiene | — | 2 h |
| 11 | 4.1 devdb-openclaw decision | 1.2 | 1 h 30 m |
| 12 | 4.2 skill raw-SQL retirement | 4.1 | 45 m |
| 13 | 6.1 meet retry | 1.1 | 1 h 30 m |

**Total:** 13 items, ~19.5 h, roughly 2.5 focused days.

**Order rationale.** 1.1 first because a red baseline makes every later gate
meaningless. 1.2 before 1.3 because the column is pointless until new rows stop
being polluted. 5.1 sits high despite being M5 because it is the only
high-severity correctness bug and it silently strands completed plans. Item 4.2
is last only because it depends on the 4.1 decision about which write path the
harness uses.

**Parallel lane.** Items 5, 6, 8 and 10 touch disjoint packages
(`planning`, `cli`, `hub`) and can run concurrently if separate branches are used.
`internal/cli/commands.go` is touched by 2.1, 2.2 and 2.3 — serialize those three.

## Repo conventions these items must follow

- `gofmt` clean; the tree was normalized in `52efaf1`.
- Tests are in-package for domain logic, black-box in `internal/cli`.
- New list verbs return `[]` under `--json` and a phrase in human mode (2.3 sets
  the helper; do not hand-roll it afterwards).
- Any new reader of a live table must state whether it also reads
  `archive_entries`. That omission is the cause of two of these findings.
- CI currently runs `go test ./...` + `go build`. Once 1.1 lands, add
  `-count=1` so a cached pass cannot mask a regression.

## Closing the findings

Each item's close note carries the command output that proved it, and closes the
matching feedback row with `devdb feedback close <id> --proposed-fix "<commit>"`.
The mapping is one-to-one: F1 `2720b1b9`, F2 `a18974ce`, F3 `cfe4f72a`,
F4 `c69967c6`, F5 `4da5b607`, F6 `845ca1a7`, F7 `ec923264`, F8 `1c6c5967`,
F9 `a0033eeb`, plus `4f7fc1a7` (reconcile) and `34656eb3` (hermetic tests). F10 `5ceeb9a4` is a
documentation correction, not code: the analysis article byline credited the model
most common in the ledger instead of the session runtime — the same class of error
as F8, fixed by editing the article, not the repo.