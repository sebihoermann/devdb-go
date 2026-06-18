# Devdb extension: engineering scope

The plan to extend devdb so agents can log architecture notes and code-review findings in the same DB they already log feedback to. Companion to `README.md` (what devdb is and why) and `features.md` (original brainstorm).

This document is the engineering scope. The *product* is the SKILL.md change in Milestone 5 — every milestone before it is plumbing that makes that change possible. If we ship M1–M4 and skip M5, users see nothing.

## Brainstorm session insights (2026-05-22)

This SPEC was developed in a single brainstorm session that itself became proof of why devdb matters: every decision, frustration, course-correction, and insight from that conversation lives durably in this repo's own `.devdb/development.db` (~60 rows at session end). A future agent can read the trail and rebuild the entire thinking without re-having the conversation. That is the value proposition.

The session produced eight irreducible principles, recorded here so they cannot drift back into ambiguity. Each has corresponding rows in the devdb itself (goals table for the do/dont rules; feedback-user rows for the longer narratives).

### 1. State-first beats document-first
Model the project as state in a structured store (SQLite). Let the LLM render from it. The brain IS the connected state — never the LLM holding it in working memory. AI writes to state and queries it; AI never tries to be it. Storyblender's pivot from markdown-driven to state-engine-driven is the empirical evidence: one month of pain proved the principle. devdb is the same shape applied to project state.

### 2. No hybrids
If a design combines devdb + markdown + Obsidian + custom indexer + ad-hoc convention, it is wrong. Pick one storage primitive (SQLite) and make it carry everything that should be queryable state. Markdown stays for human-facing fronts (`README.md`, `SKILL.md`) only. Hybrids are the failure pattern to avoid.

### 3. Frictionless or it does not stick
Every design decision is measured against: does this remove friction for the human and the AI, or does it add a layer? Layers fail. The async observation rule (log without asking), the friction-check pattern (one question at real breakpoints), and the workflow bracket (work-on / pause-on / resume) are direct expressions of this principle.

### 4. Walls of text drown the user
When uncertain, AI generates fluent prose ("exam jabber" — confidently producing comprehensive-sounding words to mask not understanding the question). Counter-rule: terse responses, concrete claims, one ask at the end. When uncertain, ask one direct question instead of generating fluent uncertainty. If a response goes past ~40 lines or 5 distinct ideas, the user loses the thread.

### 5. The model is a scribe in brainstorm mode
The most productive sessions are casual chatting where ideas are born. Agent's role: scribe + sparring partner, not task executor. Log liberally as feedback/improvement_suggestions/goals. Volume over selectivity at capture time — prune later. The conversation IS the work; the logging IS the value.

### 6. Async observation, no interruption
When working on code, if you spot a smell or pattern: log it (feedback-codebase or add-tightening) and continue the current task. Quick note, move on, no permission needed. Interruption to ask "should I fix this?" breaks flow for no benefit. User reviews logged items at natural breakpoints.

### 7. Re-read user premises before scoping
When the user reacts with confusion, do NOT double down with explanation. Restate the user's premise in plain words and ask one targeted question. Most WTF moments are context gaps, not personality clashes. The user cannot reliably remember which project a conversation happened in — that is itself the strongest case for the cross-project layer (M7).

### 8. Marketing influence is real; first-principles is the filter
When an idea sounds great and elegant (like Obsidian as the cross-project layer), ask whether it would have occurred from first principles. If no, suspect. Storyblender's state engine survived this filter (it emerged from pain). Obsidian did not (it would not have occurred without external promotion). SQLite ATTACH federation survives the filter; Obsidian does not.

## What we are adding

Two more things you can log to `.devdb/development.db`:

1. **Architecture notes** — short markdown chunks an agent writes after figuring out how a piece of the codebase works. Topic, body, source files, confidence. Future agents read these instead of re-deriving the world from a 10,000-line file.
2. **Code-review findings** — ranked, evidence-backed entries against named principles (KISS, DRY, YAGNI, etc.). Each has severity, confidence, effort, file:line, and a status that closes when a commit fixes it. The point is a ledger of cleanup that survives sessions, not an AI score.

Both write into the same DB devdb already manages. Both surface in `devdb report` for the next agent or human.

## What we are NOT adding (anti-scope)

These come up naturally and we are explicitly saying no until v2.

- Knowledge-graph triples or component/edge tables — architecture notes are markdown chunks, not a graph.
- `review_scores` table — priority is computed in queries, not stored as theater.
- Automatic LLM extraction or auto-summary inside the CLI — the CLI is dumb. The agent writes content. Python doesn't parse natural language.
- A continuous watch mode, daemon, or filesystem hook.
- Web UI or dashboard.
- Embeddings or vector search.
- Multi-repo or cross-project queries.
- Automatic close-on-commit detection — the agent calls `review-resolve` explicitly when work lands.
- "Agent-readiness" or "Boy Scout Rule" as review dimensions — the first is meta-cute, the second is a process habit, not a static-code lens.

## Locked decisions (no more questions)

- **Migrations are additive-only.** Existing users have real data in `feedback`, `goals`, etc. We never rewrite their tables.
- **Output format for `devdb context` and renderers: markdown.** JSON via `--json`.
- **Scan ignores:** `.git`, `.devdb`, `__pycache__`, `.venv`, `node_modules`, `dist`, `build`. Plus respect `.gitignore`.
- **Binary files:** detect by null-byte heuristic. Store with `kind='binary'` and no content_hash.
- **Architecture note body length:** no hard cap, it's markdown.
- **`scan` discovery:** filesystem walk by default. `--git-aware` opt-in uses `git ls-files`.
- **`review-resolve` commit SHA:** required for `status=resolved`. Optional for `wontfix`, `accepted`, `duplicate`.
- **Markdown render output location:** `.devdb/architecture.md`. Generated, not committed.
- **Topic naming for arch notes:** kebab-case, `^[a-z][a-z0-9-]{2,40}$`. Banned topics: `misc`, `general`, `notes`, `stuff`, `improvements`. The CLI rejects them.

## Amendments from dogfood (2026-05-22)

These were not visible from reading the source — only from batching real inserts through the CLI. Logged as `feedback-codebase` rows in this repo's own devdb. They drove the insertion of **M1.5** below.

