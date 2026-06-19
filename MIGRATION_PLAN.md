# Migration Plan: Python devdb → Go devdb (`devdb-go`)

**Status:** ✅ Executed 2026-06-19
**Snapshot date:** 2026-06-19
**Source:** `/root/.openclaw/workspace/devdb` (Python) → `/root/.openclaw/workspace/projects/devdb-go` (Go)
**Targets (7):** `workspace`, `devdb`, `devdb-public`, `storyblender`, `trips`, `MostActive`, `mail-server`

---

## Execution summary

All 21 plan items completed. Final hub state: 8 projects registered, hub sync 8/8 passed. Binary: `/root/go/bin/devdb` (Go 1.26.0 built from `/root/.openclaw/workspace/projects/devdb-go`), `/usr/local/bin/devdb` symlinks to it. Python checkout moved to `/root/.openclaw/workspace/.archive/devdb-python-2026-06-19/`.

### What happened during execution

| # | Project | Source DB size | Pre-mig counts | Post-mig counts | Outcome |
|---|---------|---------------:|----------------|-----------------|---------|
| 1 | devdb-go | — | (fresh init, no python source) | `schema_kind=go v2` | ok |
| 2 | devdb | 1.2 MB | plan_items=12, feedback=34, goals=1, plans=2, repo_files=121, verification_runs=11, entity_links=5, anchors=5 | all preserved; entity_links + anchors archived | ok |
| 3 | devdb-public | 606 KB | plan_items=0, repo_files=108, no python-only populated | all preserved | ok |
| 4 | MostActive | 6.5 MB | plan_items=9, feedback=2, plans=1, repo_files=7000, verification_runs=2 | all preserved | ok |
| 5 | storyblender | 3.0 MB | plan_items=19, feedback=38, plans=3, repo_files=938, verification_runs=1, loc_snapshots=6541, entity_links=9, anchors=9 | migrated after importer patch (see issue §A below) | ok |
| 6 | trips | 528 KB | plan_items=11, plans=1 | all preserved | ok |
| 7 | mail-server | 549 KB | plan_items=54, plans=2, features=5 | all preserved | ok |
| 8 | workspace | 8.8 MB | plan_items=55, feedback=44, goals=5, plans=7, repo_files=9337, entity_links=30, anchors=30 | all preserved; entity_links + anchors archived | ok |

### Issues encountered & resolved

**§A — Importer status-enum gap (storyblender).** The importer's feedback-status CASE allowed `'resolved'` through unchanged (because `IN ('deferred','wontfix','resolved','closed','open')` includes `'resolved'`), but the Go CHECK constraint only accepts `('open','closed')`. First apply attempt failed at row 275: `copy feedback: constraint failed: CHECK constraint failed: status IN ('open','closed')`.

**Fix:** edited `devdb-go/internal/importer/python_ledger.go` to map non-open feedback status values explicitly:
```sql
CASE WHEN status = 'open' THEN 'open'
     WHEN status IN ('deferred','wontfix','resolved','closed') THEN 'closed'
     WHEN status IS NULL OR status = '' THEN 'open'
     ELSE 'closed' END
```
Same defensive pattern applied to `goals`, `milestones`, `plan_items`, `plan_item_acceptance`, `tasks`, `reminders` so any future unknown status values map to a safe default instead of crashing the import. Rebuilt binary and re-ran storyblender successfully. Storyblender also had a `plan_item_acceptance.status='wontfix'` row that the patched CASE handles (mapped to `'open'`).

**§B — devdb alias broke on archive.** Step §7.1 moved `/root/.openclaw/workspace/devdb` to `.archive/devdb-python-2026-06-19/`, but the `devdb` hub alias still pointed at the original path. After archive, `hub sync` failed for that one project. Re-registered the alias pointing at the new archived path (`hub register` updates an existing alias in place; no separate `unregister` verb in the Go CLI). Final sync: 8/8 passed.

