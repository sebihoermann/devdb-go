# Go Native Schema and Importer Mapping

One-time migration reference for `devdb import python-db`. Runtime schema is defined in `golang/internal/migrate/source.go` (project DB) and `golang/internal/migrate/hub.go` (metadata hub). Importer logic: `golang/internal/importer/python_ledger.go`.

Snapshot: 2026-06-16.

---

## Schema overview

### Project ledger (`development.db`)

| Migration | Description |
|-----------|-------------|
| v1 | Core tables: goals, feedback, features, plans, milestones, plan_items, acceptance, files, status_log, tasks, reminders, approval_log, repo_files, scan_runs, architecture_notes, review_*, verification_*, missed_cli_calls, archive_entries, sync_state, file_change_events |
| v2 | `commit_archeology` (branch commit import target) |

Go schema uses explicit `CHECK` constraints on status/kind enums. Python allowed wider string sets; importer normalizes where noted below.

### Hub (`metadata.db`)

Rebuilt in Go (`migrate/hub.go`): project registry, snapshots, attention items, sync history. Not populated by `import python-db` — re-sync with `hub sync` after project import.

---

## Table mapping: imported

| Source table | Destination | Transform | Notes |
|--------------|-------------|-----------|-------|
| `goals` | `goals` | `status inactive → wontfix` | Enum tightened in Go |
| `feedback` | `feedback` | `deferred/wontfix → closed` | Go uses `open`/`closed` only |
| `features` | `features` | direct copy | |
| `plans` | `plans` | direct copy | |
| `milestones` | `milestones` | direct copy | |
| `plan_items` | `plan_items` | direct copy | Legacy flat items retain `phase`/`step` |
| `plan_item_acceptance` | `plan_item_acceptance` | direct copy | |
| `plan_item_files` | `plan_item_files` | direct copy | |
| `status_log` | `status_log` | direct copy | |
| `tasks` | `tasks` | direct copy | |
| `reminders` | `reminders` | direct copy | |
| `approval_log` | `approval_log` | direct copy | |
| `scan_runs` | `scan_runs` | direct copy | |
| `repo_files` | `repo_files` | direct copy | |
| `file_change_events` | `file_change_events` | direct copy | |
| `architecture_notes` | `architecture_notes` | direct copy | |
| `review_runs` | `review_runs` | direct copy | |
| `review_findings` | `review_findings` | direct copy | |
| `verification_runs` | `verification_runs` | direct copy | |
| `verification_inputs` | `verification_inputs` | direct copy | |
| `verification_failures` | `verification_failures` | direct copy | |
| `missed_cli_calls` | `missed_cli_calls` | direct copy | |
| `archive_entries` | `archive_entries` | direct copy | |
| `sync_state` | `sync_state` | direct copy | |
| `commit_archeology` | `commit_archeology` | direct copy | If present in source |

All copies use `INSERT OR REPLACE` via attached `legacy` database (`copyLegacyData`).

---

## Table mapping: skipped (Python-only)

| Source table | Rationale |
|--------------|-----------|
| `code_tightening` | Feature removed; use feedback/review instead |
| `loc_snapshots` | LoC history not persisted in Go; `inventory loc` is live |
| `improvement_suggestions` | Merged into `feedback` with `category=idea` |
| `agent_documents` | Experimental; no Go consumer |
| `anchors` | Unused in production workflows |
| `entity_links` | `link-source` not ported |
| `project_links` | Cross-project links not ported |

Skipped tables are reported in `ImportResult.skipped` JSON field.

---

## Import modes

| Mode | Command | Behavior |
|------|---------|----------|
| Copy | `import python-db --source PATH --dest PATH` | New Go DB at `--dest` |
| In-place | `import python-db --apply` | Atomic: backup to `development.db.python-bak`, replace with migrated Go schema |

Guards:

- Source must be Python schema (`storage.SchemaPython`)
- Destination must not be Python schema
- Non-empty Go destination requires `--replace`

---

## Parity verification

`importer.CountParity` compares row counts for: `feedback`, `plan_items`, `goals`, `plans`, `repo_files`, `verification_runs`, `archive_entries`.

Tests in `golang/internal/importer/python_ledger_test.go` assert count preservation on fixture databases.

---

## Status value mapping

| Entity | Python values | Go values | Import rule |
|--------|---------------|-----------|-------------|
| Goal | `active`, `inactive`, `done`, `wontfix` | `active`, `done`, `wontfix` | `inactive → wontfix` |
| Feedback | `open`, `closed`, `deferred`, `wontfix` | `open`, `closed` | non-open non-closed → `closed` |
| Plan item | `planned`, `in_progress`, `done`, `wontfix` | same | direct |
| Acceptance | `open`, `met` | same | direct |

---

## Post-import checklist

1. Run `devdb doctor` — expect `schema_kind: go`
2. Run `devdb status --json` — verify counts match expectations
3. Spot-check one plan item with acceptance: `devdb plan item show <id>`
4. If hub-registered: `devdb hub sync` to refresh metadata
5. Retire Python CLI references in project docs (M14)

---

## Sign-off

| Review item | Result | Date |
|-------------|--------|------|
| Go schema covers all production entities used in dogfood | Approved | 2026-06-16 |
| Skipped tables documented with rationale | Approved | 2026-06-16 |
| Importer transforms preserve auditable history | Approved | 2026-06-16 |
| Parity tests pass on representative fixtures | Approved — `go test ./internal/importer/...` | 2026-06-16 |
| Safe in-place migration path (`--apply` + backup) | Approved | 2026-06-16 |

Approved for cutover planning (M14). Importer may be removed after a successful dogfood window once all active projects have migrated.
