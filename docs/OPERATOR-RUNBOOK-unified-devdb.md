# One canonical devdb: operator runbook

Written 2026-11-07. Supersedes the two-checkout arrangement described in
`projects/openclaw-devdb-integration/docs/hermes-devdb-bootstrap.md`.

## What this box looks like now

| Thing | Path | Owner / mode |
|---|---|---|
| Canonical checkout | `/srv/devdb/devdb-go` | `root:hermes`, `2775` |
| Canonical ledger | `/srv/devdb/devdb-go/.devdb/development.db` | `root:hermes`, `0664` |
| Hub metadata | `/var/lib/devdb/hub/metadata.db` | `root:hermes`, `0664` |
| Hub registry | `/var/lib/devdb/hub/.devdb-projects` | `root:hermes`, `0664` |
| Shared config | `/etc/devdb/config.json` | `root:root`, `0644` |
| Drift-check allowlist | `/etc/devdb/drift-ignore` | `root:root`, `0644` |
| Binary (system) | `/usr/local/bin/devdb` | `root:root`, `0755` |
| Binary (root's GOPATH) | `/root/go/bin/devdb` | same sha256 |
| Binary (hermes) | `/var/lib/hermes/bin/devdb` | `hermes:hermes`, `0755` |
| Compatibility symlink | `/root/.openclaw/workspace/projects/devdb-go` → `/srv/devdb/devdb-go` | |

Both users resolve the same hub **with no flags and no environment setup**,
because `/etc/devdb/config.json` is the last fallback in the CLI's search
order:

```
explicit flag -> DEVDB_METADATA_DB / DEVDB_REGISTRY
             -> $DEVDB_CONFIG -> $XDG_CONFIG_HOME/devdb/config.json
             -> ~/.config/devdb/config.json -> /etc/devdb/config.json
             -> $HOME default
```

The canonical path is deliberately **not** under `/root`: `/root` is `0700`,
so a database that is perfectly readable is still unreachable when a parent
directory denies traversal. That was the original bug.

## Why it was broken before

`internal/domain/hub/paths.go` resolved the metadata DB and registry from
only a CLI flag, one environment variable, and `$HOME`. Two users on one box
therefore got two different hubs, each with a partial view, and neither
noticed — every individual `devdb status` looked healthy. Tracked as feedback
`11b221cd`.

The repo had also been forked into two checkouts (`/root/.../devdb-go` and
`/var/lib/hermes/projects/devdb-go`) with separate ledgers, which made the
hub show `devdb-go` twice and produce duplicate ids in `hub audit`.

## Daily operation (nothing to do)

`/usr/local/bin/devdb-maintain` runs at 06:00 America/Montevideo and does:

1. `devdb hub sync`
2. `devdb inventory scan`
3. `/usr/local/bin/devdb-drift-check`

The drift check is the safety net. It fails (exit 1) on any of:

- more than one `devdb-go` checkout with its own ledger
- the three installed binaries disagreeing on sha256
- a repo that has `.devdb/` but is missing from the hub registry

Exit 0 prints `RESULT: no drift`. It never auto-fixes: two ledgers is a
decision, not a guess a cron job should make.

### The allowlist

`/etc/devdb/drift-ignore` holds paths that are legitimately unregistered.
**Every line needs a written reason.** Current entries:

- `/var/lib/hermes/projects/devdb-go` — stale second checkout, content fully
  merged into canon (verified: 0 rows unique to it). Kept only because
  hermes's weekly hygiene cron still points at it. **Retire the checkout and
  delete this line.**
- `/var/lib/hermes/projects/research-dashboard-cold-2026-09-17` — archived
  cold copy, superseded.

## Common tasks

**Rebuild after a source change**

```bash
cd /srv/devdb/devdb-go && git pull --ff-only
go build -o /usr/local/bin/devdb ./cmd/devdb
cp /usr/local/bin/devdb /root/go/bin/devdb
cp /usr/local/bin/devdb /var/lib/hermes/bin/devdb
chown hermes:hermes /var/lib/hermes/bin/devdb
sha256sum /usr/local/bin/devdb /root/go/bin/devdb /var/lib/hermes/bin/devdb
/usr/local/bin/devdb-drift-check   # must print RESULT: no drift
```

**Verify both agents agree**

```bash
devdb --repo /srv/devdb/devdb-go feedback list --status all --all --json | <count>
cd /var/lib/hermes && sudo -u hermes devdb --repo /srv/devdb/devdb-go \
  feedback list --status all --all --json | <count>
```

The two counts must match. Pass `--repo` **and**, when the ledger is not the
one implied by `--repo`, `--db` as well.

**A new agent user joins the box**

1. `usermod -aG hermes <user>` (or add the user to whatever group owns
   `/var/lib/devdb`), then recycle the user manager — group membership in a
   long-running `user@UID.service` is cached and `daemon-reexec` does not
   refresh it.
2. `git config --global --add safe.directory /srv/devdb/devdb-go` as that
   user, or git refuses to read the shared repo with *dubious ownership*.
3. Verify with the two-command check above. If one user gets
   `permission denied`, the problem is a directory mode on the path, not
   SQLite.

**Rollback**

Real (non-symlink) pre-change binaries are kept:

```
/usr/local/bin/devdb.bak-main-475ba85
/usr/local/bin/devdb.bak-pre-unify-1e6f1ec092d2
/usr/local/bin/devdb.bak-pre-configpath-6c693c6
```

`cp -a /usr/local/bin/devdb <backup>` copies the **symlink**, not the
binary — always verify a backup is an ELF before trusting it.

Ledger backups from the merge live in `/root/backups/devdb-unify-2026-11-07/`.

## Known residue

- `/var/lib/hermes/projects/devdb-go` still exists as a second checkout.
  Content-wise it is fully contained in canon. It is allowlisted rather than
  deleted because hermes's `devdb-hygiene.sh` cron still iterates it.
  **M5 follow-up: repoint that script at `/srv/devdb/devdb-go`, then delete
  the clone and its allowlist line.**
- The compatibility symlink at the old workspace path can be removed once
  nothing references it. `grep -rl "workspace/projects/devdb-go"` across
  `scripts/`, `skills/`, and the systemd units is a reasonable check first.