**§C — Go test suite had 7 pre-existing chmod-as-root failures.** All in `TestInitDBMkdirFailure`, `TestOpenStatPermissionError`, `TestImportPythonDBMkdirFailure`, `TestApplyInPlaceImportFailureOnReadOnlyDir`, `TestOpenHubRunHubFailureOnReadonlyFile`, `TestRunHubCommitFailure`, `TestRunAllFailsOnReadonlyDatabase`. Root cause: tests `chmod 0o500/0o555` a temp dir and expect the chmod to block writes; running as root bypasses the restriction. Not a code bug. Importer functional tests (`TestImportPythonDBFromFixture`, `TestImportFullLegacyFixture`, all `CountParity*`) pass.

### §8 Python → Go mapping — used in step §5.6

Applied to:
- `~/.openclaw/workspace/AGENTS.md` — rewritten line 220 ("Top 5 verbs").
- `~/.openclaw/workspace/MEMORY.md` — line 6 `devdb hub-dashboard --view work` → `devdb hub dashboard --view work`.
- `~/.openclaw/workspace/storyblender/AGENTS.md` — 12 references rewritten across §Feedback routing, §Devdb, §Session protocol.
- `~/.openclaw/workspace/storyblender/scripts/development.py` — replaced the 128-line Python wrapper with a 35-line thin shim that execs the Go binary with `--repo` pinned to storyblender's repo root.

Skill symlinks:
- `~/.openclaw/workspace/storyblender/.claude/skills/devdb` — repointed from broken `/home/maluko/devdb/skills/devdb` to `/root/.openclaw/workspace/projects/devdb-go/skills/devdb`.
- `~/.codex/skills/devdb` — created (did not exist before), pointing at the Go skill.

### Final verification (§6) results

| Check | Result |
|-------|--------|
| `devdb --help` first line | `Queryable per-project memory for codebases` (Go) |
| `go test ./...` | 23 packages `ok`, 4 packages `FAIL` (only the 7 chmod-as-root tests, all in known-fixture code paths) |
| Importer functional tests | All pass (including `TestImportFullLegacyFixture` after patch) |
| `devdb doctor` per project | 8/8 → `doctor: ok · schema: go v2` |
| `devdb hub dashboard --view summary` | 8 projects listed, no errors |
| Callsite sweep (grep for old verbs in AGENTS.md/MEMORY.md/wrappers) | zero hits |
| Cron / systemd / scheduled tasks | none found |

### Pre-go backups (90-day retention)

Created `development.db.pre-go-2026-06-19` alongside each in-scope `.devdb/development.db` per §3.2. Importer also wrote `development.db.python-bak` at §4.3. Both classes of backup are reversible via `mv`. **Trash all backups on 2026-09-19** (90 days post-cutover, per §10 rollback plan).

Hub file backups (`/tmp/devdb-backup-2026-06-19/`):
- `~/.devdb-projects` (registry file)
- `~/.devdb/metadata.db` (hub DB at the time of cutover; superseded by rebuilt hub)

Python-only-table archives (per §4.9, JSONL in `.devdb/archive-python-only/`):
- `devdb`: `entity_links.jsonl` (5 rows), `anchors.jsonl` (5 rows)
- `storyblender`: `entity_links.jsonl` (9 rows), `anchors.jsonl` (9 rows), `loc_snapshots.jsonl` (6541 rows — the largest archive)
- `workspace`: `entity_links.jsonl` (30 rows), `anchors.jsonl` (30 rows)

---

## Lessons learned (process improvements for future migrations)

These address the *migration process itself*, not the devdb-go tool. Filed as feedback in devdb-go where the fix is in the tool; listed here where the fix is in the plan/runbook.

