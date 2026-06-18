# devdb-go

[![CI](https://github.com/sebihoermann/devdb-go/actions/workflows/ci.yml/badge.svg)](https://github.com/sebihoermann/devdb-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Queryable per-project memory for humans and AI agents working on a codebase together.

Each project keeps one SQLite file at `.devdb/development.db`. The `devdb` CLI is the write and read surface — goals, plans, feedback, architecture notes, review findings, and verification state survive sessions and model swaps.

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
devdb import python-db --apply                        # in-place (backs up to .devdb/development.db.python-bak)
```

See [docs/go-native-schema-importer-mapping.md](docs/go-native-schema-importer-mapping.md) for table mapping details.

## Agent skill

`skills/devdb/SKILL.md` is the agent playbook. Symlink it into your agent skills directory:

```bash
ln -sfn /path/to/devdb-go/skills/devdb ~/.codex/skills/devdb
```

## Layout

```text
cmd/devdb/     — CLI entrypoint
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
