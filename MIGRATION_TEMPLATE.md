# Migration Plan Template

> **Reusable template** for migrating a `devdb` ledger across versions, repos, or rewrites.
> Derived from the `python → go` devdb cutover executed 2026-06-19. Copy this file, fill the
> bracketed `[...]` placeholders, and adapt the step counts to your migration's actual surface area.
>
> Sections are numbered to match the originating plan; renumber as needed for your migration.

---

## 0. Frontmatter

```
# Migration Plan: <FROM> → <TO>

**Status:** Draft | Executed | Archived
**Snapshot date:** YYYY-MM-DD
**Source:** <path to source tool/repo>
**Target:** <path to target tool/repo>
**Targets (N):** [list each project with a `.devdb/development.db`]
```

---

## 1. Background

One paragraph: why this migration, what's changing at a high level (e.g., command grammar,
storage shape, status enums). Cite the source-of-truth parity doc if one exists.

Common ingredients:
- New tool ships an importer (`import <old>`) — copy mode + `--apply` in-place mode
- Command grammar changes (e.g., `verb-noun` → `noun verb`)
- Schema enums tighten on import (cite the enum-mapping table)
- Hub / metadata layer is rebuilt, not carried over

---

## 2. Goals & non-goals

**Goals** — bullet list. Always include:
- Replace `<OLD>` end-to-end with `<NEW>`.
- Migrate every `<OLD>`-created data file without losing auditable history.
- Preserve agent workflow continuity (skill symlinks, AGENTS.md, wrapper scripts).

**Non-goals** — bullet list. Always include:
- Re-implementing intentionally-skipped source features (cite the skip table).
- Maintaining a compatibility shim if hard-cut was chosen.
- Touching the public `<OLD>` repo. Cutover is local to this machine.

---

## 3. Pre-migration (M-day minus)

| # | Task | Tool | Output |
|---|------|------|--------|
| 3.1 | Inventory every ledger file | `find $HOME $WORKSPACE -name development.db` | List of paths + sizes — search **both** $HOME and $WORKSPACE |
| 3.2 | Snapshot each DB before any change | `cp <db> <db>.pre-<target>-$(date +%F)` then tar them: `tar -czf backups-$(date +%F).tar.gz <list>` | Single archive per migration |
| 3.3 | Capture schema kind + row counts | read-only sqlite3 probe (`PRAGMA …`, `SELECT COUNT(*)`) | `reports/before.csv` |
| 3.4 | Identify source-only tables that won't be ported | sqlite3 probe | Decision list: archive vs. accept loss |
| 3.5 | **Enumerate callsites using old verbs** — wrapper scripts, AGENTS.md, cron, systemd, .envrc, Makefile | `grep -REn -- "<old-verb-regex>"` | Hotspots list, grouped by file. Required because hard-cut rewrites them all in one pass |
| 3.6 | Confirm target toolchain available | `<tool> --version` (≥ version expected) | Build OK |
| 3.7 | Build target CLI from checkout | `cd <target> && <build>` + `<test>` | Tests green, binary at known path |
| 3.8 | Run parity + importer tests | `<test>` targeted at importer | Functional tests pass (note any env-dependent skips) |
| 3.9 | Backup hub / registry files | `cp -a` to a dated dir | Backed up before hub rebuild |
| 3.10 | **NEW: Python-value audit (or source-value audit)** | sqlite3 query for status fields outside target enum | Per-project list of values that need normalization before import |

§3.10 catches what §4 will crash on. One query per status field, per project:

```sql
-- example: feedback status values in source
SELECT DISTINCT status FROM feedback;
```

For each value outside the target enum, decide:
- Map (most common — `deferred`/`wontfix` → `closed`, `inactive` → `wontfix`)
- Discard (rare — log to a sidecar file, drop the row)
- Halt (rare — the value is meaningful; redesign target schema first)

---

## 4. Per-project migration — sequential order

List your targets in dependency order, e.g.:
1. The target tool's own repo (dogfood first)
2. Toolchain repos with rich history
3. User projects
4. Workspace root

Steps per project (run inside the target repo, with the freshly built target binary):

| Step | Command | Notes |
|------|---------|-------|
| 4.1 | `<target> import <old> <db>` | Dry-run; prints row counts + skipped tables |
| 4.2 | Compare reported counts to `reports/before.csv` | Manual diff; abort if drift |
| 4.3 | `<target> import <old> --apply` | Atomic: writes `<db>.<old>-bak`, replaces in place |
| 4.4 | `<target> doctor` | Expect `<target_schema_kind>` |
| 4.5 | `<target> status --json` + plan/feedback list | Spot-check preservation |
| 4.6 | `<target> plan item show <id>` on one in-progress item | Confirm acceptance + filescope survive |
| 4.7 | `<target> hub sync` | Re-populate metadata for this project |
| 4.8 | `<target> resume` | Surfaces the right in-flight item |
| 4.9 | For each populated source-only table from §3.4, export JSONL to `.devdb/archive-source-only/<table>.jsonl` | Preserve what the importer drops — do this BEFORE removing `<db>.<old>-bak` |

**Acceptance gate for a project to be marked migrated:**
- `<target> doctor` clean.
- Counts match `reports/before.csv` within `<parity-tolerance>`.
- Hub dashboard lists the project post-`hub sync`.
- One in-progress plan item, if any, round-trips through plan-show with intact acceptance criteria.

