# devdb-go

Go-native devdb CLI. Dogfoods itself — development state lives in `.devdb/development.db`.

## Running tests

```bash
go test ./...
go test ./... -run TestName -v
```

## Build & install

```bash
go build -o devdb ./cmd/devdb
go install ./cmd/devdb
```

Migrate a legacy Python ledger: `devdb import python-db --apply`

## Project structure

- `cmd/devdb/` — CLI entrypoint
- `internal/` — domain services, storage, migrations
- `skills/devdb/` — agent skill and documentation
- `.devdb/development.db` — this project's ledger (generated; not committed)

## Dogfooding

Before any task, run `devdb report`. Bracket work with `devdb plan item start` and `devdb plan item pause`.

Read the agent playbook: **[skills/devdb/SKILL.md](skills/devdb/SKILL.md)**
