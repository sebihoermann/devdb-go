---
name: devdb
description: Per-project memory and quality ledger. Use when working in any repository that has a .devdb/ directory — log feedback, architecture knowledge, and code-review findings, and read what previous agents already logged before doing new work.
---

# Devdb

One SQLite DB per project at `.devdb/development.db`. Everyone writes to it. Everyone reads from it. It replaces markdown sprawl with structured project state.

> **Audience:** This is the AI agent playbook. For human contributors, see [CONTRIBUTING.md](../../CONTRIBUTING.md).

> **Runtime:** Install with `go install github.com/sebihoermann/devdb-go/cmd/devdb@latest` or `go install ./cmd/devdb` from a checkout. Legacy Python sources: [sebihoermann/devdb](https://github.com/sebihoermann/devdb) `archive/python/`.

## Command shape

```text
devdb [--repo PATH | --repo-root PATH] [--db PATH] [--json] [--all] [--verbose] <noun> <verb> [flags]
```

Top-level reads: `init`, `status`, `quality`, `report`, `resume`, `doctor`.

Legacy flat verbs (`work-on`, `feedback-codebase`, etc.) are **not** registered. Unknown commands log missed-call telemetry and print a suggestion for the new grammar.

## Target repo rule

Devdb code and devdb data are separate. The `devdb` command may come from the devdb checkout, but the active database must belong to the repository you are working in.

- Run commands from the target repo root, or pass `--repo <target-repo>`.
- Do not symlink a project's `.devdb/` to the devdb tool repo's own `.devdb/`; that writes project feedback into the devdb tool repo's own ledger.
- Use the devdb tool repo's `.devdb/` only when you are actually working on the devdb repo itself.

### Compatibility wrappers

Some repos ship `scripts/development.py` that pins `--repo` and may rewrite argv before calling devdb. Prefer the wrapper when present so the ledger stays in the target project. New wrappers should target the Go CLI grammar (`plan item start`, `feedback add --role …`, etc.) — do not rewrite to removed Python verb names.

## Operating modes

There are two modes. Identify the mode before acting.

**Brainstorm mode.** The user is exploring ideas. Casual chatting, half-baked thoughts, "what if" questions. Your job is scribe plus sparring partner. Log liberally with `feedback add --role user|model|codebase` and `goal add`. Do not execute. Do not draft a multi-step implementation plan. The conversation is the work.

**Execution mode.** The user says "go", "start M1", "ship it", or similar. Now act. Bracket the work with `plan item start` and `plan item pause`. Log async observations as you find them. Run the session-start ritual first.

If you are not sure which mode you are in, use brainstorm mode.

## Session-start ritual

Run these commands in order before any code work.

**Cross-project orientation** (when you touch more than one repo this session, or you need to pick which repo to work in):

```bash
devdb hub sync

devdb hub dashboard --view delivery
devdb hub dashboard --view quality
devdb hub dashboard --view work
devdb hub dashboard --attention-only
devdb hub project <alias>
devdb hub doctor --json
```

Register a repo once with `devdb hub register <path> --alias <name>` — that updates both `~/.devdb-projects` and the hub. `devdb hub list` reads from the hub when `metadata.db` exists.

**Per-project ritual** (always, from the target repo root or with `--repo`):

```bash
devdb resume
devdb status
devdb quality
devdb report
devdb arch list --stale
devdb review list --status open --severity high
```

The hub is a cached dashboard, not the source of truth. Goals, plans, feedback, and findings still live in `<repo>/.devdb/development.db`. After a successful write in a registered project, devdb opportunistically refreshes that project's hub row; use `hub sync` when you need every registered project refreshed at once (new machine, stale dashboard, or projects you have not opened recently). Disable push with `--no-metadata-push` or `DEVDB_DISABLE_METADATA_PUSH=1`.

If `devdb resume` shows an in-progress plan item, ask the user whether to continue it or start something new. Do not silently switch contexts. When picking it up, run `devdb plan item show <id>` to read the acceptance criteria, file scope, and last status note before editing anything.

If `arch list --stale` returns anything for files you intend to touch, verify the note if it is still accurate or update it if it drifted before editing those files.

If `review list --status open` shows high-severity findings on files you are about to touch, decide explicitly: fix now or leave them and proceed. Do not edit a file with high-severity findings without acknowledging them.

Use `devdb status` for delivery state, `devdb status --json` for compact machine-readable delivery state, `devdb quality` for trust signals, and `devdb report` for the full memory/diagnostic read.

If `.devdb/` does not exist yet, run `devdb init` first — the ritual depends on a schema. If the DB is a legacy Python ledger, run `devdb import python-db --apply` once before dogfooding on Go.

## Picking work when nothing is in flight

If `devdb resume` prints `no in-flight work`, do not ask "what should I work on?" without context. In this order:

1. **High findings first.** If `devdb review list --status open --severity high` returned rows, intersect their `file_path` with `plan file` scope (run `devdb plan item list` and `devdb plan item show <id>` for candidates). The plan whose scope touches the most open high findings wins.
2. **Lowest M-step with `status='planned'`.** Phase-and-step (`M8.1` before `M8.2`) reflects build order; later steps often depend on earlier ones.
3. **Ask the user.** Only after the above are exhausted or ambiguous.

`devdb plan item start <plan_item_id>` then prints the plan item id on stdout; file allowlist and unmet acceptance (with `[id-prefix]` lines) go to stderr — your authoritative scope.

## Dogfood rule

Every time you finish a unit of work in a repo that has `.devdb/`, log it. Do not wait to be asked:

- New understanding of how a piece of code works → `devdb arch add`.
- Friction, bug, surprise, or working pattern worth keeping → `devdb feedback add --role codebase` (or `--role user` / `--role model`).
- Passing verification you just ran → `devdb verify record "…" --scope … --status passed --exit-code 0 --finished` with the current input hashes. Future agents cannot reuse tests that nobody records.
- Resolved review finding → `devdb review resolve <id> --commit <sha>` (or `--status wontfix`).
- Acceptance criterion met → `devdb plan acceptance meet <acceptance_id> --evidence <commit-or-note>`. Tick each criterion individually as it lands; do not batch at the end.
- All criteria met → `devdb plan item close <plan_item_id> --evidence <commit-or-note>` (preferred) or meet each criterion then `devdb plan item status <id> done --note "..."`. **Never call `plan item status … done` while any criterion is still `[open]`** — see "Closure ritual" below.

The skill is wasted if the ledger does not grow with the work.

## Hygiene rule

A ledger nobody prunes becomes silt. Run `devdb archive gc --dry-run` at the start of any session whose `devdb report` shows a populated stale section. If the dry-run summary matches your expectations, run `devdb archive gc --yes` to archive feedback older than 30 days that's still open, and to resolve review findings on files no longer present in `repo_files`. Adjust the threshold with `--older-than DAYS` when needed.

Archiving is reversible (`UPDATE feedback SET status='open' WHERE id=?`), so bias toward archiving aggressively.

### Heavy hygiene: `archive run` and `archive restore`

`archive gc` handles stale-open feedback and missing-file findings. For *closed* work that has accumulated — resolved review findings, finished review runs, completed plan items, historical features — use `archive run`:

```bash
devdb archive run --dry-run           # preview what would move out of primary tables
devdb archive run --yes               # do it
devdb archive run --yes --vacuum      # do it, then VACUUM to reclaim freed pages
```

Archived rows go to `archive_entries`. `plan_items` archive cascades to acceptance, scoped files, and status_log children.

Inspect what's in the archive without restoring anything:

```bash
devdb archive list                              # all entries (up to 50)
devdb archive list --table feedback             # scoped to one source table
devdb archive list --since 2026-05-01 --json   # machine-readable, with date filter
devdb archive list --limit 200                 # raise the cap
```

Recover with `archive restore`:

```bash
devdb archive restore --id <archive_entry_id> --yes              # one row
devdb archive restore --source-table feedback --source-id <orig_id> --yes
devdb archive restore --table features --since 2026-05-01 --yes  # bulk by table + window
devdb archive restore --id <plan_item_archive_id> --cascade --yes  # plan_item + its children
```

Conflict policy is `INSERT OR IGNORE` — if the source row already exists, restore counts it as skipped rather than overwriting.

### Auto-archive on a schedule

`archive run --yes --json` is non-interactive and machine-readable. A weekly cron entry keeps the ledger from accumulating silt without manual intervention:

```cron
# every Sunday 03:00 — prune devdb closed/resolved rows
0 3 * * 0  cd /path/to/repo && devdb archive run --yes --json >> .devdb/archive.log 2>&1
```

For multi-project setups, run `devdb hub sync` so `hub dashboard` reflects current state, then loop registered projects (`devdb hub list`) with `devdb --repo <path> archive run --yes`. Skip repos with active in-flight plan items if an agent may still be editing them.

## Workflow bracket

Bracket real work with these commands:

```bash
devdb plan item start <plan_item_id>
... do work, log async observations ...
devdb plan item pause <plan_item_id> --note "next step is X, blocker is Y"
```

`--note` is required on `plan item pause`. If you cannot say what comes next in one sentence, you do not know what comes next.

Pause often. A coffee break is a pause. End of day is a pause. If the session ends without a pause, the plan item stays `in_progress` and the next `devdb resume` should surface it.

### Plan detail (acceptance + file scope)

A plan item can carry structured detail. Use it when work is non-trivial.

```bash
devdb plan create "Title" --slug my-plan
devdb plan milestone add --plan my-plan --title "M1"          # title text only — see Convention 1 below
devdb plan item add --plan my-plan --milestone M1 --title "..."
devdb plan acceptance add --plan-item <plan_item_id> "text"   # criterion text is positional — see Convention 2 below
devdb plan file add --plan-item <plan_item_id> --path PATH --role create|modify|forbidden|touched
devdb plan show <plan-slug>                        # plan header + milestone list
devdb plan item show <plan_item_id>               # item detail: body, acceptance, files, status log
devdb plan acceptance meet <acceptance_id_or_prefix> --evidence <commit_or_note>
```

`plan item show` and `plan tree` print acceptance rows with `[id-prefix]` for copy-paste into `plan acceptance meet`.

### Conventions (read these before scaffolding a plan)

1. **Don't prefix milestone titles with `M1` / `M2`.** `devdb plan show` already
   renders the milestone number ahead of the title. A title of `"M1 Med-severity
   fixes"` displays as `M1 M1 Med-severity fixes [planned]`. Use only the
   descriptive part: `"Med-severity fixes"`.

2. **Don't start plan acceptance criterion text with `--`.** The CLI parser
   interprets any leading flag-like token as a flag and rejects the criterion
   with `unknown flag`. Start criteria with natural-language phrasing, e.g.
   `"Passing the plan uuid via --plan still works"` rather than
   `"--plan <uuid> still works"`.

3. **When a session-start ritual step itself surfaces a bug, fold the fix +
   regression test into the plan you're scaffolding.** A bug that blocks
   `devdb inventory scan`, `devdb status`, or `devdb plan item show` is
   unblockable for every future session. Treat it as priority one — log it
   with `feedback add --role codebase`, then add an acceptance criterion that
   requires the regression test alongside the code fix. Example:
   `TestScanWithNullLanguageAndContentHash` was born this way.

```bash
devdb plan scaffold "Title" --mode design    # define-scope acceptance template
devdb plan scaffold "Title" --mode implement # default; ship-code template
devdb plan promote --plan <slug>             # design → implement titles + acceptance text
devdb plan reconcile --plan <slug>           # detect plan/header vs item status drift; --apply to repair
devdb plan acceptance backfill --plan-item <id>   # add missing criteria from templates or stdin
```

## Closure ritual

Before marking a plan item `done`, run these in order:

```bash
devdb plan item show <plan_item_id>                               # 1. read the unmet criteria
devdb plan acceptance meet <criterion_id> --evidence <commit-or-note>  # 2. tick each met criterion
devdb plan item close <plan_item_id> --evidence <commit-or-note> [--note "free-form annotation"]  # 3. preferred atomic close; auto-completes the parent milestone when its last item closes
# or: devdb plan item status <plan_item_id> done --note "..."     # 3alt. only after all criteria are met
```

The closure ritual is non-negotiable. If you skip step 2 and jump straight to `plan item status … done`, `devdb plan show` will forever report `status: done` with every criterion still `[open]` — the audit trail dies.

**If a criterion turns out to be wrong, infeasible, or already covered elsewhere:** meet it with `--evidence "covered by <other_commit_or_plan>"` or `--evidence "wontfix: <reason>"`, or explain the gap explicitly in the `plan item status done --note`.

**If you forget the ritual:** the next agent's spot-check (or `devdb status --json` showing a done plan with open criteria) will catch it. Backfill by running `plan acceptance meet` with `--evidence <commit-sha>` against the original commit.

## Behavioral rules

**Async observation, no interruption.** When you spot a smell or surprise while working, log it with `feedback add --role codebase` and continue. Do not stop the current task to ask permission.

**Friction-check at real breakpoints.** At natural breakpoints, ask one short question: `any friction in the last X to log?` Write the reply directly into `feedback add --role user`.

**No walls of text.** Keep responses short. When uncertain, ask one direct question instead of producing fluent uncertainty.

**Re-read user premises before scoping.** If the user sounds confused, restate the premise in plain words and ask one targeted question.

## CLI conventions

Hold these in mind so scripted use does not break:

- **Stdout contract.** Write verbs print a **bare entity id on stdout** (or `{"id": "..."}` with `--json`). Human acks and scope detail go to **stderr**. `plan item start` stdout is only the plan_item id; allowlist and unmet acceptance are on stderr.
- **Global `--all`.** Expands default row caps on list-style reads (`feedback list`, `review list`, `reminder list`, `report`, etc.).
- **Metadata hub.** `--metadata-db` and `DEVDB_METADATA_DB` override `~/.devdb/metadata.db`. `--no-metadata-push` and `DEVDB_DISABLE_METADATA_PUSH=1` skip opportunistic hub refresh after writes.
- **Severity aliases.** `med` and `medium` are interchangeable; `info|low|med|medium|high|critical` are accepted and normalized.
- **Severity scale.** `info` = note for completeness. `low` = clean up if nearby. `med` = real defect. `high` = correctness, security, or DX block. `critical` = production / data-loss class.
- **Category convention (`feedback add --category`).** Reach for: `correctness`, `convention`, `dx`, `docs`, `test-coverage`, `sprawl`, `dogfood`, `audit`, `skill`. Single word, kebab-case.
- **Plan-item statuses.** `planned` → `in_progress` (set by `plan item start`) → `done` (close via `plan item close` or status after acceptance). Use `wontfix` for explicit cancel.

## Playbook 1: Log feedback

```bash
devdb feedback add "..." --role model --severity ... --category ...
devdb feedback add "..." --role user --severity ... --category ...
devdb feedback add "..." --role codebase --severity ... --category ...
devdb feedback close <id> --proposed-fix "..."
devdb feedback annotate <id> --note "..."
devdb feedback import markdown path/to/archive.md
devdb feedback import commits --branch feature/foo
```

Use `--category idea` for improvement suggestions — there is no separate suggestions table in Go.

## Playbook 2: Architecture notes

```bash
devdb arch add TOPIC --body "..." --source path1 --source path2 ...
devdb arch verify <ID>
devdb arch verify all                    # bulk staleness check before large edits
devdb arch update <ID> --body "..." --source ...
devdb arch list --touching path --stale
devdb arch render --output .devdb/architecture.md
```

Staleness is derived from source file content hashes. `arch list --stale` surfaces drifted notes.

Rules: topic is kebab-case, 3–40 chars; body is 2–5 sentences; name files, do not paste code; source paths must exist in `repo_files` (run `inventory scan` first).

## Playbook 3: Code review

```bash
devdb review start --paths src tests
devdb review finding --run <RUN_ID> --file ... --principle ... --severity ... --confidence ... --effort ... --title "..." --recommendation "..."
devdb review finish --run <RUN_ID> --summary "..."
devdb review list --status open
devdb review resolve <FINDING_ID> --commit SHA
devdb review resolve <FINDING_ID> --status wontfix
devdb review report --run <RUN_ID> --output .devdb/reviews/RUN_ID.md
devdb review principles --tier default
```

Tier rules: `default` (cap 3 findings per file), `extended` (cap 5), `grass-cutter` (heuristic discovery via `inventory suggest-cuts`).

## Playbook 4: Grass-cutter (cheap heuristic discovery)

```bash
devdb inventory scan
devdb inventory suggest-cuts --dry-run
devdb inventory suggest-cuts
devdb review list --principle dead
devdb review resolve <id> --commit <sha>
```

Scope large trees with `--paths`. `test_*` under `tests/` are exempt from the dead heuristic.

## Playbook 5: Verification ledger and rerun policies

**Required habit:**

1. Run `devdb inventory scan` after code edits and before verification decisions.
2. Build the input evidence set from the command's real dependencies.
3. Query first with the exact command string, scope, and input triples.
4. If the result is `fresh_pass`, reuse the prior run and say which run you reused.
5. If stale, unsupported, absent, failed, or ambiguous, rerun the verifier.
6. When the rerun passes, immediately record it. Do not leave successful verification unrecorded.

```bash
devdb inventory scan
devdb verify query "go test ./..." --scope . --json
devdb verify record "go test ./..." --scope . --git-sha $(git rev-parse HEAD) --status passed --exit-code 0 --finished
devdb verify show <run_id>
devdb verify dismiss <run_id> --reason "..."
```

After `inventory scan`, `verify query` and `verify record` auto-collect inputs from `repo_files` when `--inputs` is omitted.

**When in doubt:** Rerun. Log the decision as feedback if the pattern is worth remembering.

## Playbook 6: Reminders

```bash
devdb reminder add "re-verify auth module" --due 2026-06-15 --file src/auth.py
devdb reminder add "ship M4" --due tomorrow --plan-item <plan_item_id>
devdb reminder list --overdue
devdb reminder list --status open --json
devdb reminder show <id>
devdb reminder snooze <id> --until 2026-06-20
devdb reminder dismiss <id>
```

## Playbook 7: Tasks and approval

```bash
devdb task add "verify CLI works" --priority high
devdb task list --status open
devdb approval request --task <task_id>
devdb approval approve --task <task_id>
devdb approval reject --task <task_id> --reason "superseded"
```

## Playbook 8: Missed CLI calls analytics

```bash
devdb analytics missed [--since ISO-TS] [--limit N] [--json]
devdb analytics summary [--since ISO-TS] [--json]
```

Missed calls land in `missed_cli_calls` with `suggested_command` for the new grammar.

## Playbook 9: Metadata hub (cross-project dashboard)

| Location | Role |
|----------|------|
| `<repo>/.devdb/development.db` | Source of truth for that project's memory |
| `~/.devdb/metadata.db` | User-local hub: cached metrics, attention items, sync history |

```bash
devdb hub register /path/to/repo --alias myapp
devdb hub register --auto --scope /path/to/parent    # walk + register every .devdb/development.db
devdb hub unregister <alias_or_path>                 # remove from hub (registry + metadata)
devdb hub list
devdb hub sync [--strict] [--json]
devdb hub sync --watch --interval 60
devdb hub dashboard [--view summary|work|delivery|quality] [--attention-only] [--json]
devdb hub project <alias_or_path> [--json]
devdb hub doctor [--json]
devdb hub across similar-feedback --keyword "import error"
devdb hub across open-debt
devdb hub audit [--severity <level>] [--kind <list>] [--project <alias>] [--cached] 
```

`hub audit` is the one-command cross-project session-start read: live
federation read by default (parity with `hub across`), `--cached` for
hub-snapshot parity. Eight sections: high feedback, high review
findings, stale arch notes, overdue reminders, in-progress plan items,
blocked, planned per project, stale verification. Replaces 30+ manual
commands.

Federation queries (`hub across`) attach each project's `development.db` directly — they do not read copied hub rows.

## Other useful reads

- `devdb report` — whole-project overview (use `--all` for more feedback rows).
- `devdb status --json` — one-line delivery snapshot.
- `devdb hub audit` — one-command cross-project open issues + plans snapshot. The session-start cross-project read.
- `devdb inventory context --files PATH ...` — arch notes, findings, reminders for files you will touch; `--strict` exits non-zero on stale notes or high findings.
- `devdb inventory diff --since <git-ref>` — changes since ref and related notes/findings.
- `devdb list <table>` / `devdb show <table> <id>` — raw enumeration when you need it.

## Command cheatsheet

**Initialization:** `init`, `import python-db`

**Planning:** `plan create|list|show|tree|status|scaffold|promote|reconcile` · `plan milestone add|list|status` · `plan item add|list|show|start|pause|close|status` · `plan acceptance add|meet|backfill` · `plan file add`

**Memory:** `feedback add|list|show|close|annotate|import` · `goal add|list|set` · `feature add|list`

**Architecture:** `arch add|list|show|update|verify|render`

**Inventory:** `inventory scan|loc|context|diff|suggest-cuts`

**Review:** `review start|finding|list|resolve|finish|report|import|principles`

**Verification:** `verify record|query|show|dismiss`

**Tasks / approval / reminders:** `task add|list|show|done|status` · `approval request|approve|reject|withdraw|list|log` · `reminder add|list|show|dismiss|snooze|unsnooze`

**Hub:** `hub register|unregister|list|sync|dashboard|project|doctor|across`

**Hygiene / analytics:** `archive run|list|restore|gc` · `analytics missed|summary`

**Reads:** `status`, `quality`, `report`, `resume`, `doctor`, `help`, `list`, `show`

Global flags: `--repo` (alias: `--repo-root`), `--db`, `--json`, `--all`, `--verbose`, `--metadata-db`, `--no-metadata-push`.

## Why these habits matter

A project's devdb is its memory. The session you are in is one of many. Write so the next agent can pick up cold.

When you are working *inside* this very repo (the `devdb` project itself), the dogfood rule is non-negotiable: the project's value is proved by its own ledger.