1. **Severity enum drift.** `feedback` uses `low|medium|high|critical`. `code_tightening` uses `info|low|med|high`. `add-suggestion` priority is `low|med|high`. Agents will fat-finger this. Fix: unified enum with backward-compatible aliases at the parser layer (M1.5).
2. **Argument-shape inconsistency.** `add-goal kind title --body` mixes two positionals before flags. `add-plan title --phase --step --body` uses one positional then all flags. Predictability beats terseness when agents script these. Convention going forward: title is the only positional; everything else is a flag (M1.5 + applied to all new verbs from M2 onward).
3. **No machine-readable output.** Every write verb prints just the new row id. Fragile for tooling. Convention going forward: every write verb accepts `--json` and emits `{"id": "..."}` (M1.5 + applied to all new verbs from M2 onward).

Meta-rule going forward: **dogfood the existing CLI before designing extensions to it.** Code-reading misses what only emerges from use in anger.

## Schema additions

All tables land via the new migration runner. v1 captures everything currently shipped (currently created via the broken `try/except` pattern). v2+ adds the new tables.

### Migration infrastructure

```sql
CREATE TABLE schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL,
  description TEXT NOT NULL
);
```

### File inventory (v3)

```sql
CREATE TABLE repo_files (
  path TEXT PRIMARY KEY,           -- relative to repo root
  language TEXT,                   -- by extension
  kind TEXT NOT NULL,              -- code | test | doc | config | agent_doc | generated | binary | other
  lines INTEGER,
  content_hash TEXT,               -- sha256, NULL for binary
  size_bytes INTEGER,
  last_seen_at TEXT NOT NULL,
  last_scan_run_id TEXT
);
CREATE INDEX idx_repo_files_kind ON repo_files(kind);
CREATE INDEX idx_repo_files_last_seen ON repo_files(last_seen_at);

CREATE TABLE scan_runs (
  id TEXT PRIMARY KEY,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  git_sha TEXT,
  files_seen INTEGER,
  files_added INTEGER,
  files_changed INTEGER,
  files_removed INTEGER,
  model_id TEXT NOT NULL
);
```

`content_hash` is the single source of truth for "did the source drift" everywhere downstream.

### Architecture notes (v4)

```sql
CREATE TABLE architecture_notes (
  id TEXT PRIMARY KEY,
  topic TEXT NOT NULL,             -- kebab-case, unique per active note (see CLI rules)
  body TEXT NOT NULL,              -- markdown
  source_paths TEXT NOT NULL,      -- JSON array of repo-relative paths
  source_hashes TEXT NOT NULL,     -- JSON object {path: hash} snapshot at last verify
  confidence TEXT NOT NULL DEFAULT 'medium',  -- low | medium | high
  status TEXT NOT NULL DEFAULT 'active',      -- active | stale | archived
  last_verified_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  model_id TEXT NOT NULL
);
CREATE INDEX idx_arch_notes_topic ON architecture_notes(topic);
CREATE INDEX idx_arch_notes_status ON architecture_notes(status);
```

Staleness is derived: a note is stale when *any* path in `source_paths` has a current `repo_files.content_hash` that differs from the stored `source_hashes` entry, OR when a source path is missing from `repo_files` entirely. `status='stale'` is only persisted when an agent explicitly calls `mark-arch-note-stale`.

### Code review (v5)

```sql
CREATE TABLE review_runs (
  id TEXT PRIMARY KEY,
  scope_paths TEXT NOT NULL,       -- JSON array
  tier TEXT NOT NULL DEFAULT 'default',  -- default | extended
  started_at TEXT NOT NULL,
  finished_at TEXT,
  git_sha TEXT,
  files_total INTEGER,
  files_reviewed INTEGER,
  summary TEXT,                    -- agent-written wrap-up
  model_id TEXT NOT NULL
);

CREATE TABLE review_findings (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  file_path TEXT,                  -- nullable: cross-cutting finding
  line_start INTEGER,
  line_end INTEGER,
  principle TEXT NOT NULL,
  title TEXT NOT NULL,
  recommendation TEXT NOT NULL,
  severity TEXT NOT NULL,          -- info | low | med | high | critical
  confidence TEXT NOT NULL,        -- low | medium | high
  effort TEXT NOT NULL,            -- trivial | small | med | large
  status TEXT NOT NULL DEFAULT 'open',  -- open | accepted | wontfix | resolved | duplicate
  resolved_in_commit TEXT,
  source_hash TEXT,                -- file_path's hash at write time
  created_at TEXT NOT NULL,
  model_id TEXT NOT NULL,
  FOREIGN KEY (run_id) REFERENCES review_runs(id)
);
CREATE INDEX idx_findings_run ON review_findings(run_id);
CREATE INDEX idx_findings_file ON review_findings(file_path);
CREATE INDEX idx_findings_status ON review_findings(status);
CREATE INDEX idx_findings_severity ON review_findings(severity);
```

Priority is derived at query time:

```
priority = severity_weight * confidence_weight / effort_weight

severity:    info=1  low=2  med=4   high=8   critical=16
confidence:  low=1   medium=2  high=3
effort:      trivial=1  small=2  med=4  large=8
```

Tunable in code if the weights turn out wrong. No `review_scores` table.

### Verification ledger (M10)

Track test execution state reusably so agents can query whether a passing test result is still valid instead of always rerunning.

```sql
CREATE TABLE verification_runs (
  id TEXT PRIMARY KEY,
  command TEXT NOT NULL,              -- e.g., "pytest tests/"
  scope TEXT NOT NULL,                -- e.g., "src/devdb/"
  status TEXT NOT NULL,               -- pending|passed|failed
  git_sha TEXT NOT NULL,
  exit_code INTEGER,
  output TEXT,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  created_at TEXT NOT NULL,
  model_id TEXT NOT NULL
);
CREATE INDEX idx_verification_runs_status ON verification_runs(status);
CREATE INDEX idx_verification_runs_git_sha ON verification_runs(git_sha);

CREATE TABLE verification_inputs (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  file_path TEXT NOT NULL,            -- source file path
  role TEXT NOT NULL,                 -- source|test|fixture|config|dependency|tooling
  content_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  model_id TEXT NOT NULL,
  FOREIGN KEY (run_id) REFERENCES verification_runs(id)
);
CREATE INDEX idx_verification_inputs_run ON verification_inputs(run_id);
CREATE INDEX idx_verification_inputs_file ON verification_inputs(file_path);
```

