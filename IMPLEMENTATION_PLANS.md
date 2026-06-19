# devdb-go Improvement Implementation Plans

> Concrete plans for the 7 improvement entries filed in `devdb-go/.devdb/development.db` as
> `feedback … --category idea` on 2026-06-19. Ordered by recommended execution sequence.
> Each plan includes: scope, files to touch, steps, verification, and effort estimate.
>
> Sources:
> - `feedback 419df7ea5c17a5de75046ebc9c20133a` — importer status-enum fixture gap
> - `feedback 9cecd97e24fe744d2361197e0489b955` — skip chmod-as-root tests
> - `feedback 5377d2a21a1277079b291f2963e7aa66` — hub unregister verb
> - `feedback d419a47ef110ad90f515905184a5f50e` — `--repo-root` alias
> - `feedback f112be07bb9c7187291ee866daa76efb` — auto-archive python-only tables
> - `feedback 23311a6c0da2599b14cdaae5345316be` — idempotent `--apply`
> - `feedback 76331e8c3be354b97b5dcfce578dc32b` — hub register `--auto`

---

## Recommended execution order

The first two plans unblock clean test runs, so do them before any feature work.

| Order | Plan | Effort | Depends on |
|------:|------|--------|------------|
| 1 | IMP-2 — Skip chmod-as-root tests | 5 min | — |
| 2 | IMP-1 — Importer status-enum fixture gap | 15 min | IMP-2 (clean test output) |
| 3 | IMP-5 — Auto-archive python-only tables | 30 min | — |
| 4 | IMP-6 — Idempotent `--apply` | 15 min | — |
| 5 | IMP-3 — `hub unregister` verb | 20 min | — |
| 6 | IMP-4 — `--repo-root` alias | 5 min | — |
| 7 | IMP-7 — `hub register --auto` | 30 min | IMP-3 (consistent CLI surface) |

**Total estimated effort:** ~2 hours of focused work, all independent enough to land in any order.

---

## IMP-1 — Importer status-enum fixture gap

**Feedback id:** `419df7ea5c17a5de75046ebc9c20133a`
**Severity:** med
**Why:** `python_ledger_full_test.go` inserts zero feedback rows, so the importer's
CASE-WHEN status mapping for `feedback.status='resolved'` (which the original importer
preserved unchanged → CHECK constraint crash) had no fixture coverage. A future regression
of the same shape would not be caught.

### Files to touch

- `internal/importer/python_ledger_full_test.go` — add 3 INSERT statements to the
  `extra` slice and 1 statement to the `setup` block
- `internal/importer/python_ledger_fixture_test.go` — add 1 INSERT to the `stmts` slice
  to lock in the small-fixture behavior

### Steps

1. In `python_ledger_full_test.go` find the `extra := []string{` slice. Add these three
   statements (use the same `ts` placeholder the existing rows use):

   ```sql
   `INSERT INTO feedback(id, role, note, status, created_at, model_id)
       VALUES ('fb_resolved', 'model', 'resolved-row', 'resolved', '` + ts + `', 'test')`,
   `INSERT INTO feedback(id, role, note, status, created_at, model_id)
       VALUES ('fb_deferred', 'model', 'deferred-row', 'deferred', '` + ts + `', 'test')`,
   `INSERT INTO plan_item_acceptance(id, plan_item_id, ordinal, criterion, status, evidence, created_at, updated_at, model_id)
       VALUES ('acc_wontfix', 'pi1', 1, 'criterion', 'wontfix', 'note', '` + ts + `', '` + ts + `', 'test')`,
   ```

2. Update the `TestImportFullLegacyFixture` expectations to reflect the new row counts:
   `feedback` from 0 → 2, `plan_item_acceptance` from 0 → 1.

3. In `python_ledger_fixture_test.go` (the small fixture) add one INSERT to the `stmts`
   slice with `status='resolved'` so the small-fixture test also covers the case.

4. Run the suite: `go test -count=1 ./internal/importer/` — all should pass.

5. **Regression demonstration (optional but recommended):** Temporarily revert just the
   `feedback` CASE-WHEN in `python_ledger.go` to the broken form (`status` passed through
   when in IN list, including `'resolved'`), run `TestImportFullLegacyFixture`, confirm
   it now fails with the CHECK constraint error, then restore the patched version. This
   demonstrates the test catches the regression.

### Verification

