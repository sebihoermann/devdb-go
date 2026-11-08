#!/usr/bin/env python3
"""Stress one ledger with parallel writers and report whether any hit a lock.

Two things historically surfaced SQLITE_BUSY: plain feedback/plan writes through
storage.WithTx, and `plan acceptance meet`, which used a bare Exec and therefore
had no retry at all. Both paths now route through WithTx, so this script drives
both and fails if any invocation returns a lock error.

Usage:
    python3 scripts/concurrency/stress_ledger.py --devdb /path/to/devdb [--writers 24]

Exit codes: 0 all writers succeeded, 1 any writer failed.
"""
from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor

LOCK_MARKERS = ("sqlite_busy", "database is locked", "code 5", "code 6")


def run(devdb: str, db: str, args: list[str]) -> tuple[int, str]:
    proc = subprocess.run(
        [devdb, "--db", db, *args],
        capture_output=True,
        text=True,
        env={**os.environ, "DEVDB_TELEMETRY": "off"},
    )
    return proc.returncode, (proc.stdout + proc.stderr).strip()


def looks_locked(output: str) -> bool:
    low = output.lower()
    return any(marker in low for marker in LOCK_MARKERS)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--devdb", default=shutil.which("devdb") or "devdb")
    ap.add_argument("--writers", type=int, default=24)
    ap.add_argument("--keep", action="store_true", help="keep the temp repo for inspection")
    args = ap.parse_args()

    root = tempfile.mkdtemp(prefix="devdb-stress-")
    repo = os.path.join(root, "stress")
    db = os.path.join(repo, ".devdb", "development.db")
    os.makedirs(os.path.dirname(db), exist_ok=True)

    try:
        rc, out = run(args.devdb, db, ["init", "--repo", repo])
        if rc != 0:
            print(f"init failed: {out}", file=sys.stderr)
            return 1

        # Phase 1: parallel feedback writes (WithTx path).
        def write_feedback(i: int) -> tuple[str, int, str]:
            code, text = run(args.devdb, db, [
                "feedback", "add", f"stress writer {i}", "--role", "model",
            ])
            return "feedback", code, text

        # Phase 2: parallel acceptance meets (the path that used a bare Exec).
        rc, out = run(args.devdb, db, ["plan", "create", "stress plan", "--slug", "stress", "--json"])
        if rc != 0:
            print(f"plan create failed: {out}", file=sys.stderr)
            return 1
        rc, out = run(args.devdb, db, ["plan", "item", "add", "stress item", "--plan", "stress", "--json"])
        if rc != 0:
            print(f"plan item add failed: {out}", file=sys.stderr)
            return 1
        item_id = json.loads(out)["id"]
        prefixes = []
        for i in range(args.writers):
            rc, out = run(args.devdb, db, ["plan", "acceptance", "add", f"criterion {i}", "--plan-item", item_id])
            if rc != 0:
                print(f"acceptance add failed: {out}", file=sys.stderr)
                return 1
        rc, out = run(args.devdb, db, ["plan", "acceptance", "list", "--plan-item", item_id])
        for line in out.splitlines():
            # Human list lines are "[ ] <prefix> <n>. <text> (<item>)".
            parts = line.split()
            if len(parts) >= 3 and parts[0] == "[" and parts[1] == "]":
                prefixes.append(parts[2])

        def meet(prefix: str) -> tuple[str, int, str]:
            code, text = run(args.devdb, db, [
                "plan", "acceptance", "meet", prefix, "--evidence", "stress",
            ])
            return "meet", code, text

        failures: list[str] = []
        busy: list[str] = []

        with ThreadPoolExecutor(max_workers=args.writers) as pool:
            results = list(pool.map(write_feedback, range(args.writers)))
            results += list(pool.map(meet, prefixes))

        for kind, code, text in results:
            if code != 0:
                failures.append(f"{kind}: {text}")
                if looks_locked(text):
                    busy.append(f"{kind}: {text}")

        total = len(results)
        print(f"writers: {total} ({args.writers} feedback + {len(prefixes)} meets)")
        if busy:
            print(f"BUSY errors: {len(busy)}")
            for b in busy[:5]:
                print(f"  {b}")
        else:
            print("BUSY errors: 0")
        if failures:
            print(f"failures: {len(failures)}")
            for f in failures[:5]:
                print(f"  {f}")
            return 1
        print("result: PASS — all writers succeeded")
        return 0
    finally:
        if args.keep:
            print(f"kept: {root}")
        else:
            shutil.rmtree(root, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())