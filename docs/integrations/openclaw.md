# OpenClaw memory adapter

`devdb-openclaw` is an optional first-party adapter between an OpenClaw
workspace and a devdb project ledger. It discovers OpenClaw memory files,
indexes them as source-backed architecture notes, and turns explicit friction
markers into deduplicated feedback.

## Architecture boundary

The adapter does not change devdb's storage model. Each target project keeps
its own `.devdb/development.db` SQLite file, and Markdown remains the canonical
store for OpenClaw memory. Provider integrations neither replace SQLite nor
require remote database synchronization. Multi-machine transport, if ever
needed, belongs above the local database and must preserve synchronous local
writes.

OpenClaw policy stays outside `cmd/devdb`: workspace filenames, daily-note
patterns, and friction markers live under
`internal/integrations/openclaw` and are exposed by the separately installed
`cmd/devdb-openclaw` binary. The adapter reuses devdb domain services and never
writes ledger tables directly.

## Memory references

`plan_items.memory_ref` is an opaque string in core devdb. The OpenClaw adapter
uses this provider-specific format:

```text
openclaw:<workspace-relative-path>[#<fragment>]
```

Examples:

```text
openclaw:MEMORY.md#release-process
openclaw:memory/2026-06-22.md#adapter-decision
openclaw:memory/2026-06-22.md#L42
```

Paths use forward slashes, must be relative to the configured workspace, and
must not contain `..` traversal. A fragment either names a Markdown heading
after lowercase slug normalization or uses `L<number>` for a one-based source
line. Consumers may preserve unknown fragments; core devdb treats the entire
reference as opaque.

## Supported memory files

Discovery is deliberately conservative:

- bootstrap and curated files: `AGENTS.md`, `SOUL.md`, `USER.md`,
  `IDENTITY.md`, `TOOLS.md`, `HEARTBEAT.md`, `BOOT.md`, `BOOTSTRAP.md`, and
  `MEMORY.md`;
- daily notes matching `memory/YYYY-MM-DD.md`.

The adapter never follows a discovered path outside the workspace root.
Missing optional files are reported by `list` but do not make discovery fail.

## Command contract

The adapter has explicit workspace and target-ledger inputs:

```bash
devdb-openclaw --workspace ~/.openclaw/workspace --repo ~/.openclaw/workspace list
devdb-openclaw --workspace ~/.openclaw/workspace --repo ~/.openclaw/workspace links --plan my-plan
devdb-openclaw --workspace ~/.openclaw/workspace --repo ~/.openclaw/workspace sync
devdb-openclaw --workspace ~/.openclaw/workspace --repo ~/.openclaw/workspace friction scan
devdb-openclaw --workspace ~/.openclaw/workspace --repo ~/.openclaw/workspace doctor
```

The verb list is exactly `version`, `list`, `links`, `sync`, `friction scan`, and
`doctor`. There is no `schedule` verb — it was removed because it wrote two daily
cron entries that shelled out to a binary nobody had installed, so running it
created two silently failing jobs every day. Scheduling is the operator's job:
run `sync` and `friction scan` from your own cron or systemd timer.

`--workspace` defaults to `$OPENCLAW_WORKSPACE`, then
`~/.openclaw/workspace`. `--repo` defaults to
`$OPENCLAW_DEVDB_TARGET_REPO`, then the workspace. `--json` provides stable
machine-readable output. Write commands return non-zero when the target ledger
is unavailable; they do not silently select an ancestor ledger.

No adapter command mutates scheduler configuration.

`links --plan <slug-or-id>` joins plan items to their `memory_ref` values and
reports whether the referenced workspace file currently exists. It accepts the
canonical `openclaw:` format and legacy workspace-relative references.

`friction scan` treats `--marker` values as case-insensitive literal text.
Each hit receives a stable identity derived from its workspace-relative path
and normalized line content, so inserting lines above a marker does not create
a duplicate. Editing a marked line creates a new feedback row; removing a
marker leaves its existing feedback untouched for explicit human or agent
closure. The scan never rewrites unrelated feedback.

## Install and rollback

Install the adapter separately from core devdb:

```bash
go install github.com/sebihoermann/devdb-go/cmd/devdb-openclaw@latest
```

Ensure `$(go env GOPATH)/bin` is on `PATH` before running the adapter from
cron or a systemd timer.

Existing shell integrations can migrate one command at a time. First compare
`list`, `sync`, and `friction scan` output against the scripts with scheduler
changes disabled. Keep existing cron entries until repeated adapter runs prove
idempotent, then point them at the adapter and disable the old scripts.

Rollback is removing the binary and restoring the previous shell scripts. The
adapter does not rewrite memory Markdown, and its devdb writes use ordinary
architecture-note and feedback records, so removing the binary does not make
the ledger unreadable.

## Update-survival checks

After upgrading OpenClaw or devdb:

1. Run `devdb-openclaw doctor`.
2. Compare `list --json` against the expected workspace fixture.
3. Run `sync` twice and confirm the second run creates no architecture notes.
4. Run `friction scan` twice and confirm the second run creates no feedback.

No adapter command imports OpenClaw internals. Compatibility is limited to the
documented workspace file contract and the public `openclaw` CLI.