- `go test -count=1 ./internal/importer/` — all pass (no chmod-as-root failures once IMP-2 lands).
- `TestImportFullLegacyFixture` references `fb_resolved`, `fb_deferred`, and `acc_wontfix`
  in its expectations and asserts correct row counts post-import.

---

## IMP-2 — Skip chmod-as-root tests under `euid=0`

**Feedback id:** `9cecd97e24fe744d2361197e0489b955`
**Severity:** med
**Why:** Seven tests fail when run as root because they `chmod` temp dirs to `0o500`/`0o555`
expecting writes to be blocked; root bypasses the restriction. CI passes (non-root); local
dev shows red. Adds noise to `go test ./...`.

### Files to touch

Seven test functions, each in a different file:

- `internal/app/app_test.go` — `TestInitDBMkdirFailure`, `TestOpenStatPermissionError`
- `internal/importer/coverage_boost_test.go` — `TestImportPythonDBMkdirFailure`
- `internal/importer/python_ledger_errors_test.go` — `TestApplyInPlaceImportFailureOnReadOnlyDir`
- `internal/domain/hub/service_test.go` — `TestOpenHubRunHubFailureOnReadonlyFile`
- `internal/migrate/error_paths_test.go` — `TestRunHubCommitFailure`
- `internal/migrate/runner_test.go` — `TestRunAllFailsOnReadonlyDatabase`

### Steps

1. Add a tiny helper at the top of each test file (or extract to a shared testutil):

   ```go
   func skipIfRoot(t *testing.T, reason string) {
       if os.Geteuid() == 0 {
           t.Skipf("skipping under root: %s", reason)
       }
   }
   ```

   Or inline at the top of each function:

   ```go
   if os.Geteuid() == 0 {
       t.Skip("chmod-as-user semantics don't apply under root")
   }
   ```

2. Place the check as the first statement inside each of the seven test functions.

3. Run `go test -count=1 ./...` — expect all green under root.

4. Run under a non-root user (CI matrix, or `sudo -u nobody`) — expect the same seven
   tests to actually run and pass (they exercise real chmod semantics there).

### Verification

- `go test -count=1 ./...` under root: 0 FAILs.
- `go test -count=1 ./...` under non-root: same 7 tests run, all pass.
- `go test -v -run TestInitDBMkdirFailure` shows `--- SKIP` under root, `--- PASS` under
  non-root.

---

## IMP-3 — `hub unregister` verb

**Feedback id:** `5377d2a21a1277079b291f2963e7aa66`
**Severity:** low
**Why:** When a project's repo is archived or deleted, there's no clean way to remove its
hub alias. Currently `hub register` updates an existing alias in place, which is undocumented
and surprising.

### Files to touch

- `internal/cli/hub/` (whatever file holds the `register` subcommand) — add new subcommand
- `internal/domain/hub/service.go` — add `Unregister(alias string) error` method
- `internal/domain/hub/service_test.go` — add unit tests

### Steps

1. In `service.go`, add:

   ```go
   func (s *Service) Unregister(alias string) error {
       // 1. Look up the alias in the registry
       // 2. Remove the project row from the hub metadata DB
       // 3. Return ErrAliasNotFound if alias doesn't exist
   }
   ```

2. In the CLI file, register the new subcommand:

   ```go
   hubCmd.AddCommand(newUnregisterCmd(svc))
   ```

   The command:
   - positional: `<alias>` (required)
   - optional: `--json` for machine output
   - emits `{"alias": "...", "removed": true}` to stdout on success

3. Tests in `service_test.go`:
   - `TestUnregisterRemovesAlias`: register two aliases, unregister one, verify `List`
     shows only the other.
   - `TestUnregisterUnknownAlias`: returns `ErrAliasNotFound` or equivalent.
   - `TestUnregisterIsIdempotent` (optional): running twice is a no-op the second time
     (returns error or just no-ops — pick one and document it).

### Verification

- `devdb hub unregister devdb-go` removes the alias from `hub list` and from `~/.devdb-projects`.
- `devdb hub unregister nonexistent` exits non-zero with a clear error.
- `devdb hub unregister devdb-go --json` emits the JSON shape.

---

## IMP-4 — `--repo-root` alias for `--repo`

**Feedback id:** `d419a47ef110ad90f515905184a5f50e`
**Severity:** low
**Why:** Python devdb used `--repo-root`; Go devdb uses `--repo`. Hard-cutover means muscle
memory still types `--repo-root`, which silently errors with "unknown flag".

### Files to touch