**Trust boundaries and freshness rules:**

A verification run is *reusable* if:
1. Same command and scope
2. All stored input file hashes match current `repo_files.content_hash`
3. No files outside the input set with broad-scope roles (fixture, config, dependency, tooling) have changed since the run finished

A verification run is *stale* if:
- Any input file hash has changed (file was modified)
- Any input file was removed
- A broad-scope file changed (detected by `_infer_role_from_path`)

**Recency is insufficient.** A passing run from yesterday is stale if any input file hash has changed. Matching hashes and compatible scope are required.

**Scope matching is required.** A run for `pytest src/` cannot substitute for `pytest tests/`.

**File roles are conservative.** Config files, test fixtures, dependencies, and CI tooling always trigger broad scope. Source and test files narrow the scope.

## Review dimensions (tiered)

**Default tier** — 7 lenses, always evaluated when `--tier default` (which is the default):

1. Correctness
2. KISS (simplicity)
3. DRY (duplication)
4. YAGNI (overengineering)
5. Separation of Concerns
6. Error handling
7. Test coverage

**Extended tier** — 11 additional lenses, only when `--tier extended`:

- Security footguns
- Performance hotspots
- Migration safety
- Observability / debuggability
- API boundaries
- Naming clarity
- Dead code
- Coupling / Cohesion
- Configuration clarity
- Documentation accuracy
- Dependency hygiene

Anything outside both tiers goes in as `principle='other'` with a free-form bracketed prefix in the title, e.g. `[concurrency] race in cache invalidation`.

## CLI surface

### Existing (unchanged)

Already shipped, parity invariant: `init`, `add-goal`, `add-plan`, `set-status`, `add-feature`, `add-tightening`, `snapshot-loc`, `feedback-*`, `close-feedback`, `add-suggestion`, `import-feedback-md`, `import-branch-commits`, `list`, `report`.

### New: file inventory

```
devdb scan [--paths PATH ...] [--git-aware] [--dry-run]
  stdout: <scan_run_id>
  stderr: one-line error on failure
  exit:   0 ok | 1 unexpected error
  effects: insert scan_runs row, upsert repo_files
```

### New: architecture notes

```
devdb add-arch-note TOPIC --body BODY --sources PATH [PATH ...] [--confidence low|medium|high]
  stdout: <note_id>
  exit:   0 ok | 1 unexpected | 2 invalid topic | 3 source path not in repo_files

devdb update-arch-note ID [--body BODY] [--sources PATH ...] [--confidence X]
  stdout: ok
  exit:   0 ok | 1 unexpected | 4 note not found

devdb verify-arch-note ID
  stdout: ok | stale (if sources have changed; agent must update-arch-note to clear)
  exit:   0 ok | 1 unexpected | 4 note not found

devdb mark-arch-note-stale ID [--reason TEXT]
devdb archive-arch-note ID
  stdout: ok | exit: 0 | 1 | 4

devdb list-arch-notes [--topic SUBSTR] [--touching PATH] [--stale] [--status active|stale|archived] [--json]
  stdout: human table (default) or JSON array

devdb arch-render [--output PATH]
  stdout: path written
  effects: renders all active notes grouped by topic with provenance footer
  default output: .devdb/architecture.md
```

### New: code review

```
devdb review-start --paths PATH [PATH ...] [--tier default|extended] [--git-sha SHA]
  stdout: <run_id>
  exit:   0 ok | 1 unexpected

devdb review-add-finding RUN_ID --file PATH --principle X --title TEXT \
                                --recommendation TEXT \
                                --severity info|low|med|high|critical \
                                --confidence low|medium|high \
                                --effort trivial|small|med|large \
                                [--line-start N] [--line-end N]
  stdout: <finding_id>
  exit:   0 ok | 1 unexpected | 5 run already finished | 6 invalid principle for tier

devdb review-finish RUN_ID [--summary TEXT]
  stdout: ok
  exit:   0 ok | 1 unexpected | 7 run already finished

devdb review-list [--status STATUS] [--run RUN_ID] [--principle X] [--file PATH] [--limit N] [--json]
  stdout: ranked list (highest priority first) or JSON

devdb review-resolve FINDING_ID [--commit SHA] [--status resolved|wontfix|accepted|duplicate]
  stdout: ok
  exit:   0 ok | 1 unexpected | 8 commit SHA required for status=resolved | 9 finding not found

devdb review-report RUN_ID [--output PATH]
  stdout: path written
  default output: .devdb/reviews/<run_id>.md
```

### Extended: report

`devdb report` keeps its current sections and gains four new ones at the top:

```
## Freshness
last scan: <timestamp> · <N> files indexed · <M> changed since last scan

## Architecture
<N> active notes · <S> stale · <L> low-confidence
top 5 most-recently-verified notes...

## Findings (open)
critical: N · high: N · med: N · low: N · info: N
top 5 by derived priority (file:line · principle · title)

## Latest review run
run <id> · tier <default|extended> · <coverage> files · finished <when>
```

## SKILL.md target content (what M5 ships)

This is what the agent reads. Crystal clear, no soft verbs, every instruction is a command + a condition.

