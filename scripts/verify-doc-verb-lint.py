#!/usr/bin/env python3
"""Ad-hoc verification for scripts/doc-verb-lint.py.

Not a substitute for the repo suite (which has no test for this script — that
is a gap worth closing separately). This checks the behaviours the lint's
correctness depends on, against the real built binary and the real docs.

Run: python3 scripts/verify-doc-verb-lint.py
"""

import importlib.util
import os
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
# Build into a temp dir rather than /tmp/devdb: the check must work on a
# machine that has never had a binary there, and must not pick up a stale one.
_tmp = tempfile.mkdtemp(prefix="doclint-")
BIN = os.path.join(_tmp, "devdb")
OPENCLAW = os.path.join(_tmp, "devdb-openclaw")
for pkg, out in (("./cmd/devdb", BIN), ("./cmd/devdb-openclaw", OPENCLAW)):
    res = subprocess.run(["go", "build", "-o", out, pkg], cwd=REPO,
                         capture_output=True, text=True)
    if res.returncode != 0:
        print(f"cannot build {pkg}: {res.stderr[:400]}", file=sys.stderr)
        raise SystemExit(2)

spec = importlib.util.spec_from_file_location("lint", REPO / "scripts/doc-verb-lint.py")
lint = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lint)

failures: list[str] = []
checks = 0


def check(label: str, cond: bool, detail: str = "") -> None:
    global checks
    checks += 1
    if cond:
        print(f"  PASS  {label}")
    else:
        print(f"  FAIL  {label}" + (f"\n        {detail}" if detail else ""))
        failures.append(label)


print("== 1. build_tree walks the full cobra tree ==")
tree = lint.build_tree([BIN])
check("tree has >= 100 commands (was 2 with the MULTILINE bug)", len(tree) >= 100, f"got {len(tree)}")
for path in [("init",), ("status",), ("hub", "sync"), ("plan", "acceptance", "list"),
             ("archive", "show"), ("feedback", "list"), ("ledger", "housekeeping")]:
    check(f"tree contains {' '.join(path)}", path in tree)
check("multi-level nesting resolves (hub across similar-feedback)",
      ("hub", "across") in tree)

print("\n== 2. build_tree merges the openclaw binary ==")
tree2 = lint.build_tree([BIN, OPENCLAW])
check("openclaw verbs present", ("version",) in tree2 and ("links",) in tree2)
check("merging does not lose devdb verbs", ("init",) in tree2)

print("\n== 3. nouns_after tokenisation ==")
cases = [
    ("devdb hub sync --json", ["hub", "sync"]),
    ("devdb feedback add \"auth flow\" --role model", ["feedback", "add"]),
    ("devdb --repo /x status", ["status"]),
    ("devdb hub register /path/to/repo --alias myapp", ["hub", "register"]),
    ("devdb plan acceptance list --plan-item <id>", ["plan", "acceptance", "list"]),
    ("devdb feedback list --severity high --category dx", ["feedback", "list"]),
]
for line, want in cases:
    got = lint.nouns_after(line)
    check(f"{line[:44]!r} -> {want}", got == want, f"got {got}")

print("\n== 4. non-invocations are ignored ==")
for line in ["devdb", "devdb <id>", "  # a comment about devdb"]:
    got = lint.nouns_after(line)
    check(f"{line!r} yields no command", got is None, f"got {got}")
# Prose is excluded by fence language (only command-bearing fences are
# scanned) — but prose can still sit inside a ```bash block, and a proposal
# for a verb that was never built can sit inside an inline code span. What
# matters is that neither produces a finding.
prose = Path("/srv/devdb/devdb-go/docs/INSTALLATION_MODEL.md")
lines = lint.command_lines(prose, prose.read_text())
check("layout diagram in a ```text fence is not scanned as commands",
      not any("binary on PATH" in l for l in lines),
      f"leaked in: {[l for l in lines if 'binary on PATH' in l][:2]}")
nouns = lint.nouns_after("Global tool:   devdb binary on PATH")
tree_all = lint.build_tree([BIN])
check("'devdb binary on PATH' would not resolve anyway (so no finding even if scanned)",
      nouns is not None and not any(tuple(nouns[:n]) in tree_all
                                    for n in range(len(nouns), 0, -1)),
      f"nouns={nouns}")

