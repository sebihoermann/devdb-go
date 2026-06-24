---
name: codegraph
description: Use when working in a repository indexed by CodeGraph (.codegraph/ exists at the repo root). Reach for codegraph_explore, codegraph_node, codegraph_callers, or codegraph_search before falling back to grep, Glob, or whole-file reads. Covers tool selection, anti-patterns, index hygiene, and self-audit logging when codegraph was skipped.
---

# CodeGraph

A pre-indexed code intelligence layer. When `.codegraph/` exists at the repo root, the project is already parsed into a queryable symbol graph. Use the graph instead of re-reading the source.

> **Quick rule:** before any grep / Glob / Read on an indexed repo, open with one codegraph call. If the answer requires reading more than one file or understanding a flow, `codegraph_explore` returns the verbatim source plus caller/callee trails in a single capped call.

## 1. When to load this skill

- `.codegraph/` exists at the repo root (look for `.codegraph/codegraph.db`, `.codegraph/daemon.pid`) → load this skill.
- No `.codegraph/` → do **not** initialize mid-task; indexing is the user's decision. Tell them once, then fall back to grep/Read and load the `devdb` skill to log the gap as feedback.

## 2. Pre-flight: confirm the index is live

A stale index is worse than no index. Before trusting a result, run:

```bash
codegraph status
```

Expect a report like `Files: 261 · Nodes: 3,099 · Edges: 10,504 · Backend: node:sqlite`. If `daemon.pid` is older than the last commit on the branch, refresh:

```bash
codegraph sync
```

Do not run `codegraph init` mid-task without asking — that re-parses the whole tree and writes files the user may not want.

## 3. Tool decision tree

| Question | Tool | Why |
|---|---|---|
| How does X work? Where is X defined? What calls X? What does X's source look like? | `codegraph_explore "<symbols or question>"` | Returns verbatim source for the matching symbols plus the call path between them and a blast-radius list of dependents, in one capped call. The verbatim source block **is** a Read. |
| I need one specific symbol's full body + who calls it + who it calls | `codegraph_node <symbol>` | Tighter than `explore`; returns one symbol or one whole file (when called with a file path, no symbol). |
| I need to know every caller of X (audit prep, blast-radius before deletion) | `codegraph_callers <symbol>` | Flat caller list, no source bodies. Use this when you want the dependency map, not the code. |
| I have a partial name and want to locate the symbol | `codegraph_search <partial>` | Symbol-name search; cheap. |
| I'm looking for a literal string (comment, log line, error text) | `grep` | codegraph is symbol-aware, not text-aware. Don't fight it. |
| I know the exact line and need to edit it | `Edit` directly | No call needed; you already have what you need. |

**Default first move:** `codegraph_explore` with one query that names the symbols and any relevant file:line hints. The hint cuts down what the graph returns.

## 4. Anti-patterns

These came from real feedback in this repo. Do not repeat them.

- **Don't grep for a function name.** `grep -rn "SetItemStatus"` returns text. `codegraph_explore "SetItemStatus"` returns the function source plus its callers plus the status_log it touches, in one call.
- **Don't re-Read a file codegraph already returned verbatim.** The source blocks in a codegraph result are read from disk on the call and byte-for-byte identical to what `Read` returns. Reading them again is wasted tokens.
- **Don't codegraph for one-line tweaks.** If you already have the line and you're about to call `Edit`, skip the codegraph step. The skill is for understanding flows, not for ceremony.
- **Don't initialize codegraph mid-task.** A user who hasn't run `codegraph init` has made a deliberate choice. Log the gap as `devdb feedback add --role codebase --category skill --severity low` and continue.
- **Don't run two codegraph calls when one will do.** If `explore` returned the call graph and the source, you have what you need. The `node` / `callers` tools are for narrower follow-ups.

## 5. Index hygiene

The `.codegraph/` directory is generated state. Treat it like `.devdb/`: not for git, not for review.

- Add `.codegraph/` to the project's `.gitignore`. If you forgot, the index will show up as untracked and tempt the next agent into committing it.
- Refresh after large code changes: `codegraph sync` (incremental) or `codegraph index` (full rebuild).
- One project = one daemon. If `daemon.pid` is gone but `codegraph.db` exists, the daemon died — restart by running any codegraph command; the daemon respawns on first use.
- `codegraph.db-shm` and `codegraph.db-wal` are SQLite WAL files. They appear in `git status` only if `.codegraph/` isn't gitignored.

## 6. Self-audit on skip

If you call `grep`, `Glob`, or `Read` on an indexed repo **without a prior codegraph call in the same task**, immediately log:

```bash
devdb feedback add --role model --category audit --severity low --note "Skipped codegraph in <file or query>; reason: <why grep/Read was right here>"
```

This is the pattern that produced feedback `[030a4551]` in this repo. Codifying it makes the friction visible during the session instead of after, and gives future agents a baseline to spot regressions.

If you called codegraph and **then** fell back to grep/Read because the graph was incomplete or stale, log the same note but mark `--note "...; codegraph returned N symbols, M files; fallback to grep because <reason>"`. The delta is the signal that the index needs refreshing.

## 7. Concrete examples from this repo

These are real first-moves from a triage session on `devdb-go`. They show the difference between codegraph-first and grep-first.

### Good: `codegraph_explore` for an arch-note verification

Question: *is the SQL table-name interpolation guard from the arch note still enforced after recent commits?*

```text
codegraph_explore "archive.Restore service.go:666 rowCount writeTableJSONL ColumnNames PRAGMA table_info SQL identifier validation"
```

Result: 4 files, one call flow (`Restore → restoreLocSnapshot → NewID`), blast radius listing every caller plus the test files. The answer (1 of 3 sites guarded, 2 still unvalidated) was visible in the verbatim source block without any further reads.

### Good: `codegraph_explore` for a bug investigation

Question: *does `plan item pause` on a never-started item silently flip its status?*

```text
codegraph_explore "cmdPlanItemPause SetItemStatus pause plan_items status transition planned in_progress"
```

Result: call chain `cmdPlanItemPause → PauseItem → SetItemStatus`, full source of each, plus 11 callers of `SetItemStatus` and 6 callers of `archiveRow` (the related archive path). The bug (`SetItemStatus(db, id, "in_progress", ...)` unconditional) was visible without opening any other file.

### Counter-example: the same questions, grep-first

```bash
grep -rn "func PauseItem" internal/
grep -rn "ErrNoteRequired" internal/
grep -rn "status" internal/domain/planning/service.go | head
# Read internal/domain/planning/service.go (621 lines)
# Read internal/domain/planning/errors.go (31 lines)
# Read internal/cli/commands.go lines 727-750
```

Six tool calls where one `codegraph_explore` would have done. The cost shows up in the ledger as feedback `[030a4551]` (closed) and in the original session's git log as multiple extra round-trips.

## Cheatsheet

```text
codegraph explore "<symbols or question>"   # first move
codegraph node <symbol-or-file>             # one symbol or one file
codegraph callers <symbol>                  # who calls X
codegraph search <partial>                  # locate by partial name
codegraph status                            # index health
codegraph sync                              # refresh after edits
```

Load this skill with `skill codegraph`. Pair it with `skill devdb` for any project that has `.devdb/` — the two cover most of what an agent needs in an indexed repo with a memory ledger.