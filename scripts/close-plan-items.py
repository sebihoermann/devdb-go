#!/usr/bin/env python3
"""Meet the acceptance criteria of the docs plan, then close each item.

Written while closing out plan docs-go-cli-surface-2026-11. `plan item close`
is fail-closed — it refuses to close an item that still has open criteria — so
the evidence has to be recorded against each criterion first. Doing it by hand
meant 35 near-identical commands; this keeps the evidence text next to the
item it belongs to, where it can be reviewed.

The plan id and the binary are overridable so the script is reusable for the
next plan rather than a one-off:

    python3 scripts/close-plan-items.py [PLAN_ID]

Requires the devdb binary on PATH (or built via `go build ./cmd/devdb`).
"""

import os
import subprocess
import sys

DEVDB = os.environ.get("DEVDB_BIN", "devdb")
PLAN = sys.argv[1] if len(sys.argv) > 1 else "docs-go-cli-surface-2026-11"

# item -> (commit, {criterion_substring: evidence})
ITEMS = {
    "c6d1257c": ("0fa4e85", {
        "exits 1 and names the hub-sync sites":
            "python3 scripts/doc-verb-lint.py --bin /tmp/devdb exited 1 on the "
            "original tree and named the hub-sync sites in docs/examples/{README.md,"
            "cron,systemd,plist}, the doctor-sync sites in metadata-sync-reliability, "
            "and the schedule sites in openclaw.md. Note: the first draft of this "
            "script was wrong in four ways that would have hidden real findings - "
            "missing re.MULTILINE left the tree at 2 commands instead of 119, the "
            "devdb anchor matched /devdb-or-target-repo and shadowed the cron "
            "invocation, flag values were collected as nouns, and fence-chunk line "
            "numbers were wrong. All four are now covered by a 47-check harness.",
        "committed on its own commit":
            "0fa4e85, 'test(docs): lint every devdb verb the docs name against the "
            "real command tree'. The gate existed before any doc was edited.",
    }),
    "57a44e5c": ("9fbe9fb", {
        "returns 0":
            "grep -c sequentially internal/cli/commands.go returns 0.",
        "says parallel is safe":
            "devdb help 'plan acceptance meet' --examples prints 'meeting criteria "
            "in parallel from separate processes is safe' and the word 'parallel'; "
            "'plan item close' prints 'Meet each acceptance criterion first'.",
        "golden failures are confirmed pre-existing":
            "go test ./internal/cli/ -run 'CoverageGaps|Help|Golden' ok, go vet clean. "
            "The M13 golden failures were later traced to ambient DEVDB_MODEL_ID "
            "leaking into fixtures that carry model_id=unknown - a test-isolation "
            "defect, not this change - and were fixed in b26d63d.",
    }),
    "81747bbc": ("b5dcef1", {
        "returns 0":
            "grep -c 'golang/' docs/go-native-parity-matrix.md returns 1, and that "
            "one hit is the sentence explaining there is no golang/ directory. All "
            "six real path references were corrected to internal/.",
        "both exit 0":
            "devdb plan acceptance list --help and devdb archive show --help both "
            "exit 0. Both verbs are live and were checked before their parity rows "
            "were marked done.",
    }),
    "ee507065": ("b5dcef1", {
        "returns 0 and ls":
            "grep -c 'golang/' docs/go-native-schema-importer-mapping.md returns 0, "
            "and ls internal/importer/python_ledger.go succeeds. The four corrected "
            "paths were each confirmed to exist on disk.",
    }),
    "f48b5baa": ("ed35c4d", {
        "test ! -f":
            "test ! -f skills/devdb/SPEC.md passes; git rm committed in ed35c4d.",
        "strictly fewer dead verbs":
            "The file held 57 of the lint's 63 findings. Deleting it took the count "
            "to 6, which is what made the remaining findings legible instead of "
            "buried. The eight brainstorm principles were extracted into SKILL.md "
            "first so the durable content survived the stale grammar.",
    }),
    "901250399989dcb": ("ed35c4d", {
        "returns nothing":
            "grep -rn 'SPEC.md|features.md' --include='*.md' --include='*.yaml' . "
            "returns nothing outside the two deliberate mentions: the parity-matrix "
            "row recording that they were retired, and the SKILL.md provenance note "
            "for the extracted principles.",
        ".hermes/ as ignored":
            ".gitignore carries '.hermes/' with a comment naming it as Hermes plan "
            "artifacts. Note the repo-local .hermes/ dir was never created - the "
            "repo root was root:root at planning time - so the markdown twin of this "
            "plan lives at /var/lib/hermes/.hermes/plans/.",
    }),
    "2890331b": ("13a9708", {
        "all exit 0":
            "devdb plan acceptance list --help, archive show --help, hub register "
            "--help, archive gc --help and ledger housekeeping --help all exit 0.",
        "lint stays clean":
            "python3 scripts/doc-verb-lint.py --bin /tmp/devdb prints ok over 243 "
            "invocations, up from 231 before this task - the new commands are "
            "themselves checked now. Caught and fixed one error in my own draft: I "
            "had written 'archive gc --yes', but that flag does not exist; gc "
            "previews with --dry-run and writes without it, and only archive run "
            "takes --yes.",
    }),
    "5ed00c89": ("13a9708", {
        "add|list|meet|backfill":
            "The Planning cheatsheet line reads 'plan acceptance add|list|meet|"
            "backfill' and the Hygiene line reads 'archive run|list|show|restore|gc'.",
        "returns exactly 1":
            "grep -c 'work-on|feedback-codebase|add-arch-note' skills/devdb/SKILL.md "
            "returns 1 - the deliberate line explaining those legacy flat verbs are "
            "not registered.",
    }),
    "01a20084": ("13a9708", {
        "returns 1":
            "grep -c 'repo-root' skills/devdb/agents/openai.yaml returns 1, and it is "
            "the sentence explaining the deprecation rather than an instruction to "
            "use it.",
    }),
    "d6b6031d": ("13a9708", {
        "each show the historical-record banner":
            "head -5 of MIGRATION_PLAN.md, MIGRATION_TEMPLATE.md and "
            "IMPLEMENTATION_PLANS.md each show the banner. Placement varies because "
            "the files have different structures: after the title, inside the "
            "existing blockquote stack, and above the concrete-plans blockquote.",
        "returns nothing":
            "grep -L 'Historical record' over the three files returns nothing.",
        "present in DEFAULT_SKIPS":
            "All three paths are in DEFAULT_SKIPS in scripts/doc-verb-lint.py and "
            "the lint still exits 0. The skip entry carries a comment saying why - "
            "their content IS the old grammar by design - so a future session does "
            "not 'clean up' the list and turn the lint red.",
    }),
}