print("\n== 5. integration: the real repo, expected dead verbs ==")
proc = subprocess.run([sys.executable, str(REPO / "scripts/doc-verb-lint.py"), "--bin", BIN],
                      capture_output=True, text=True, cwd=REPO)
out = proc.stdout
# SPEC.md and features.md were deleted on 2026-11-08, so the tree is now clean.
check("exits 0 on the current repo (all dead verbs fixed)", proc.returncode == 0,
      f"rc={proc.returncode}\n{out[:600]}")
check("prints the ok line", "ok: every devdb verb" in out, out.strip()[:200])
check("checks a substantial number of invocations", "231 invocations" in out or
      any(str(n) + " invocations" in out for n in range(200, 260)), out.strip()[:200])
# The regression that matters most: valid verbs must NEVER be reported dead.
for valid in ["plan acceptance list", "archive show", "ledger housekeeping",
              "feedback severity", "hub register", "goal delete", "plan delete",
              "hub sync", "hub doctor", "archive gc", "plan reconcile"]:
    check(f"valid verb {valid!r} is not flagged",
          f"devdb {valid}\n" not in out and f": devdb {valid}\n" not in out)
check("devdb-openclaw verbs are not flagged (second binary walked)",
      "devdb version" not in out and "devdb friction" not in out)
check("go install .../cmd/devdb@latest does not trigger (the @ breaks the noun run)",
      "devdb@" not in out)

print("\n== 6. skip list ==")
check("parity matrix skipped (its left column is the old grammar)",
      "go-native-parity-matrix" not in out)
check("improvement plan skipped (dated record)",
      "improvements-plan-2026-11" not in out)
for f in ["MIGRATION_PLAN.md", "MIGRATION_TEMPLATE.md", "IMPLEMENTATION_PLANS.md"]:
    check(f"{f} skipped (bannered historical)", f not in out)

print("\n== 7. a clean doc tree exits 0 ==")
with tempfile.TemporaryDirectory(prefix="hermes-verify-") as td:
    tmp = Path(td)
    subprocess.run(["git", "init", "-q"], cwd=tmp, check=True)
    doc = tmp / "CLEAN.md"
    doc.write_text(
        "# Clean doc\n\n"
        "```bash\n"
        "devdb status\n"
        "devdb hub sync --json\n"
        "devdb plan acceptance list --plan my-plan\n"
        "devdb feedback add \"note\" --role codebase --severity high\n"
        "```\n\n"
        "Inline: `devdb archive show <id>` and `devdb ledger housekeeping`.\n",
        encoding="utf-8",
    )
    subprocess.run(["git", "add", "-A"], cwd=tmp, check=True)
    p = subprocess.run(
        [sys.executable, str(REPO / "scripts/doc-verb-lint.py"), "--bin", BIN,
         "--root", str(tmp)],
        capture_output=True, text=True,
    )
    check("clean tree exits 0", p.returncode == 0, f"rc={p.returncode}\n{p.stdout}")
    check("prints the ok line", "ok: every devdb verb" in p.stdout, p.stdout.strip())

    print("\n== 8. --skip-path suppresses a real finding ==")
    bad = tmp / "BAD.md"
    bad.write_text("```bash\ndevdb hub-sync\n```\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=tmp, check=True)
    p2 = subprocess.run(
        [sys.executable, str(REPO / "scripts/doc-verb-lint.py"), "--bin", BIN,
         "--root", str(tmp)],
        capture_output=True, text=True,
    )
    check("dead verb detected in the temp repo",
          p2.returncode == 1 and "hub-sync" in p2.stdout,
          f"rc={p2.returncode} out={p2.stdout.strip()[:200]}")
    p3 = subprocess.run(
        [sys.executable, str(REPO / "scripts/doc-verb-lint.py"), "--bin", BIN,
         "--root", str(tmp), "--skip-path", "BAD.md"],
        capture_output=True, text=True,
    )
    check("--skip-path suppresses it", p3.returncode == 0, p3.stdout.strip())

print(f"\n{'=' * 60}")
print(f"{checks - len(failures)}/{checks} checks passed")
if failures:
    print("FAILED:")
    for f in failures:
        print(f"  - {f}")
    sys.exit(1)
print("all ad-hoc checks passed")