| # | Lesson | Where to fix | Severity |
|---|--------|--------------|----------|
| L1 | **Tar per-project backups into a single archive** instead of leaving 14 separate `.pre-go-*` + `.python-bak` files scattered across `.devdb/` dirs. Single file, single `trash` on day 91, simpler cron. | Plan template §3.2 | med |
| L2 | **Add a §3.10 "python-value audit" step** before §4: query every status field in the source DB for values outside the Go enum. Skip migration of any project that has unresolvable values, or normalize them first. This would have caught storyblender's `feedback.status='resolved'` and `plan_item_acceptance.status='wontfix'` BEFORE the importer crashed. | Plan template §3.10 | high |
| L3 | **Cron the backup cleanup**: `0 9 19 9 *` (single-shot, Sep 19) `find $WORKSPACE -name '*.python-bak' -o -name '*.pre-go-*' \| xargs trash`. | New cron entry | med |
| L4 | **`find $HOME` not just `$WORKSPACE`** during §3.1 — discovered an 8th `.devdb/development.db` at `/root/.openclaw/.devdb/` (openclaw-home, not workspace). It was empty, but a populated one could have been missed. | Plan template §3.1 | med |
| L5 | **Make `import python-db --apply` idempotent** so a second `--apply` doesn't overwrite the original `.python-bak` (which is the rollback path) with already-Go data. | devdb-go importer (filed as `feedback 23311a6c…`) | low |
| L6 | **Record the importer test pass** as a `devdb verify record` entry once the migration verifier is reused. Currently the verification ledger is empty for this migration, so future migrations can't query for prior pass. | New step §6.0 | med |
| L7 | **Split plan vs. report** for clarity. Current MIGRATION_PLAN.md has the v1 plan plus a 50-line execution summary. Future readers might confuse "what we planned" with "what happened". Consider `MIGRATION_PLAN.md` (frozen plan) + `MIGRATION_REPORT.md` (execution log) for future migrations. | File layout | low |
| L8 | **Auto-archive python-only tables during `--apply`** instead of requiring a manual JSONL step in the plan. Saves the migration-author from forgetting. | devdb-go importer (filed as `feedback f112be07…`) | med |
| L9 | **Skip chmod-as-root tests** under `euid == 0` so local dev runs show green. | devdb-go test fixtures (filed as `feedback 9cecd97e…`) | med |
| L10 | **Fixture must cover non-conforming values** so future enum-tightening regressions get caught. The `python_ledger_full_test.go` fixture has zero feedback rows; one row each of `status='resolved'` and `status='deferred'` would have caught storyblender's bug. | devdb-go test fixtures (filed as `feedback 419df7ea…`) | med |
| L11 | **`hub register --auto`** to walk the parent dir and register every `.devdb/development.db`. Manual loops scale poorly past 10 projects. | devdb-go hub CLI (filed as `feedback 76331e8c…`) | low |
| L12 | **`hub unregister <alias>` verb** for cleaning up after archived projects. Currently `register` updates in place, which is undocumented. | devdb-go hub CLI (filed as `feedback 5377d2a2…`) | low |
| L13 | **`--repo-root` alias for `--repo`** to ease muscle-memory migration from the python CLI. | devdb-go CLI (filed as `feedback d419a47e…`) | low |

devdb-go–side improvements are persisted as `feedback` rows in the devdb-go project ledger (7 entries, all `category=idea`). Process-side improvements (L1–L7) belong in the migration plan template, which should be lifted out of this file once it stabilizes.

---

## Plan body (preserved for reference)

## Plan body (preserved for reference)

## 1. Background

The Go CLI (`sebihoermann/devdb-go`) reached feature parity with the Python CLI as of 2026-06-16 (see `devdb-go/docs/go-native-parity-matrix.md`). Key facts:

- The Go binary ships a one-shot importer: `devdb import python-db --apply` (backs up the source DB to `development.db.python-bak`, then replaces it in place with the Go schema).
- Command grammar changes from `verb-noun` to `noun verb` (e.g. `feedback add --role user` instead of `feedback-user`). The Python wrapper at `/usr/local/bin/devdb` still dispatches to `scripts/devdb.py`.
- Six tables are intentionally **not** ported: `code_tightening`, `loc_snapshots`, `improvement_suggestions`, `agent_documents`, `anchors`, `entity_links`, `project_links` (rationale in `go-native-schema-importer-mapping.md`).
- Status enums tighten on import: `goal.status` `inactive → wontfix`; `feedback.status` `deferred|wontfix → closed`.
- The metadata hub (`~/.devdb/metadata.db`) is rebuilt by `hub sync` after project migration — it is **not** carried over by the importer.

## 2. Goals & non-goals

