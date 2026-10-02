# Trustworthy Ledger

**Plan:** `f8dec91739f6e8af30d9d136b56d1849` (slug `trustworthy-ledger`)
**Ledger:** `.devdb/development.db` (this repo — previously empty)
**Scaffolded:** 2026-10-02 · 6 milestones · 29 items · 115 acceptance criteria · 66 scoped files

This document is the narrative. The ledger is the source of truth. If they
disagree, the ledger wins and this file is wrong.

```bash
devdb plan show trustworthy-ledger      # milestone header
devdb plan tree trustworthy-ledger      # full hierarchy
devdb plan status trustworthy-ledger    # delivery counts
devdb plan item show <item-id>          # acceptance + file scope
```

---

## Thesis

The gap is not features. It is trustworthiness of the thing whose entire
purpose is to be trusted.

devdb is an instrument for recording what is known about a codebase so the next
agent does not have to re-derive it. Three findings from the 2026-10-02 audit
say that instrument currently cannot be trusted about itself, and that its own
memory has already been lost once.

### Finding 1 — the ledger is in the wrong repo, and half of it is gone

| Where | What |
|---|---|
| `devdb-go/.devdb/development.db` | **empty** — this repo has never kept its own state |
| `~/devdb/.devdb/development.db` | 114 feedback, 96 plan items, 26 plans, 22 arch notes, 38 verification runs — the *Python* repo |
| anywhere | **zero rows after 2026-06-18** |

All ten feedback ids cited in [`workflow-review-2026-06-24.md`](workflow-review-2026-06-24.md)
— `e9f47e9b`, `362f652e`, `bbec89e6`, `ad7073e8`, `030a4551` and the rest —
resolve to **no row in any surviving database**. The retrospective that is
currently driving the work rests entirely on citations that cannot be checked.

`CLAUDE.md` states this repo dogfoods itself. It did not. The target-repo rule
in `SKILL.md` was violated in the one repository where it mattered most, and
`.devdb/` is gitignored, so the ledger is unbacked machine state.

### Finding 2 — the tool reports state it has not verified

`status.Quality()` cannot return an error: eight aggregate `Scan` failures are
discarded and it then returns `q, nil`. `hub/snapshot.go` discards a
`migrate.RunAll` failure and computes a snapshot against a half-migrated
database, then labels it fresh. `hub` sync loops do `_, _ = SyncOne(...)`.
`hub/across` silently `continue`s past failed projects. `architecture` discards
`json.Unmarshal` on the hashes that staleness detection depends on. Roughly
twelve sites, same shape.

Net effect: **a schema error renders as a healthy all-zeros dashboard.** For a
trust instrument, silently wrong is worse than loudly broken.

### Finding 3 — the tests and CI are set below the bar they measure

- 63 files named `coverage_*` — 7,931 lines, 36% of test code — all landed in
  commit #1. Generated, not written.
- 809 test functions, **44** `t.Run` subtests.
- `TestInProcessMegaCoverage` fires 55 CLI commands with zero assertions.
- `TestErrorPathsClosedDB` exists 13 times, asserting `database/sql` returns
  `ErrDBClosed`. The stdlib is being tested.
- 14 tests assert nothing at all.
- **749 discarded errors in test code.** `planning/coverage_final_test.go` does
  `planID, _ := CreatePlan(...)` then asserts on `planID` — if setup fails,
  every assertion is vacuous and the test is green.
- 86.5% line coverage measures how many branches a generator enumerated.

CI is 26 lines: `go test ./...` and one `go build`. No vet, no gofmt (43 files
are unformatted), no lint, no `-race`, no coverage, no matrix. It never builds
`cmd/devdb-openclaw` (0% coverage). `git tag -l` returns **zero tags**, so the
README's `go install ...@latest` cannot work.

And the live bug CI is blind to: `storage.Open` applies
`journal_mode = WAL` *before* `busy_timeout = 5000`. WAL takes an exclusive
lock, so it runs with a zero timeout. Reproduced at ~50% failure rate on
full-suite runs. The repo's own retrospective diagnosed this exact case and
left it open with an "Action: reopen" that never happened.

---

## Milestones

Ordered by leverage, not by size. M1 and M3 are the ones that make everything
else trustworthy.

### M1 · Durability and citation integrity — `c8ae173c`

Fix the thing that already went missing, then build the verb that would have
caught it.

| Item | Title |
|---|---|
| `7b81e052` | Export the ledger to a committed diffable JSONL |
| `4b1a933e` | Migrate the surviving Python-repo ledger into devdb-go |
| `3532398d` | Add `devdb integrity check` |
| `b7301826` | Add `devdb verify audit` |
| `f04ae74f` | Doctor checks for an empty or abandoned ledger |

`integrity check` cross-checks every citation the ledger and the docs make:
`--evidence` and `--commit` values against real git history, arch-note sources
against `repo_files`, feedback ids referenced from plans and markdown. It exits
non-zero on a dangling citation.

`verify audit` is the missing checker for the checker. The verification ledger
is the best idea in the codebase — a pass from yesterday is stale if any input
hash moved — but nothing ever audits it. It walks every recorded run,
re-derives current input hashes, and reports which recorded passes have
expired.

### M2 · Honest state: warnings instead of silence — `1040c561`

The largest internal change in the plan, and the best value per line. Same
work, opposite outcome: honest degradation instead of fake health.

| Item | Title |
|---|---|
| `e3ae9286` | Add `warnings` as a first-class field in the JSON envelope |
| `58a5cd91` | Convert discarded errors into warnings |
| `77b9cd7b` | Fix dead error returns and fragile error matching |
| `45e07fd7` | Stop dropping model provenance |
| `dcf6e853` | Reconcile `plan item pause` with its documented behaviour |