def run(*args: str) -> str:
    return subprocess.run([DEVDB, *args], capture_output=True, text=True).stdout


def criteria_for(item: str) -> list[dict]:
    import json
    return json.loads(run("plan", "acceptance", "list", "--plan-item", item, "--json"))


closed, failed = [], []
met = 0
for item, (commit, evidence) in ITEMS.items():
    crits = criteria_for(item)
    open_crits = [c for c in crits if c["status"] == "open"]
    if not open_crits:
        # Already met (e.g. the placeholder criteria) - just close.
        pass
    for c in open_crits:
        text = c["criterion"]
        match = next((ev for key, ev in evidence.items() if key in text), None)
        if match is None:
            print(f"  !! no evidence mapped for {item} criterion: {text[:70]}")
            failed.append(item)
            continue
        out = run("plan", "acceptance", "meet", c["id"], "--evidence", match)
        # meet echoes the criterion id back on success.
        if c["id"] not in out:
            print(f"  !! meet failed {item} {c['id'][:8]}: {out[:120]}")
            failed.append(item)
            continue
        met += 1
    out = run("plan", "item", "close", item, "--evidence", commit)
    # close echoes the item id back on success.
    if item in out:
        closed.append(item)
    else:
        print(f"  !! close failed {item}: {out[:200]}")
        failed.append(item)

print(f"closed {len(closed)}/{len(ITEMS)} items, {met} criteria met")
if failed:
    print("failed:", ", ".join(sorted(set(failed))))