**Goals**
- Replace the Python CLI in this workspace with the Go CLI, end-to-end.
- Migrate every Python-created `development.db` without losing auditable history.
- Keep the agent playbook (`skills/devdb/SKILL.md`) usable in both source repos during the transition.
- Preserve shell-script compatibility wherever reasonable (keep `devdb` as the binary name).

**Non-goals**
- Re-implementing the six intentionally-skipped Python tables.
- Maintaining a Python compatibility shim. The Go `import python-db` is one-shot; after migration the Python CLI is no longer invoked.
- Touching the public `sebihoermann/devdb` Python repo. Cutover is local to this machine.

## 3. Pre-migration (M-day minus)

| # | Task | Tool | Output |
|---|------|------|--------|
| 3.1 | Inventory every Python-created `.devdb/` in workspace | `find ~/.openclaw -name development.db` | List of 7 paths + sizes |
| 3.2 | Snapshot each DB before any change | `cp <db> <db>.pre-go-$(date +%F)` | Per-DB backup |
| 3.3 | Capture schema kind + row counts for every DB | read-only sqlite3 probe | `reports/before.csv` |
| 3.4 | Identify which Python-only tables are populated (`loc_snapshots`, `code_tightening`, `entity_links`, …) | sqlite3 probe | Decision list: archive vs. accept loss |
| 3.5 | Enumerate every wrapper script, cron entry, AGENTS.md-style doc that names a Python verb (`feedback-user`, `add-plan`, `work-on`, `hub-dashboard`, …) | grep across `~/.openclaw`, scripts, docs, crontab | Hotspots list — needed because the hard cut rewrites them all in one pass |
| 3.6 | Confirm Go toolchain available + version | `go version` (≥ 1.22 expected) | Build OK |
| 3.7 | Build Go CLI from checkout | `cd devdb-go && go build -o /tmp/devdb ./cmd/devdb && go test ./...` | Tests green, binary at `/tmp/devdb` |
| 3.8 | Run parity tests against synthetic Python fixtures | `go test ./internal/importer/...` | Re-confirm importer works |
| 3.9 | Capture `~/.devdb-projects` and `~/.devdb/metadata.db` | `cp -a` | Backed up before hub rebuild |

## 4. Per-project migration — sequential order

The 7 ledgers migrate in this order, one at a time, each through the same 8-step gate:

1. `devdb-go` (dogfoods the migration on its own future home)
2. `devdb` (Python toolchain repo, rich history)
3. `devdb-public` (Python public-cleanup branch)
4. `MostActive`
5. `storyblender`
6. `trips`
7. `mail-server`
8. workspace root (`/root/.openclaw/workspace/.devdb/`)

Steps for each project (run inside the target repo, with the freshly built Go binary):

| Step | Command | Notes |
|------|---------|-------|
| 4.1 | `devdb import python-db .devdb/development.db` | Dry-run; prints row counts + skipped tables |
| 4.2 | Compare reported counts to `reports/before.csv` | Manual diff; abort if drift |
| 4.3 | `devdb import python-db --apply` | Atomic: writes `development.db.python-bak`, replaces in place |
| 4.4 | `devdb doctor` | Expect `schema_kind: go` |
| 4.5 | `devdb status --json` + `devdb plan item list --legacy` | Spot-check plan/feedback/acceptance preservation |
| 4.6 | `devdb plan item show <id>` on one in-progress item | Confirm acceptance rows + filescope survive |
| 4.7 | `devdb hub sync` | Re-populate `~/.devdb/metadata.db` for this project |
| 4.8 | `devdb resume` | Surfaces the right in-flight item |
| 4.9 | For each populated Python-only table from step 3.4, export JSONL from the `.python-bak` to `.devdb/archive-python-only/<table>.jsonl` before the backup is removed | Preserve what the importer drops |

**Acceptance gate for a project to be marked migrated:**
- `devdb doctor` clean (`schema_kind: go`, no missing tables).
- `devdb status --json` row counts for `feedback`, `plan_items`, `goals`, `plans`, `repo_files`, `verification_runs`, `archive_entries` match `reports/before.csv` within `CountParity` tolerance.
- `devdb hub project <alias>` lists the project post-`hub sync`.
- One in-progress plan item, if any, round-trips through `plan item show` with intact acceptance criteria.