---

## 5. Workspace cutover (single-day switch, hard cut)

Triggered once 2-3 lead projects clear the gate. Steps run in one continuous session:

| # | Task |
|---|------|
| 5.1 | Install target CLI to a stable path (e.g., `go install`, `pip install --upgrade`, `npm i -g`) |
| 5.2 | Replace shell wrapper so it execs the target binary directly |
| 5.3 | Verify `which <tool>` resolves to the target and `<tool> --help` confirms identity |
| 5.4 | For each remaining project, run the §4 gate |
| 5.5 | `<target> hub sync` from every project root — one batch |
| 5.6 | **Sweep + rewrite all old-verb callsites** from §3.5 per the mapping table |
| 5.7 | Repoint the symlinked agent skill to the new source |
| 5.8 | Run the session-start ritual in all projects back-to-back as a smoke test |

---

## 6. Verification

| Check | Command | Pass condition |
|-------|---------|---------------|
| Binary identity | `<tool> --help \| head -1` | Mentions new tool |
| Parity tests | `<test>` | All green (note env-dependent skips) |
| Per-project doctor | `<tool> doctor` in each | `<target_schema_kind>`, no missing tables |
| Hub consistency | `<tool> hub dashboard --view summary` | Each project listed with fresh sync timestamp |
| Audit spot-check | Open one in-progress plan item per project | Acceptance + filescope match pre-migration snapshot |
| Callsite sweep | `grep -REn -- "<old-verb-regex>"` | Zero hits outside the target's own docs/ |

**Optional but recommended:** `<target> verify record "importer functional tests" --scope internal/importer --git-sha $(git rev-parse HEAD) --status passed --exit-code 0 --finished` so future agents can reuse this verification.

---

## 7. Decommission (same day, no stability window)

Hard cut means there's no parallel source path to fall back to. Decommission immediately:

| # | Task |
|---|------|
| 7.1 | Archive (do not delete) the source checkout: `mv <source> <archive-dir>/<source>-$(date +%F)` |
| 7.2 | Drop every cron entry that called source verbs (audited from §3.5) |
| 7.3 | Final AGENTS.md / workspace-config sweep (already done in §5.6, this is a read-through) |
| 7.4 | Schedule cleanup: `0 9 <DAY+90> * find <archive-dir> -name '*.pre-<target>-*' -o -name '*.<old>-bak' \| xargs trash` |

---

## 8. Verb mapping (for §5.6 sweep)

Cheat sheet for the callsite rewrite. Cite your target tool's parity doc for the full table.

| Source verb | Target command |
|-------------|----------------|
| `<verb-noun>` | `<noun verb>` |
| `<add-thing>` | `<thing add>` |
| ... | ... |

Not ported (intentional): list the verbs your target skipped.

---

## 9. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Importer silently drops a populated source-only table | Med | Med | §3.4 inventories; §4.9 archives before backup removal |
| Status enum tightening changes user-visible state | Med | Low | Documented in target's enum-mapping doc. Diff before/after for one project |
| Hub rebuild loses attention items or sync history | Med | Low | §3.9 backup; hub sync repopulates from per-project DBs |
| Wrapper script / AGENTS.md hides old grammar after cutover | High | Med | §3.5 enumerates; §5.6 sweeps; §6 grep gate confirms zero hits |
| Plan item in flight depends on a verb that was `not ported` | Med | Med | Rescope those plan items before migration |
| Target toolchain missing or version too old | Low | Blocker | §3.6 confirms; fall back to `go install <path>@latest` |
| Cron entry runs old wrapper after cutover | Med | Med | §3.5 + §7.2 audit |
| Backup retention mis-managed under hard cut | Low | High | Tar backups into one file (§3.2); cron cleanup (§7.4) |

---

## 10. Rollback

For any single project: `mv .devdb/<db>.<old>-bak .devdb/<db>` and reinstall the previous wrapper.
Workspace-wide: repoint the shell wrapper to the archived source checkout and re-symlink the old
skill. Backup retention: **90 days post-cutover**, after which all backups are removed in one batch
via `trash`.

---

## 11. Decision log

| # | Decision | Choice |
|---|----------|--------|
| 1 | Plan file path | `<path>/MIGRATION_PLAN.md` |
| 2 | Scope | All N ledgers |
| 3 | Transition style | Hard cut / soft cut / shim |
| 4 | Project order | <list> |
| 5 | Backup retention | 90 days |

---

## Appendix: lessons learned (lifted from 2026-06-19 python→go migration)

Apply these to your plan:
- **§3.1**: search `$HOME` not just `$WORKSPACE` for inventory.
- **§3.2**: tar per-project backups into one file (single `trash` on day 91).
- **§3.10**: add a source-value audit step before §4 — query every status field for values outside the target enum. Skip migration of any project that has unresolvable values, or normalize them first.
- **§4.9**: archive source-only tables to JSONL before removing `.bak`.
- **§5.2**: prefer a symlink over a wrapper script for the cutover.
- **§6**: record the importer functional-test pass as a verification entry.
- **§7.4**: schedule cleanup with a single-shot cron.
- Consider splitting `<MIGRATION_PLAN>.md` (frozen plan) + `<MIGRATION_REPORT>.md` (execution log) for future migrations.