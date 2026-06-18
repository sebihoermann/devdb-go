# Contributing

## Setup

```bash
git clone https://github.com/sebihoermann/devdb-go
cd devdb-go
go install ./cmd/devdb
devdb --help
```

## Tests

```bash
go test ./...              # full suite
go test ./internal/cli -run TestName -v   # one test, verbose
```

All tests must pass before a PR is merged.

## Adding a migration

Schema migrations live in `internal/migrate/source.go` (project DB) and `internal/migrate/hub.go` (metadata hub).

1. Always append — never modify or reorder existing migrations.
2. Only add columns or tables; no drops, renames, or type changes.
3. Write idempotent SQL where the driver allows it.

Run migrations in tests via `testutil.TempDB(t)`.

## Dogfooding

This repo uses its own `.devdb/development.db` (gitignored). Log finished work — architecture notes, feedback, review findings, plan acceptance — with the CLI.

Read the agent playbook: [skills/devdb/SKILL.md](skills/devdb/SKILL.md)
