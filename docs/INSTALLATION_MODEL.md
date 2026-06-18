# Devdb Installation Model

Devdb has two separate concerns:

- **Code**: the Go CLI (`devdb-go` repo).
- **Data**: one SQLite ledger per target repository at `.devdb/development.db`.

Do not symlink databases. Install the CLI globally; each project keeps its own local `.devdb/development.db`.

## Default behavior

From a repository root:

```bash
cd /path/to/project
devdb init
devdb report
```

Creates and uses `/path/to/project/.devdb/development.db`.

## Machine setup

```bash
go install github.com/sebihoermann/devdb-go/cmd/devdb@latest
```

Or from a checkout:

```bash
git clone https://github.com/sebihoermann/devdb-go
cd devdb-go
go install ./cmd/devdb
```

Install the agent skill as a symlink:

```bash
ln -sfn /path/to/devdb-go/skills/devdb ~/.codex/skills/devdb
```

## New repository setup

```bash
cd /path/to/project
devdb init
devdb report
```

Keep `.devdb/` gitignored and repo-local.

## Wrapper pattern

Pin `--repo` when invoking from scripts:

```bash
#!/usr/bin/env bash
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec devdb --repo "$ROOT" "$@"
```

## What not to symlink

Do not symlink a project's `.devdb/` to the devdb-go tool repo's own `.devdb/`.

```text
Global tool:   devdb binary on PATH
Global skill:  devdb-go/skills/devdb/
Project data:  /path/to/project/.devdb/development.db
```

## Migrate from Python

One-time import from a Python-era ledger:

```bash
devdb import python-db --apply
```

Legacy Python sources: [sebihoermann/devdb](https://github.com/sebihoermann/devdb) `archive/python/`.