## 5. Workspace cutover (single-day switch, hard cut)

Triggered once projects 1-3 (devdb-go, devdb, devdb-public) all clear the acceptance gate. Steps run in one continuous session:

| # | Task |
|---|------|
| 5.1 | `go install ./cmd/devdb` from the devdb-go checkout → binary at `$(go env GOPATH)/bin/devdb` |
| 5.2 | Replace `/usr/local/bin/devdb` so it execs the Go binary directly (no bash wrapper, no Python) |
| 5.3 | Verify `which devdb` resolves to the Go binary and `devdb --help` first line confirms Go |
| 5.4 | For each remaining project (4-7) plus the workspace root, run the §4 gate |
| 5.5 | `devdb hub sync` from every project root — one batch to repopulate the rebuilt hub for all 7 |
| 5.6 | Sweep every wrapper script and doc from step 3.5 for old verb names. Rewrite each callsite to the Go grammar (mapping table in §8). |
| 5.7 | Repoint the symlinked agent skill: `ln -sfn /root/.openclaw/workspace/projects/devdb-go/skills/devdb ~/.codex/skills/devdb` (or whichever skills dir the agent actually reads) |
| 5.8 | Run the session-start ritual (`devdb resume`, `status`, `quality`, `report`) in all 7 projects back-to-back as a smoke test |

## 6. Verification

| Check | Command | Pass condition |
|-------|---------|---------------|
| Binary identity | `devdb --help \| head -20` | First line mentions Go |
| Parity unit tests | `go test ./...` | All green |
| Importer fixture tests | `go test ./internal/importer/...` | All green |
| Per-project doctor | `devdb doctor` in each of the 7 | `schema_kind: go`, no missing tables |
| Hub consistency | `devdb hub dashboard --view summary` | Each of 7 listed with fresh sync timestamp |
| Audit spot-check | Open one in-progress plan item per project via `devdb plan item show` | Acceptance rows + filescope match pre-migration snapshot |
| Callsite sweep | `grep -REn "feedback-user\|add-plan\|work-on\|hub-dashboard\|add-arch-note" ~/.openclaw` | Zero hits outside devdb-go's own docs/ |

## 7. Decommission Python (same day, no stability window)

Because the cutover is hard, decommission runs immediately after §5.8 passes — there is no parallel Python path to fall back to.

| # | Task |
|---|------|
| 7.1 | Archive (do not delete) the Python checkout: `mv /root/.openclaw/workspace/devdb /root/.openclaw/workspace/.archive/devdb-python-$(date +%F)` |
| 7.2 | Drop every cron entry that called Python devdb verbs (audited from step 3.5) |
| 7.3 | Update workspace `AGENTS.md` to reference `devdb-go/skills/devdb/SKILL.md` (already swept in step 5.6, this is a final read-through) |

## 8. Python → Go verb mapping (for step 5.6 sweep)

Cheat sheet for the callsite rewrite. Full table: `devdb-go/docs/go-native-parity-matrix.md`.