- The root command file in `cmd/devdb/main.go` or wherever the `--repo` flag is registered
- Possibly one of the internal/cli files if the global flag is defined there

### Steps

1. Find the `--repo` flag definition. It's a `string` flag attached to the root cobra command.
   Add an alias:

   ```go
   rootCmd.PersistentFlags().StringVar(&repoPath, "repo", "", "repository root (alias: --repo-root)")
   rootCmd.PersistentFlags().StringVar(&repoPath, "repo-root", "", "alias for --repo (deprecated)")
   ```

   The trick: cobra allows registering the same destination variable under two flag names.

2. Alternative (cleaner): use cobra's `Aliases` mechanism if it supports flag aliases (it
   doesn't, so the var-binding trick is the standard approach).

3. Tests:
   - `devdb --repo /tmp/foo doctor` works.
   - `devdb --repo-root /tmp/foo doctor` works.
   - Both error if both flags are given to different values.

### Verification

- `devdb --repo-root /tmp/foo doctor` no longer errors with "unknown flag".
- `--repo` and `--repo-root` are documented as interchangeable in `--help`.
- `devdb --repo /a --repo-root /b` errors with a clear "conflicting repo flags" message.

---

## IMP-5 — Auto-archive python-only tables during `import python-db --apply`

**Feedback id:** `f112be07bb9c7187291ee866daa76efb`
**Severity:** med
**Why:** The plan required manually JSONL-archiving populated python-only tables (e.g.
`loc_snapshots=6541` in storyblender) from `.python-bak` before deleting it. Fragile —
the migration-author has to remember. Bake it into the importer.

### Files to touch

- `internal/importer/python_ledger.go` — extend `ImportPythonDB` to auto-archive
- `internal/cli/importer/python_db.go` (or wherever the `--apply` flag is wired) — add
  `--no-archive-python-only` opt-out flag

### Steps

1. In `python_ledger.go`, define the python-only table list (already exists as
  `pythonOnlyTables` per the importer source — verify).

2. In `ImportPythonDB`, after `copyLegacyData` succeeds and only when `--apply` is true,
   iterate the python-only tables: for each, query the source DB's row count. If non-zero,
   write `.devdb/archive-python-only/<table>.jsonl` using the same column-discovery +
   JSONL serialization pattern from step §4.9 of the migration plan.

3. Add a new flag `--no-archive-python-only` on the CLI subcommand that, when true,
   skips step 2. Default is `false` (i.e., default-on).

4. Refactor: extract the JSONL writer from the migration plan's `python3 -c` snippet into
   a Go helper `internal/importer/archive.go` so the Python tooling and Go tooling use
   the same shape.

5. Tests in `internal/importer/python_ledger_archive_test.go` (new file):
   - `TestApplyArchivesPythonOnlyTables`: create a fixture with 5 `entity_links` rows,
     run `ImportPythonDB` with `--apply`, assert
     `.devdb/archive-python-only/entity_links.jsonl` exists with 5 rows.
   - `TestApplyWithNoArchiveFlagSkipsArchive`: same setup, with `--no-archive-python-only`,
     assert no archive file created.
   - `TestApplyWithEmptyPythonOnlyTables`: fixture with zero python-only rows, assert
     no archive directory created.

### Verification

- `devdb import python-db --apply` against a fixture with `loc_snapshots=10` writes
  `.devdb/archive-python-only/loc_snapshots.jsonl` with 10 rows.
- `devdb import python-db --apply --no-archive-python-only` against the same fixture
  skips the archive.
- Existing test `TestApplyInPlaceImportSuccess` (or equivalent) still passes.

---

## IMP-6 — Make `import python-db --apply` idempotent

**Feedback id:** `23311a6c0da2599b14cdaae5345316be`
**Severity:** low
**Why:** If `--apply` is run twice, the second run overwrites `.python-bak` with already-Go
data, destroying the rollback path. Detect and reject unless `--force` is passed.

### Files to touch

- `internal/importer/python_ledger.go` — early-exit check
- `internal/cli/importer/python_db.go` — add `--force` flag

### Steps

1. In `ImportPythonDB`, before any work, check if `<db>.python-bak` exists and what schema
   it has:

   ```go
   bakPath := dstPath + ".python-bak"
   if _, err := os.Stat(bakPath); err == nil {
       info, err := inspectPythonDB(bakPath)
       if err == nil && info.Kind == SchemaGo {
           return ImportResult{}, ErrPythonBakAlreadyMigrated
       }
   }
   ```

