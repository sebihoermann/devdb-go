# devdb-go

[![CI](https://github.com/sebihoermann/devdb-go/actions/workflows/ci.yml/badge.svg)](https://github.com/sebihoermann/devdb-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Queryable per-project memory for humans and AI agents working on a codebase together.

Each project keeps one SQLite file at `.devdb/development.db`. The `devdb` CLI is the write and read surface — goals, plans, feedback, architecture notes, review findings, and verification state survive sessions and model swaps.

Provider integrations are optional adapters over that file-backed core. They do
not replace SQLite or require a remote synchronization service. See
[OpenClaw integration](docs/integrations/openclaw.md) for the first adapter
contract.

The legacy Python prototype lives in [sebihoermann/devdb](https://github.com/sebihoermann/devdb) under `archive/python/`.

## Install

```bash
go install github.com/sebihoermann/devdb-go/cmd/devdb@latest
```

Ensure `$(go env GOPATH)/bin` is on your `PATH`.

From a checkout:

```bash
git clone https://github.com/sebihoermann/devdb-go
cd devdb-go
go install ./cmd/devdb
```

Install the optional OpenClaw memory adapter separately:

```bash
go install github.com/sebihoermann/devdb-go/cmd/devdb-openclaw@latest
```

## Quick start

```bash
cd /path/to/your-project
devdb init
devdb status
devdb report
devdb feedback add --role user "auth flow is confusing" --severity med
```

## Command shape

```text
devdb [--repo PATH] [--db PATH] [--json] [--all] [--verbose] <noun> <verb> [flags]
```

Top-level reads: `init`, `status`, `quality`, `report`, `resume`, `doctor`.

See [docs/go-native-parity-matrix.md](docs/go-native-parity-matrix.md) for the full command vocabulary and Python → Go mapping.

## Migrate from Python ledger

If your project still has a Python-created `.devdb/development.db`:

```bash
devdb import python-db .devdb/development.db          # inspect
devdb import python-db --apply                        # in-place; backs up + auto-archives python-only tables
devdb import python-db --apply --no-archive-python-only  # skip auto-archive (e.g., already archived manually)
devdb import python-db --apply --force                # override .python-bak-already-migrated guard
```

On `--apply`, populated python-only tables (`entity_links`, `loc_snapshots`, etc.)
are JSONL-archived to `.devdb/archive-python-only/<table>.jsonl` before the
python DB is moved aside as `development.db.python-bak`. Pass
`--no-archive-python-only` to opt out, `--force` to re-apply over a stale `.python-bak`.

See [docs/go-native-schema-importer-mapping.md](docs/go-native-schema-importer-mapping.md) for table mapping details.

## Hub register/unregister

```bash
devdb hub register /path/to/repo --alias myapp        # register one project
devdb hub register --auto --scope /path/to/parent    # walk + register every .devdb/development.db
devdb hub unregister myapp                            # remove a project from the hub
```

`hub register --auto` skips `.git`, `node_modules`, and `vendor` directories.

## Agent skill

`skills/devdb/SKILL.md` is the agent playbook. Symlink it into your agent skills directory:

```bash
ln -sfn /path/to/devdb-go/skills/devdb ~/.codex/skills/devdb
```

## Layout

```text
cmd/devdb/     — CLI entrypoint
cmd/devdb-openclaw/ — optional OpenClaw memory adapter
internal/      — domain services, storage, migrations
skills/devdb/  — agent skill (SKILL.md, SPEC.md)
docs/          — parity matrix, installation model, hub sync notes
```

## Development

```bash
go test ./...
go build -o devdb ./cmd/devdb
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