| Python verb | Go command |
|-------------|------------|
| `feedback-user` / `feedback-model` / `feedback-codebase` | `feedback add --role user\|model\|codebase` |
| `close-feedback` | `feedback close` |
| `add-goal` | `goal add` |
| `set-goal-status` | `goal set` |
| `add-feature` | `feature add` |
| `add-plan` (legacy flat) | `plan item add --legacy --phase … --step …` |
| `add-plan-item` | `plan item add` |
| `create-plan` | `plan create` |
| `add-milestone` | `plan milestone add` |
| `add-acceptance` | `plan acceptance add` |
| `meet-acceptance` | `plan acceptance meet` |
| `add-plan-file` | `plan file add` |
| `show-plan` | `plan show` |
| `show-plan-item` | `plan item show` |
| `list-plan-items` | `plan item list` |
| `work-on` | `plan item start` |
| `pause-on` | `plan item pause --note "…"` |
| `set-status` | `plan item status` |
| `close-plan-item` | `plan item close --evidence "…"` |
| `add-arch-note` | `arch add` |
| `update-arch-note` | `arch update` |
| `verify-arch-note` | `arch verify ID` |
| `list-arch-notes` | `arch list` |
| `arch-render` | `arch render` |
| `scan` | `inventory scan` |
| `snapshot-loc` | `inventory loc` (live only — no `loc_snapshots` in Go) |
| `context` | `inventory context` |
| `diff-since` | `inventory diff` |
| `suggest-cuts` | `inventory suggest-cuts` |
| `review-start` | `review start` |
| `review-add-finding` | `review finding` |
| `review-finish` | `review finish` |
| `review-list` | `review list` |
| `review-resolve` | `review resolve` |
| `review-report` | `review report` |
| `record-verification-run` | `verify record` |
| `query-verification` | `verify query` |
| `show-verification-run` | `verify show` |
| `dismiss-verification` | `verify dismiss` |
| `gc` | `archive gc` |
| `archive` | `archive run` |
| `restore-list` | `archive list` |
| `restore` | `archive restore` |
| `list-missed-calls` | `analytics missed` |
| `missed-calls-summary` | `analytics summary` |
| `register` | `hub register` |
| `list-projects` | `hub list` |
| `hub-sync` | `hub sync` |
| `hub-dashboard` | `hub dashboard` |
| `hub-project` | `hub project` |
| `doctor-sync` | `hub doctor` |
| `across` | `hub across` |
| `add-task` | `task add` |
| `list-tasks` | `task list` |
| `complete-task` | `task done` |
| `add-reminder` | `reminder add` |
| `list-reminders` | `reminder list` |
| `snooze-reminder` | `reminder snooze` |

Not ported (intentional): `link-source`, `log-session`, `add-tightening`, `close-tightening`, `add-suggestion` (use `feedback add --role model --category idea`), `list-verification-runs` (use `verify query`).

## 9. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Importer silently drops a Python-only table we actually depend on | Med | Med | Step 3.4 inventories populated tables; step 4.9 archives them before `.python-bak` is removed |
| Status enum tightening changes user-visible state | Med | Low | Documented in `go-native-schema-importer-mapping.md`. Diff `goals.status` and `feedback.status` before/after for one project |
| Hub rebuild loses attention items or sync history | Med | Low | Backup `metadata.db` at step 3.9; `hub sync` repopulates from per-project DBs |
| A wrapper script or AGENTS.md still hides the old verb grammar after the cutover | High | Med | Step 3.5 enumerates them; step 5.6 sweeps them; step 6 grep gate confirms zero hits |
| A plan item in flight depends on a verb that was `not ported` (e.g. `link-source`, `log-session`) | Med | Med | Parity matrix already lists these as `not ported`. Rescope those plan items before migration |
| Go toolchain missing or version too old | Low | Blocker | Step 3.6 confirms; fall back to `go install github.com/sebihoermann/devdb-go/cmd/devdb@latest` |
| Cron entry runs old Python wrapper after cutover | Med | Med | Step 3.5 + step 7.2 audit |
| `.python-bak` retention mis-managed under hard cut | Low | High | 90-day retention, single scheduled `trash` at day 91 (see §10) |

## 10. Rollback

For any single project: `mv .devdb/development.db.python-bak .devdb/development.db` and reinstall the previous wrapper script. Workspace-wide: repoint `/usr/local/bin/devdb` to the archived Python checkout and re-symlink the old `skills/devdb/SKILL.md`. `.python-bak` retention: **90 days post-cutover**, after which all 7 backups are removed in one batch via `trash .devdb/development.db.python-bak`.

## 11. Decision log

| # | Decision | Choice |
|---|----------|--------|
| 1 | Plan file path | `/root/.openclaw/workspace/projects/devdb-go/MIGRATION_PLAN.md` |
| 2 | Scope | All 7 ledgers |
| 3 | Transition style | Hard cut, no Python-compat shim |
| 4 | Project order | devdb-go → devdb → devdb-public → MostActive → storyblender → trips → mail-server → workspace root |
| 5 | `.python-bak` retention | 90 days |