Every one of those `_ = Scan(...)` sites becomes a collected warning in the
response envelope. A missing table produces "I could not read `plan_items`",
not a dashboard of confident zeros.

`45e07fd7` matters more than it looks: four write paths accept a `modelID` and
never persist it, in a tool whose premise is recording which model did what.
The columns already exist. The data does not.

### M3 · CI and release that can fail — `68b91a07`

Start with the live bug, then build the pipeline that catches the next one.

| Item | Title |
|---|---|
| `445210c7` | Fix PRAGMA ordering and add the regression test |
| `d07d1e7b` | Make CI able to fail |
| `e2ee8835` | Add a release workflow so the documented install works |
| `b39f803a` | Dependency hygiene |

`445210c7` is a one-line reorder with a test that runs the full suite 20 times.
It is the smallest item in the plan and the only one that is a live production
defect today.

`e2ee8835` is a product bug, not CI hygiene: with zero tags, a first-time user
who follows the README gets nothing at all.

### M4 · Replace generated tests with real ones — `25fe602d`

| Item | Title |
|---|---|
| `2cabc4ef` | Delete the `coverage_*` files and record the delta |
| `b2043b51` | Eliminate ignored errors in test code |
| `3cc37ef5` | Delete tests that cannot fail |
| `233c5cec` | Fuzz the hand-rolled parsers |
| `65f19a06` | Add benchmarks and enforce formatting |

Delete first, measure second. The coverage number only becomes informative once
the generated layer is gone — record the delta as the new baseline.

`233c5cec`: `splitStatements` is a hand-rolled SQL lexer that has already been
broken once (`b950b52`). It does not handle `BEGIN...END` trigger bodies or
`X'blob'` literals, and a migration that hits either is silently truncated.
Go fuzzing is stdlib and three lines to start. There are zero fuzz targets.

### M5 · Restructure the CLI and retire dead dependencies — `81cd0fb2`

| Item | Title |
|---|---|
| `1654792d` | Split `internal/cli` by noun family |
| `fcfd3185` | Replace `map[string]any` in domain signatures with typed DTOs |
| `765a6314` | Port grasscutter to the Go AST and drop gpython |
| `ae60a8dc` | Deduplicate the migration runner and the id resolver |
| `f1a5f882` | Add a JSON contract test across the whole verb surface |

`1654792d`: 4,640 lines in one flat package with four package-level mutable
globals, which is why `cli.Execute` is non-reentrant and why tests hand-reset
`os.Args` and flags instead of running in parallel.

`765a6314`: `grasscutter`'s entry point is
`SELECT path FROM repo_files WHERE path LIKE '%.py'`. On a Go codebase it
structurally cannot return a candidate. It is the sole reason a frozen,
effectively unmaintained dependency tree sits in `go.mod`. Porting to `go/ast`
+ `go/packages` is stdlib-only, and `dead`/`inlinable` come free from the
toolchain the repo already builds with.

`f1a5f882` is the cheap insurance policy: a generated test that walks the cobra
tree and asserts every one of the 123 command nodes accepts `--json` and emits
a valid envelope. It would have caught the swallowed-error class on day one.

### M6 · Product surface — `87239c2e`

| Item | Title |
|---|---|
| `d2d12c2d` | Collapse the session-start ritual into `devdb start` |
| `821ad1fd` | Shrink the verb surface |
| `7c6a57cc` | Invert the JSON contract |
| `0bf3eac7` | Close the documentation loop |
| `54270992` | State a concurrency model |

`d2d12c2d`: the tool's own session ritual is six to ten commands. `hub audit`
already replaced thirty-plus manual reads; the per-project ritual deserves the
same collapse. A tool whose own ritual gets skipped is a tool that does not get
used.

`0bf3eac7` is the self-referential one: every `Action:` item in the
retrospective is currently a markdown line with no mechanism turning it into
tracked work. `feedback import markdown` already exists. The loop should
terminate in the ledger, not in prose — and this file, plus this plan, is the
first time it has.

`54270992`: the documented workaround for the SQLITE_BUSY bug is "run meets
sequentially". A CLI used by parallel agents on one repo needs a decision —
single-writer lock file, or properly concurrent access — not a workaround
bolted onto an unfixed ordering bug.

---

## Working this plan

Per `skills/devdb/SKILL.md` — session ritual, then bracket, then close with
evidence:

```bash
devdb plan item start <item-id>
# ... work ...
devdb plan acceptance meet <criterion-id> --evidence <commit-sha>   # one at a time, sequentially
devdb plan item close <item-id> --evidence <commit-sha>
```

Evidence names the artifact, not the behaviour:

```bash
devdb feedback close <id> --proposed-fix \
  "Resolved by <commit> which adds <artifact>: <one-line description>"
```

**Sequential meets, not parallel.** `445210c7` exists because parallel writes
still race. Fix M3 first, then the workaround can be retired by `54270992`.

Suggested entry point: `445210c7` (the live bug, one line, ~an hour including
the test), then `d07d1e7b` (so nothing else like it ships), then `7b81e052`
and `4b1a933e` (so this plan's own tracking stops depending on unbacked machine
state).

## Maintenance

This file is narrative. The ledger is authoritative. When they diverge, fix
this file from the ledger:

```bash
devdb plan tree trustworthy-ledger --json
```

If this plan is ever completed or abandoned, archive it rather than deleting
the rows — the ledger is the record, and `archive restore` is reversible.
