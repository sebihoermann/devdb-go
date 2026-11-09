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
devdb ledger export > docs/ledger.md   # prose report for a human administrator
devdb feedback add --role user "auth flow is confusing" --severity med
devdb plan acceptance list --plan my-plan      # criteria for one plan
devdb archive show <id>                        # read an archived row without restoring
```

## Command shape

```text
devdb [--repo PATH] [--db PATH] [--json] [--all] [--verbose] <noun> <verb> [flags]
```

Top-level reads: `init`, `status`, `quality`, `report`, `resume`, `doctor`.

Nouns: `plan` · `feedback` · `goal` · `feature` · `arch` · `review` · `verify` ·
`task` · `approval` · `reminder` · `hub` · `archive` · `analytics` ·
`inventory` · `ledger` · `import` · `list` · `show`

Every noun takes `<verb>`; group commands take a further `<subverb>`
(`plan acceptance meet`). Run `devdb <noun> --help` for the verbs it accepts,
and `devdb help <noun> <verb> --examples` for real command shapes plus known
gotchas.

[docs/COMMANDS.md](docs/COMMANDS.md) is the full command reference — every
verb, its flags and defaults, and its exit codes. It is generated from the
command tree by `go run ./cmd/gen-docs`, so it cannot describe a verb the
binary does not register; CI runs `gen-docs --check` and fails when a new verb
lands without a regeneration.

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
devdb hub register --auto --scope /path/to/parent --dry-run   # report what would register, write nothing
devdb hub unregister myapp                            # remove a project from the hub
devdb hub sync                                        # refresh registered projects from the hub side
devdb hub doctor                                      # explain each project's sync state and the next command
```

`hub register --auto` skips `.git`, `node_modules`, and `vendor` directories.
It is idempotent: an already-registered path is skipped, a derived alias that
collides gets `-2`/`-3`, and an **explicit** alias that already points at a
different root is an error rather than a silent repoint — an alias is an
identity, so unregister the old owner or pick another name.

## Hygiene

```bash
devdb archive gc --dry-run            # report stale open feedback + missing-file findings
devdb archive gc                      # apply them (omit --dry-run to write)
devdb archive run --yes               # move closed/resolved rows into archive_entries
devdb archive show <id>               # read an archived row without restoring it
devdb ledger housekeeping             # archive + reconcile + scan in one command
```

`archive gc` never closes high or critical severity unless you say otherwise:
`--keep-severity` sets what it protects (default `high,critical`) and
`--older-than` sets the age threshold (default 30 days). Its natural mode is
preview, so it needs no `--yes` to run.

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
skills/devdb/  — agent skill (SKILL.md)
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
