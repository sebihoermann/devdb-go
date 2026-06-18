# Metadata sync reliability

This document describes how devdb keeps the cross-project metadata hub current without relying on a single brittle background process.

## Reliability model

The hub is treated as a derived cache, not the source of truth.

### 1) Write-through on local changes

Every local write:

- increments the source DB metadata version
- marks the source as dirty
- records the last local write time
- keeps the dirty state persisted in `sync_state`

That means a write is never “invisible” to the source DB even if the hub is offline.

### 2) Opportunistic push after successful writes

After a successful write command, devdb tries to refresh the hub row for the active project.

This is best-effort:

- it improves freshness when the user is already active in a project
- it never becomes the only sync mechanism
- failures are recorded so they can be diagnosed later

### 3) Periodic reconciliation

`devdb hub-sync` refreshes registered projects from the hub side.

It can run:

- once, as a normal command
- in watch mode with `--watch`
- with bounded iterations via `--iterations`
- with a configurable delay via `--interval`

This makes it portable:

- on Linux, you can run it under `systemd`
- on macOS, you can run it under `launchd`
- anywhere, you can run it from `cron` or another scheduler

The core logic stays the same in every environment.

### 4) Repair on read

Hub-facing commands attempt to refresh dirty source projects before rendering:

- `hub-dashboard`
- `hub-status`
- `hub-quality`
- `hub-project`
- `list-projects` when the hub exists

That means stale data self-heals when the user asks for it.

### 5) Diagnosis

`devdb doctor-sync` explains the current state of each registered project and recommends the next command.

It is designed for questions like:

- Is the source DB missing?
- Is the source dirty?
- Is the hub behind?
- Is there a recorded push error?

## Practical recommendation

Use the layered approach rather than a single daemon:

1. **Always keep write-through and dirty state.** This is the correctness foundation.
2. **Run `hub-sync` periodically.** This keeps the hub fresh even when nobody opens a project.
3. **Let read-time repair clean up stragglers.** This reduces user-visible staleness.
4. **Use `doctor-sync` when something looks wrong.** It tells you whether the issue is missing data, stale data, or a push error.

A daemon is optional, not required. It can be useful as an accelerator, but the system should remain correct without it.

## Suggested service patterns

Ready-to-drop examples live under `docs/examples/`:

- `docs/examples/devdb-hub-watch.systemd.service`
- `docs/examples/devdb-hub-watch.launchd.plist`
- `docs/examples/devdb-hub-sync.cron`

### systemd

Run the reconciler as a simple oneshot timer or a long-lived watch loop:

```ini
[Service]
Type=simple
WorkingDirectory=/path/to/repo
ExecStart=/usr/local/bin/devdb hub-sync --watch --interval 60
```

### launchd

Use the same CLI command from a LaunchAgent or LaunchDaemon.

### cron

For a periodic pull model, run a one-shot sync every few minutes:

```cron
*/5 * * * * cd /path/to/repo && devdb hub-sync --json >> .devdb/hub-sync.log 2>&1
```

## Operational notes

- If a source DB disappears, `doctor-sync` should report it as missing.
- If a sync fails, the last error stays recorded in the source DB state.
- If a write succeeds locally but hub refresh fails, the project is still correct; only the hub cache is stale.
- If the hub is stale but the source is fresh, running `hub-sync` should reconcile them.

## Summary

The design goal is: **correctness first, freshness second, daemon third**.

That gives you a sync system that works on Linux with a background watcher, but also remains portable on any platform where only scheduled commands are available.