```
---
name: devdb
description: Per-project memory and quality ledger. Use when working in any
  repository that has (or should have) a .devdb/ directory — log feedback,
  architecture knowledge, and code-review findings, and read what previous
  agents already logged before doing new work.
---

# Devdb

One sqlite DB per project at .devdb/development.db. Everyone writes to it.
Everyone reads from it. Replaces the markdown sprawl that buries projects
over time. See README.md for the full origin story.

## Operating modes

There are two modes. Recognize which one you are in before acting.

**Brainstorm mode.** The user is exploring ideas. Casual chatting, half-baked
thoughts, "what if" questions. Your role: scribe + sparring partner. Log
liberally as feedback-user / feedback-model / improvement_suggestions /
goals. Volume over selectivity at capture time — prune later. Do NOT try
to execute. Do NOT propose a 12-step plan. The conversation IS the work.

**Execution mode.** The user says "go", "start M1", "ship it", or similar.
Now you act. Bracket the work with work-on / pause-on. Log async observations
as you find them. Run the session-start ritual first.

If you are not sure which mode you are in, you are in brainstorm mode.

## Session-start ritual (do this first, every invocation)

Run these four commands in order before any code work:

  devdb resume                                        # what was in flight?
  devdb report                                        # high-level state
  devdb list-arch-notes --stale                       # drift warnings
  devdb review-list --status open --severity high     # top debt

If `devdb resume` shows an in-progress plan item, ask the user whether to
continue it or start something new. Do not silently switch contexts.

If `list-arch-notes --stale` returns anything for files you intend to touch,
verify-arch-note (if still accurate) or update-arch-note (if drifted) before
editing those files.

If `review-list --status open` shows high-severity findings on files you're
about to touch, decide explicitly: fix now, or leave them and proceed.
Never edit a file with high-severity findings without acknowledging them.

## Workflow bracket (M1.6, applies during execution mode)

Bracket all real work with these three commands:

  devdb work-on <plan_item_id>          # sets in_progress, logs status_log
  ... do work, log feedback async as you go ...
  devdb pause-on <plan_item_id> --note "next step is X, blocker is Y"

`--note` is REQUIRED on pause-on. No optional. If you cannot say what's next
in one sentence, you do not know what's next, and the next session will be
lost. Pause OFTEN: a coffee break is a pause. End-of-day is a pause. Internet
dropping (pampa case) is a pause that the system has to survive without your
help — so write a real note every time you stop, not just at "session end".

If the session is cut off without a pause, the plan item stays in_progress
and the next agent's `devdb resume` flags it for the user to decide.

## Behavioral rules (cross-cutting, apply always)

**Async observation, no interruption.** When working on code, if you spot
a smell or pattern (duplicate function, dead branch, sketchy assumption):
log it (feedback-codebase or add-tightening) and continue the current task.
Quick note, move on. No permission required. Interruption to ask "should
I fix this?" breaks flow for no benefit. No one gets hurt by a logged
observation; everyone loses when flow breaks.

**Friction-check at real breakpoints (not per turn).** At natural breakpoints
(end of a milestone, after a long file edit, after the user says "cool, next"),
ask ONE short question: "any friction in the last X to log?" — and write
the user's reply directly into feedback-user. NEVER on every turn. Real
breakpoints only.

**No walls of text.** If your response goes past ~40 lines or 5 distinct
ideas, the user loses the thread. Terse responses, concrete claims, one
ask at the end. When uncertain, ask one direct question instead of
generating fluent uncertainty. Exam-jabber (confidently producing
comprehensive-sounding prose when you do not understand) is the failure
mode to avoid.

**Re-read user premises before scoping.** When the user reacts with confusion,
do NOT double down with explanation. Restate the user's premise in plain
words and ask one targeted question. Most WTF moments are context gaps,
not personality clashes.

## Playbook 1: Log feedback

Trigger:
  - You hit friction, a bug, a surprise, or a dead end.
  - You see something praiseworthy (working pattern, good naming, etc.).
  - The user gives you feedback worth keeping.

Commands:
  devdb feedback-model "..." --severity ... --category ...
  devdb feedback-user  "..." --severity ... --category ...
  devdb feedback-codebase "..." --severity ... --category ...

Do not log:
  - "I added feature X" — that's a feature, use add-feature.
  - Tautologies ("the code does what it does").
  - One-time confusion that you resolved within the same turn.

Observational feedback about the user (category=quirks):
  - feedback-user is also for noticing the user's working style — affectionate,
    not corrective. The next agent benefits from operating instructions framed
    as observation rather than rules.
  - Examples worth logging:
      "user types lowercase with typos when fast — do not correct unless asked"
      "user curses ('what the fuck', 'for christs sake') when a response is
       confidently wrong — treat as escalation signal, re-examine the framing"
      "user says 'honest no bs thoughts' as a literal operating instruction,
       not flavor — respond accordingly, hedging will frustrate"
  - Do not log judgmental observations. The bar is: would the user laugh
    reading this back? If no, do not log it.

## Playbook 2: Architecture notes

Trigger:
  - You just spent effort understanding how part of the codebase works
    and the next agent should not have to re-derive it.
  - You're reading a long file to answer a small question — your answer
    is the seed of a note.

Commands:
  devdb add-arch-note TOPIC --body "..." --sources path1 path2 ...
  devdb verify-arch-note ID         # after re-reading and confirming
  devdb update-arch-note ID --body "..." --sources ...   # after drift
  devdb list-arch-notes --touching path   # before touching that file

Topic rules:
  - kebab-case, 3-40 chars
  - must name a code surface (cli-entrypoint, feedback-loop, schema-migrations)
  - banned: misc, general, notes, stuff, improvements

Body rules:
  - 2-5 sentences. Plain prose.
  - Name files. Do not paste code. Do not speculate.
  - If you can't name files, the note is too abstract — refine.

Read habit:
  - list-arch-notes before reading source. If a note covers what you need,
    use it. Re-reading code that's already noted is waste.

## Playbook 3: Code review

Trigger (only when explicit):
  - User asks "review the code", "find smells", "what's getting messy",
    or names a specific area to audit.
  - Do not start unsolicited reviews — that wastes tokens.

Lifecycle:
  devdb review-start --paths src tests   # prints run_id
  devdb review-add-finding RUN_ID --file ... --principle ... --severity ... \
                                  --confidence ... --effort ... \
                                  --title "..." --recommendation "..."
  devdb review-finish RUN_ID --summary "..."

Tier:
  - default: 7 principles. Cap 3 findings per file. Anything more = principle misuse.
  - extended: 11 more principles. Cap 5 findings per file. Use only when asked.

Per finding:
  - Cite file:line.
  - Title states the problem in 6-10 words.
  - Recommendation is a specific change, not "consider refactoring".
  - severity * confidence / effort is the priority. Honesty matters more
    than dramatic ratings.

Never log:
  - "Looks fine" findings.
  - Vague vibes ("this feels weird").
  - Restatements of generic style guides.

Close the loop:
  - When a commit fixes a finding: review-resolve FINDING_ID --commit SHA
  - When you decide not to fix: review-resolve FINDING_ID --status wontfix
  - When a finding turns out to be a dupe: --status duplicate

## Why these habits matter

A project's devdb is its memory. The session you're in is one of many.
Write so the next agent (or you, in a week) can pick up cold. Read so you
don't waste time re-deriving what's already known.
```

## Milestones

Each milestone is one branch, one PR, all tests green, only the files in its scope touched. Conventional-commit prefix per milestone (`feat(migrations):`, etc.). No drive-by edits. If something else is wrong, log a finding and move on.