2. Add `ErrPythonBakAlreadyMigrated` as a sentinel error.

3. On the CLI side, accept `--force` and, when set, ignore the sentinel error (overwrite
   anyway — useful for re-running after the user has decided they don't care about rollback).

4. Tests in `python_ledger_idempotency_test.go` (new file):
   - `TestApplyTwiceRejectsSecond`: run `--apply` once successfully, attempt `--apply`
     again, expect `ErrPythonBakAlreadyMigrated`.
   - `TestApplyTwiceWithForceSucceeds`: same as above but with `--force`, second succeeds.
   - `TestApplyWithIntactPythonBakRejects`: pre-place a fresh python source as
     `.python-bak`, run `--apply`, expect rejection.

### Verification

- Running `devdb import python-db PATH --apply` twice in a row — second errors with a
  clear message.
- Running with `--apply --force` succeeds and overwrites `.python-bak`.
- The error message names `.python-bak` and tells the user about `--force`.

---

## IMP-7 — `hub register --auto`

**Feedback id:** `76331e8c3be354b97b5dcfce578dc32b`
**Severity:** low
**Why:** Migration registered 8 projects via a manual loop. For larger fleets, a flag
that walks the parent dir and registers every `.devdb/development.db` would scale.

### Files to touch

- `internal/cli/hub/` (the file with `registerCmd`) — add `--auto` flag
- `internal/domain/hub/service.go` — add `AutoRegister(scope string) ([]string, error)`

### Steps

1. Implement the walker:

   ```go
   func (s *Service) AutoRegister(scope string) ([]string, error) {
       root := scope
       if root == "" { root = "." }
       var registered []string
       err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
           if err != nil { return err }
           if d.Name() != "development.db" { return nil }
           if filepath.Base(filepath.Dir(path)) != ".devdb" { return nil }
           repoRoot := filepath.Dir(filepath.Dir(path))
           alias := filepath.Base(repoRoot)
           if err := s.Register(repoRoot, alias); err == nil {
               registered = append(registered, alias)
           }
           return nil
       })
       return registered, err
   }
   ```

2. On the CLI:

   ```go
   registerCmd.Flags().Bool("auto", false, "walk scope and register every .devdb/development.db")
   registerCmd.Flags().String("scope", ".", "scope directory for --auto (default: cwd)")
   ```

3. Tests:
   - `TestAutoRegisterSingleProject`: create a temp dir tree with one `.devdb/development.db`,
     run `AutoRegister`, verify one alias registered with the dir basename.
   - `TestAutoRegisterMultipleProjects`: tree with three, verify three aliases.
   - `TestAutoRegisterIgnoresNonDevdb`: tree with one `.devdb/` plus an unrelated
     `database.sqlite`, verify only one alias.
   - `TestAutoRegisterSkipsNestedDevdb`: ensure it doesn't double-register nested dirs.

### Verification

- `devdb hub register --auto --scope /tmp/empty` registers nothing (empty tree) and
  reports zero registrations.
- `devdb hub register --auto --scope /root/.openclaw/workspace` registers the same 8
  projects as the migration loop did (idempotent on already-registered).
- `devdb hub list` after `--auto` shows all detected projects.

---

## Cross-cutting considerations

### Test isolation

Each plan adds at least one new test file or modifies existing ones. Be careful with
shared fixtures in `internal/testutil/` — don't break other tests' setup.

### Documentation

After IMP-1, IMP-2, IMP-3, IMP-4, IMP-5, IMP-6, IMP-7 land, update:
- `README.md` — flag list and CLI surface
- `docs/go-native-parity-matrix.md` — remove the "remove" classification for any verb
  that becomes available (none in this set)
- `skills/devdb/SKILL.md` — update with new verbs

### Verification ledger

After each plan lands, run `go test -count=1 ./...` once and record the pass:

```bash
devdb --repo /root/.openclaw/workspace/projects/devdb-go verify record \
  "go test -count=1 ./..." \
  --scope ./... \
  --git-sha $(git rev-parse HEAD) \
  --status passed \
  --exit-code 0 \
  --finished
```

This makes future changes queryable for "did the test suite pass after the last known change?"

### Closing feedback entries

When each plan ships, close the corresponding feedback row in the devdb-go ledger:

```bash
devdb --repo /root/.openclaw/workspace/projects/devdb-go feedback close <id> \
  --proposed-fix "shipped in commit <sha>"
```