# devdb sync service examples

These files are ready-to-drop templates for keeping the metadata hub fresh.

## Which one should I use?

- `devdb-hub-watch.systemd.service` — use on Linux with systemd when you want a long-lived reconciler process.
- `devdb-hub-watch.launchd.plist` — use on macOS with launchd for the same watch-loop model.
- `devdb-hub-sync.cron` — use anywhere cron is available if you prefer a periodic one-shot sync instead of a resident process.

## Notes

- Replace `/path/to/devdb-or-target-repo` with the repository you want to reconcile.
- Replace `/usr/local/bin/devdb` if your `devdb` executable lives elsewhere.
- The examples all use the same core command: `devdb hub-sync`.
- For portable reliability details, see [../metadata-sync-reliability.md](../metadata-sync-reliability.md).