### M1 — Migration runner

**Goal.** Replace `try/except OperationalError: pass` with a proper migration runner. No new tables visible to users yet.

**Files created.**
- `src/devdb/migrations.py` — `Migration` dataclass, ordered list, runner
- `tests/test_migrations.py`

**Files modified.**
- `src/devdb/schema.py` — collapse to a thin delegate that calls `migrations.run_all(conn)`
- `src/devdb/db.py` — minor import shuffle if needed

**Do not touch.**
- `src/devdb/cli.py`
- `src/devdb/importers.py`, `reporting.py`, `git_helpers.py`
- Any existing test in `tests/test_cli.py`

**Acceptance checklist.**
- [ ] `schema_migrations` table exists after running `devdb init` on a fresh DB
- [ ] v1 row inserted with description matching "initial tables"
- [ ] v2 row inserted with description matching "feedback status + proposed_fix"
- [ ] Running migrations on a DB that already has all current tables succeeds without errors and records v1+v2
- [ ] Second invocation is a no-op (same migration count, no duplicate inserts)
- [ ] No `try/except sqlite3.OperationalError: pass` remains in any source file
- [ ] All 26 existing tests pass
- [ ] 4 new tests: fresh-DB apply, existing-state apply, idempotent rerun, mid-version failure rolls back atomically

### M1.5 — CLI hygiene pass

**Goal.** Make the CLI surface predictable and machine-readable before M2 onward introduces ~13 new verbs that would otherwise inherit the current drift. Discovered by dogfooding (see Amendments section). Backward-compatible, no breaking changes.

**Files modified.**
- `src/devdb/cli.py` — three changes:
  1. **Severity normalization layer.** Every `--severity` argument accepts the union of all current enums (`info|low|med|medium|high|critical`) and a normalizer maps them per-table to the values that table's CHECK constraint expects. Same for priority on `add-suggestion`.
  2. **`--json` flag on every write verb.** When passed, the verb emits `{"id": "<uuid>"}` to stdout and suppresses any other output. Default behavior unchanged (still prints raw id).
  3. **Arg-shape convention going forward (documented, not retroactively forced).** Title is the only positional. Everything else is a flag. Existing positional-heavy verbs (`add-goal kind title`) keep their current shape for backward compatibility but gain a deprecation note in `--help`.

**Files created.**
- None.

**Do not touch.**
- `schema.py`, `migrations.py` (no schema change)
- `importers.py`, `reporting.py`, `git_helpers.py`, `db.py`
- Existing test assertions — only add new cases.

**Acceptance checklist.**
- [ ] `devdb feedback-codebase "..." --severity med` works (alias for `medium`)
- [ ] `devdb add-tightening "..." --severity medium` works (alias for `med`)
- [ ] `devdb feedback-codebase "..." --severity high --json` prints exactly `{"id": "<uuid>"}` and nothing else
- [ ] All write verbs (`add-goal`, `add-plan`, `set-status`, `add-feature`, `add-tightening`, `snapshot-loc`, `feedback-*`, `close-feedback`, `add-suggestion`) accept `--json`
- [ ] `devdb --help` for any positional-heavy verb shows a one-line deprecation note pointing to the new convention
- [ ] All 26 existing tests pass; ~6 new tests added for aliases and `--json` shape
- [ ] No CHECK-constraint failures on insert when aliases are used (the normalizer ran)

### M1.6 — Workflow bracket: work-on / pause-on / resume

**Goal.** Make work interruption-resilient (the user develops without reliable internet in the pampa — interruption is the dominant case, not an edge case). Three new CLI verbs and a SKILL.md ritual block. **No schema change** — relies on existing `plan_items.status` and `status_log` rows.

**Files modified.**
- `src/devdb/cli.py` — add three subcommands:
  - `devdb work-on <plan_item_id>` — sets `plan_items.status='in_progress'`, inserts `status_log` row with `note='started'`
  - `devdb pause-on <plan_item_id> --note <REQUIRED>` — inserts `status_log` row with `note='paused: <text>'`, keeps status as `in_progress`. The required `--note` is the discipline.
  - `devdb resume` — finds all `in_progress` plan_items, joins to most-recent `status_log` per item, prints "you were on X, last action Y, paused with note: Z"

**Files created.**
- `tests/test_workflow_bracket.py`

**Do not touch.**
- `schema.py`, `migrations.py` — no schema change
- Existing `set-status` verb (work-on / pause-on are syntactic sugar over it)

**Acceptance checklist.**
- [ ] `devdb work-on <id>` sets status to in_progress and logs a status_log row
- [ ] `devdb pause-on <id>` without `--note` exits 2 with stderr message (required)
- [ ] `devdb pause-on <id> --note "..."` succeeds and the note is retrievable via `devdb resume`
- [ ] `devdb resume` with no in_progress items prints "no in-flight work"
- [ ] `devdb resume` with multiple in_progress items prints them ordered by most-recent status_log entry
- [ ] All previous tests still pass; 4-5 new tests
- [ ] SKILL.md target content (M5) gets a "session-start ritual" block updated to mention these three verbs

### M1.7 — Plan-tracking schema: plan_item_acceptance and plan_item_files

**Goal.** Make acceptance criteria and file-scope first-class structured data instead of markdown buried inside `plan_items.body`. Lets `devdb work-on M2` print the file allowlist, `devdb status` report "3 of 8 acceptance criteria met", and eventually a `devdb check-scope` command compare against `git diff`. User asked for this explicitly: "We need a plan and implementation tracking feature and db tables for that."

**Schema additions (migration appended after M1.5's most recent version).**

```sql
CREATE TABLE plan_item_acceptance (
  id TEXT PRIMARY KEY,
  plan_item_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  criterion TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',  -- 'open' | 'met' | 'wontfix'
  evidence TEXT,                          -- commit SHA, test name, or note
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  model_id TEXT NOT NULL,
  FOREIGN KEY (plan_item_id) REFERENCES plan_items(id)
);
CREATE INDEX idx_acceptance_plan ON plan_item_acceptance(plan_item_id);
CREATE INDEX idx_acceptance_status ON plan_item_acceptance(status);

CREATE TABLE plan_item_files (
  id TEXT PRIMARY KEY,
  plan_item_id TEXT NOT NULL,
  path TEXT NOT NULL,
  role TEXT NOT NULL,                    -- 'create' | 'modify' | 'forbidden' | 'touched'
  created_at TEXT NOT NULL,
  model_id TEXT NOT NULL,
  FOREIGN KEY (plan_item_id) REFERENCES plan_items(id)
);
CREATE INDEX idx_plan_files_plan ON plan_item_files(plan_item_id);
CREATE INDEX idx_plan_files_role ON plan_item_files(role);
```

**Files modified.**
- `src/devdb/cli.py` — add four subcommands: `add-acceptance`, `meet-acceptance`, `add-plan-file`, `show-plan`
- `src/devdb/migrations.py` — append the v6 migration (or whichever version)

**Files created.**
- `tests/test_plan_tracking.py`

**Do not touch.**
- Existing `plan_items` table — no column changes
- `status_log`, `goals`, `feedback` — untouched

**Acceptance checklist.**
- [ ] Acceptance rows can be added to existing plan_items with ordinal preserved
- [ ] Marking acceptance met updates `updated_at` and sets `evidence` if provided
- [ ] `devdb show-plan <id>` composes plan_items + acceptance + files + status_log into one read
- [ ] `devdb work-on <id>` (from M1.6) is updated to also print the file allowlist and unmet acceptance criteria
- [ ] All previous tests pass; ~6 new tests

### M1.8 — Active scribe: CLI coaching prompts after every write verb

**Goal.** Make the CLI an active partner that nudges discipline, not a passive sink. After every write verb, emit a short reflection prompt to **stderr** (preserves the stdout-id contract). Agent sees the nudge in the tool result and decides whether to log more, ask the user something, or move on. The mechanism that catches "polite agreement masking comprehension gap" — proven by the user themselves in this session.

**Files modified.**
- `src/devdb/cli.py` — after every successful write verb, call a new `_emit_coaching_prompt(conn, kind, row_id)` function that writes to stderr.
- `src/devdb/coaching.py` (new) — prompt rotation and signal-driven selection.

**Files created.**
- `src/devdb/coaching.py`
- `tests/test_coaching.py`

**Do not touch.**
- Stdout output of any verb (must stay the row id only).
- Existing tests that check stdout (untouched; coaching goes to stderr).

**Escalation-only (current).**
No random prompt rotation. Emit to stderr only when escalation signals fire:
- Prior `feedback` exists and none in the last 10 minutes, or
- `DEVDB_USER_TURNS` / `.devdb/user_turns` > 15 with no feedback in the last 10 minutes.

Prompt: "you've been working a while without logging — anything?"

**Acceptance checklist.**
- [ ] Every existing write verb (add-goal, add-plan, add-feature, add-tightening, snapshot-loc, feedback-*, close-feedback, add-suggestion, set-status) emits a coaching prompt to stderr after a successful write.
- [ ] Stdout of every write verb is unchanged (still just the row id, or `{"id": ...}` with `--json`).
- [ ] `devdb --quiet` accepted for script compatibility; escalation prompts are not suppressible.
- [ ] All previous tests pass; ~5 new tests assert stderr content and `--quiet` behavior.
- [ ] v2 (not in this milestone): add `coaching_prompts_log` table to tune frequency over time.

### M2 — File inventory

**Goal.** Track every non-ignored file in the repo with content_hash. The substrate for staleness everywhere downstream.

**Files created.**
- `src/devdb/inventory.py` — file walker, hasher, language/kind heuristic, ignore handling
- `tests/test_inventory.py`

**Files modified.**
- `src/devdb/cli.py` — add `scan` subcommand
- `src/devdb/migrations.py` — append v3
- `src/devdb/reporting.py` — only if the freshness section lands here (else M5)

**Do not touch.**
- Any existing CLI verb
- `schema.py`, `db.py`

**Acceptance checklist.**
- [ ] `devdb scan` on a fresh repo populates `repo_files` for all non-ignored files
- [ ] Ignored paths from default set are excluded
- [ ] `.gitignore` patterns are respected when present
- [ ] `--git-aware` uses `git ls-files` instead of filesystem walk
- [ ] Re-scan with no changes: same content_hashes, updated `last_seen_at`, `scan_runs.files_changed=0`
- [ ] Modified file: new content_hash, `scan_runs.files_changed > 0`
- [ ] Deleted file: row remains in `repo_files`, `last_seen_at` stops advancing, `scan_runs.files_removed > 0`
- [ ] Binary file (null-byte heuristic): `kind='binary'`, `content_hash IS NULL`
- [ ] Stdout prints `<scan_run_id>` only
- [ ] All previous tests still pass

### M3 — Architecture notes

**Goal.** Agents can write and read architecture notes with provenance, and staleness is derivable.

**Files created.**
- `src/devdb/architecture.py` — note CRUD, staleness query, render
- `tests/test_architecture.py`

**Files modified.**
- `src/devdb/cli.py` — add 7 verbs (`add-arch-note`, `update-arch-note`, `verify-arch-note`, `mark-arch-note-stale`, `archive-arch-note`, `list-arch-notes`, `arch-render`)
- `src/devdb/migrations.py` — append v4

**Do not touch.**
- Review-related anything (M4)
- `reporting.py` (its arch section lands in M5)

**Acceptance checklist.**
- [ ] `add-arch-note` with valid topic and sources captures `source_hashes` from current `repo_files`
- [ ] Topic regex enforced: `^[a-z][a-z0-9-]{2,40}$`, banned topics rejected with exit 2
- [ ] Sources must exist in `repo_files` or exit 3 (with stderr naming the missing path)
- [ ] `verify-arch-note` on unchanged sources updates `last_verified_at` and prints `ok`
- [ ] `verify-arch-note` on drifted sources prints `stale` and exit 0 (informational, not error)
- [ ] `list-arch-notes --stale` returns notes whose source_hashes don't match current
- [ ] `list-arch-notes --touching PATH` returns notes citing that path
- [ ] `arch-render` writes `.devdb/architecture.md` with notes grouped by topic, provenance footer per note
- [ ] Tests: write/read roundtrip, staleness detection, verify clears stale, render stability (golden file)

### M4 — Code review

**Goal.** Resumable batched reviews with ranked findings and explicit resolution.

**Files created.**
- `src/devdb/review.py` — run lifecycle, finding CRUD, priority query, report renderer
- `tests/test_review.py`
- `tests/fixtures/review_golden_report.md` (golden file for renderer)

**Files modified.**
- `src/devdb/cli.py` — add 6 verbs (`review-start`, `review-add-finding`, `review-finish`, `review-list`, `review-resolve`, `review-report`)
- `src/devdb/migrations.py` — append v5

**Do not touch.**
- `architecture.py` (M3)
- `reporting.py` (M5)

**Acceptance checklist.**
- [ ] `review-start` returns a run_id and inserts a row with `started_at` set, `finished_at NULL`
- [ ] `review-add-finding` to a finished run exits 5 with stderr message
- [ ] Invalid principle for tier exits 6 (e.g. `--tier default --principle security` rejected)
- [ ] `review-finish` sets `finished_at` and `files_reviewed`; second call exits 7
- [ ] `review-list` orders by derived priority descending
- [ ] `review-resolve --status resolved` without `--commit` exits 8
- [ ] `review-resolve --status wontfix` works without commit
- [ ] `review-report` matches the golden fixture (modulo timestamps)
- [ ] Tests: full lifecycle, error paths, priority ordering with known weights

### M4.5 — Grass-cutter: heuristic discovery of cuttable code

**Goal.** The killer feature. Reframes code review from "log smells one by one" to "find what should be cut, automatically, cheap." Solves the central pathology of AI-assisted development: agents want to build, humans want to ship, no one cuts the grass.

**Concrete scenario.** Two weeks into agent-assisted work, `cli.py` is at 1,200 lines, three functions do nearly the same thing, eight functions are never called, and four TODO comments are six months old. User runs `devdb suggest-cuts`. Output: 15 ranked candidates, each a row in `review_findings` with `principle` set to `dead`, `inlinable`, `sprawl`, `swelling`, `duplication`, or `staleness`. User runs `devdb review-list --principle dead` and decides keep/cut per row. Half the candidates get resolved as `wontfix`, half as `resolved` (with the commit SHA that removed them). The codebase shrinks.

**Files created.**
- `src/devdb/grass_cutter.py` — AST walker, call-graph builder, duplication hasher, LoC growth checker, comment scanner.
- `tests/test_grass_cutter.py`
- `tests/fixtures/grass_fixtures/` — small synthetic codebases exercising each heuristic.

**Files modified.**
- `src/devdb/cli.py` — add `suggest-cuts` subcommand.

**Do not touch.**
- `review.py` from M4 — grass-cutter inserts findings via the same code path as manual review-add-finding.
- The LLM. None of these heuristics need a model — pure AST + git history + loc_snapshots.

**Heuristic catalog (v1, all cheap to compute).**
- `principle=dead` — functions defined but called 0 times anywhere in the inventory (via AST call-graph)
- `principle=inlinable` — functions called exactly once (candidates for inlining)
- `principle=sprawl` — files > N lines (configurable; default 500; storyblender.py at 8,333 is the prototype)
- `principle=swelling` — files whose LoC grew >X% week-over-week (uses existing `loc_snapshots`; default X=20)
- `principle=duplication` — two or more functions with identical AST body hash (within configurable noise tolerance)
- `principle=staleness` — TODO/FIXME/XXX comments older than N days (uses `git blame`; default N=90)

**CLI contract.**
```
devdb suggest-cuts [--paths PATH ...] [--principles P [P ...]] [--dry-run] [--json]
  stdout: <run_id> (auto-starts a review_run with tier=grass-cutter)
  stderr: summary line — "found N candidates: dead=A inlinable=B sprawl=C ..."
  exit:   0 ok | 1 unexpected | 2 no files in inventory (run scan first)
  effects: inserts review_run row + N review_findings rows
```

**Acceptance checklist.**
- [ ] Synthetic fixture with one dead function → exactly one `principle=dead` finding emitted.
- [ ] Fixture with two identical-body functions → exactly one `principle=duplication` finding (not two — collapsed to a pair).
- [ ] Fixture with a 1000-line file (above default 500) → one `principle=sprawl` finding.
- [ ] `--principles dead duplication` runs only those two heuristics.
- [ ] `--dry-run` prints what would be inserted without writing.
- [ ] `review-resolve` works normally on grass-cutter findings (same flow as manual review).
- [ ] All previous tests pass; ~12 new tests (one per heuristic × happy + edge).

### M5 — Skill rewrite + report extension (THE PRODUCT)

**Goal.** The thing the user actually sees. Agents now behave differently because SKILL.md tells them to.

**Files created.**
- None.

**Files modified.**
- `skills/devdb/SKILL.md` — rewrite to the target content above (replaces current ~40 lines)
- `src/devdb/reporting.py` — add four new sections at the top of `devdb report`: Freshness, Architecture, Findings, Latest review run
- `tests/test_cli.py::TestReport` — extend to assert new sections appear when data is present and gracefully omit when empty

**Do not touch.**
- Any schema or CLI logic — purely additive for reporting and authoring SKILL.md.

**Acceptance checklist.**
- [ ] SKILL.md contains all three playbooks and the session-start ritual block
- [ ] No soft verbs in SKILL.md ("consider", "evaluate", "review the codebase as appropriate"). Every instruction is a command + a condition.
- [ ] `devdb report` on a populated DB shows all four new sections
- [ ] `devdb report` on an empty DB still produces valid markdown (sections may be empty but headers present or gracefully omitted — decide and document)
- [ ] All previous tests still pass
- [ ] New test fixture exercises a populated DB (scan + arch notes + review run) and snapshots the report

### M6 — Polish (optional, post-MVP, any order)

Each item ships on its own merit. None is required for the MVP.

- **`agent_documents` table + `ingest-agent-docs`** — discover CLAUDE.md, AGENTS.md, .cursorrules, etc., hash them, let agents log summaries. Migration v6.
- **`devdb context [--files ...] [--task ...] [--strict] [--json]`** — convenience read combining arch notes touching files + open findings + stale warnings, with byte budget and exit codes. Useful but `devdb report` covers 80% of the value.
- **`devdb diff-since <ref>`** — list files changed and arch notes / findings touching them, to drive targeted re-verify.
- **`devdb status --json`** — machine-readable single-line status (schema version, last scan, open runs, stale notes count, open high-severity findings count) for agents picking up mid-stream.
- **Abandoned review-run detection** — `review-list --abandoned` flags runs older than N days without finish; `--force-finish` lets you close them.

### M7 — Cross-project federation via SQLite ATTACH (post-MVP, optional)

**Goal.** Optional cross-project memory layer with no hybrids. Each project keeps its own `.devdb/development.db`. A new tool `devdb across` reads a `~/.devdb-projects` registry file and ATTACHes them at query time for cross-project SQL queries. No central database. No sync. No app dependency. No vendor lock-in. Survives the "no hybrids" principle filter.

**Why this is M7 and not earlier.**
The user does not yet have a corpus to query against (programming with agents is genuinely new). Federation pays off once devdb is deployed across enough projects to accumulate data. Until then this would be premature scaffolding. The first-day query that will actually save the user time is well-defined (cross-project code hygiene), so the design is locked even if the ship date is open.

**New files (no schema change to existing tables).**
- `~/.devdb-projects` — text file in user home: one path per line, optionally with `<path> <alias>`
- `src/devdb/federation.py` — registry reader, ATTACH composer, built-in query catalog
- `tests/test_federation.py`

**CLI verbs.**
- `devdb register <PATH> [--alias NAME]` — add a devdb to the registry
- `devdb forget <ALIAS>` — remove from the registry
- `devdb list-projects` — show registered projects with row counts per table
- `devdb across <QUERY_NAME> [--json]` — run a built-in cross-project query
- `devdb across-sql <SQL>` — run an ad-hoc SQL against all attached devdbs (advanced)

**Built-in queries (first-day value).**
1. **code-hygiene-cross** — `review_findings` filtered by principle in (dry, kiss, soc, yagni) across all projects, ordered by severity then recency. This is the query the user named explicitly as the one they would have run a year ago.
2. **loc-trend** — `loc_snapshots` aggregated per project showing growth over time.
3. **similar-feedback** — `feedback` rows with matching category or keyword across projects.
4. **open-debt** — high-severity findings or feedback with status='open' across all projects.

**Companion schema (per-project, lands with M7).**

```sql
CREATE TABLE project_links (
  id TEXT PRIMARY KEY,
  from_table TEXT NOT NULL,
  from_id TEXT NOT NULL,
  to_project TEXT NOT NULL,    -- alias from the registry
  to_table TEXT NOT NULL,
  to_id TEXT NOT NULL,
  relation TEXT NOT NULL,       -- 'same-as' | 'similar' | 'caused-by' | 'fixed-in' | 'see-also'
  note TEXT,
  created_at TEXT NOT NULL,
  model_id TEXT NOT NULL
);
CREATE INDEX idx_project_links_from ON project_links(from_table, from_id);
CREATE INDEX idx_project_links_to ON project_links(to_project, to_table, to_id);
```

Hand-curated cross-project pointers. No similarity scoring. No embeddings. No inference. The value is in explicit recognition: "I have seen this before, here."

**Acceptance checklist.**
- [ ] `~/.devdb-projects` registry can be written, read, and edited via CLI
- [ ] `devdb across code-hygiene-cross` returns ranked findings from all registered devdbs
- [ ] `devdb across` exits cleanly when registry is empty or a registered path no longer exists
- [ ] `project_links` rows can be inserted and queried from either side
- [ ] No changes to existing per-project schema beyond adding `project_links`

## Invariants (must hold at every milestone boundary)

1. `pytest -q` green. Existing 26 tests + everything added so far.
2. `devdb report` on an existing dramatica-auto-style DB still works without errors.
3. No silent error swallowing. `try/except` with a bare `pass` is forbidden in this codebase from M1 onward.
4. No file outside the milestone's "Files created/modified" list is touched in that PR.
5. SQLite schema only grows. No column rename, no column drop. Add new tables and columns, never restructure.
6. Stdout contracts are stable. Anything that prints an ID prints *only* that ID on stdout (no chatty wrappers). Human-friendly output goes to stderr or behind a `--verbose` flag.

## Test posture

- Every new CLI verb: at least one happy-path test + one failure-path test (error exit code asserted).
- Hashing and staleness: determinism tests with fixtures.
- Migrations: fresh-apply, existing-state-apply, idempotent-rerun, mid-version-rollback.
- Renderers (`arch-render`, `review-report`, `report`): golden-file tests with deterministic fixtures, timestamps masked.
- Existing tests are touched only to add cases, never to remove or weaken assertions.

## Definition of done (the whole feature)

The extension is done when:

- Milestones M1, M1.5, M1.6, M1.7, M1.8, M2, M3, M4, M4.5, M5 have shipped and are tagged. (M6 polish bucket and M7 federation are optional, not blocking.)
- A fresh project can: `devdb init` → `devdb scan` → agent writes an arch note → agent writes a review finding → next session, agent reads them all via `devdb report` and acts on them.
- The SKILL.md prose is what agents follow without further explanation from the user.
- This repo dogfoods itself — `.devdb/development.db` here has at least 3 arch notes and one completed review run for `src/devdb/`.

M6 items are nice-to-have, scheduled when needed, not blocking. M7 (federation) is post-MVP and pays off only once devdb is deployed across enough projects to have a corpus.

## Provenance: this spec is itself a proof of value

This specification was developed in one continuous brainstorm session on 2026-05-22 between the project author and an Opus 4.7 agent (Claude Code). The conversation produced ~60 durable rows in this repo's own `.devdb/development.db`: every decision, every framing miss, every course-correction, every casual aside that turned into a principle.

The session itself became the strongest argument for why devdb needs to exist:

- Multiple times during the session, the model missed framing that the user had already provided two messages earlier — exactly the context gap devdb is designed to solve. Every miss was logged as a `feedback-model` row, turning failures into durable corrections.
- The brainstorm naturally surfaced design principles (state-first, no hybrids, frictionless) that would have been lost in a markdown-based workflow but survive as `goals`, `feedback-user`, and `improvement_suggestions` rows.
- The model exhibited the exact failure modes the system is meant to prevent (walls of text, asking questions already answered, deferring instead of deciding) and logged each one as it happened.
- The user explicitly noted, in the closing message, that "this conversation exactly highlights why we need this project."

A future agent reading `devdb report` from this repo can reconstruct the entire thinking — including the rejected paths (Obsidian, knowledge-graph triples, review scores), the locked decisions, the operating principles, and the implementation order — without re-having the conversation. That capability is what this project ships.

The session is closed. The implementation starts when the user says "go" on M1.
