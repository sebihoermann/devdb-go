#!/usr/bin/env python3
"""Assert every devdb verb the docs name actually exists in the CLI.

Walks the real cobra command tree, then extracts every devdb invocation from
the docs and resolves each one. A dead verb means a reader who copy-pastes the
doc gets `unknown command`.

Scope decisions (each one prevents a class of false positive):

- Only *explicit code markers* are scanned: fenced code blocks and inline
  backtick spans in Markdown, plus the service/cron/plist files that are
  nothing but a command. Plain prose that happens to say "devdb report" is not
  a command reference and is not checked.
- `\\s+` between tokens would let a match run across a newline and glue two
  separate commands together, so tokenisation is line-based.
- Global flags (`--repo`, `--json`) and non-command tokens (paths, `<id>`,
  quoted text, `$VAR`) are dropped before the noun run is resolved.

Usage:
    python3 scripts/doc-verb-lint.py [--bin PATH] [--root PATH]
                                     [--openclaw-bin PATH]
                                     [--skip-path REL] ...

Exit codes: 0 clean, 1 dead verbs found, 2 usage error.

The OpenClaw adapter is resolved deterministically: --openclaw-bin wins,
otherwise it is built from ./cmd/devdb-openclaw, and a sibling of --bin is
the last resort. An unresolvable adapter is reported as a warning instead of
being silently skipped, because a gate whose verdict depends on install
layout cannot be trusted.
"""

from __future__ import annotations

import argparse
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

AVAILABLE = re.compile(r"^Available Commands:\s*$")
# MULTILINE so ^ anchors at each line start, not only the string start —
# without it findall sees just the first child of every parent. One-or-more
# spaces, because cobra pads short verb names with a single space
# ("ledger  housekeeping" is a real two-word path).
SUBCMD = re.compile(r"^  ([a-z][a-z0-9-]*)\s+\S", re.MULTILINE)

# Markdown code markers.
FENCE = re.compile(r"```(\w*)[^\n]*\n(.*?)```", re.DOTALL)
INLINE_CODE = re.compile(r"`([^`\n]+)`")
# Fence languages whose content is commands. A ```text block is a diagram, a
# ```python block is a program, and neither is a place a reader copy-pastes a
# devdb invocation from — scanning them produces findings that are not real.
COMMAND_FENCES = {"", "bash", "sh", "shell", "zsh", "console", "cron"}
# A command noun: lowercase, may contain hyphens. Deliberately excludes
# uppercase, digits-leading, and anything with / < > $ " ' = | in it.
NOUN = re.compile(r"^[a-z][a-z0-9-]*$")
# `devdb` as a word, not inside a longer word. The char class before it covers
# the real contexts it appears in: line start, after whitespace, after `&&` in
# a cron line, after a path separator in `/usr/local/bin/devdb`, and after the
# shell punctuation that introduces a command.
#
# The lookahead deliberately allows only whitespace or end-of-line after the
# binary name. Allowing `-` here would match `/devdb-or-target-repo` — a
# directory in a cron line — because `search` returns the FIRST match and that
# false one would shadow the real invocation later on the same line. Global
# flags that precede the verb (`devdb --repo /x status`) are handled by
# nouns_after, which skips the flag and its value.
DEVDB = re.compile(r"(?:^|[\s&|/`$()])devdb(-openclaw)?(?=[ \t]|$)")
PLIST_STRING = re.compile(r"<string>([^<]*)</string>")

DOC_SUFFIXES = {".md", ".cron", ".service", ".plist"}
# Deliberately historical: the parity matrix's left column IS the old grammar,
# the improvement plan is a dated record of a finished backlog, and the three
# root-level migration records are bannered historical artifacts. None of them
# is a live command reference.
DEFAULT_SKIPS = {
    "docs/go-native-parity-matrix.md",
    "docs/improvements-plan-2026-11.md",
    "MIGRATION_PLAN.md",
    "MIGRATION_TEMPLATE.md",
    "IMPLEMENTATION_PLANS.md",
}


def run(binary: str, args: list[str]) -> str:
    proc = subprocess.run([binary, *args, "--help"], capture_output=True, text=True)
    return proc.stdout + proc.stderr


def resolve_openclaw(
    binaries: list[str], explicit: str | None, root: Path
) -> tuple[list[str], str | None]:
    """Return the binaries to walk plus a note explaining the adapter's origin.

    The adapter used to be discovered as a *sibling* of the main binary and
    appended only when it happened to exist. That made a dead verb decision
    depend on install layout: the same docs passed or failed depending on
    whether a `devdb-openclaw` file sat next to the binary under lint. A gate
    whose verdict flips on directory contents is a gate nobody can trust, so
    the adapter is now resolved in a deterministic order:

      1. `--openclaw-bin PATH` — the operator's explicit answer, always used.
      2. build it from ./cmd/devdb-openclaw into a temp dir — the repo is the
         source of truth, so the lint always checks the adapter's real verbs.
      3. a sibling next to the main binary — last resort, for environments
         where Go is unavailable.

    A path that cannot be used is reported rather than skipped, so an operator
    who points at a stale binary hears about it instead of silently losing
    adapter coverage.
    """
    if explicit:
        if not Path(explicit).exists():
            raise SystemExit(
                f"error: --openclaw-bin {explicit} does not exist; the adapter "
                "verbs in the docs would be silently unchecked"
            )
        return [*binaries, explicit], f"--openclaw-bin {explicit}"

    built = build_openclaw(root)
    if built:
        return [*binaries, built], "built from ./cmd/devdb-openclaw"

    sibling = Path(binaries[0]).with_name("devdb-openclaw")
    if sibling.exists():
        return [*binaries, str(sibling)], f"sibling {sibling}"

    return binaries, "unresolved"


def build_openclaw(root: Path) -> str | None:
    """Build the adapter into a temp dir, or return None when Go is absent.

    internal/cli/doc_verb_lint_test.go builds both binaries into temp dirs the
    same way, so the repo suite and this script agree on what "the adapter"
    means. Building is what makes the default invocation correct on a machine
    that has neither binary installed.

    The build runs with cwd=root so `./cmd/devdb-openclaw` resolves against
    the repo the docs belong to, not the directory the lint was invoked from.
    """
    if shutil.which("go") is None:
        return None
    out = Path(tempfile.mkdtemp(prefix="doclint-openclaw-")) / "devdb-openclaw"
    proc = subprocess.run(
        ["go", "build", "-o", str(out), "./cmd/devdb-openclaw"],
        capture_output=True,
        text=True,
        cwd=root,
    )
    return str(out) if proc.returncode == 0 and out.exists() else None


def build_tree(binaries: list[str]) -> set[tuple[str, ...]]:
    """Return every (noun, verb, subverb…) path any of the binaries registers.

    Both binaries are walked because the docs name commands from each, and a
    dead verb is a dead verb regardless of which binary it belongs to.
    """
    tree: set[tuple[str, ...]] = set()

    def walk(binary: str, prefix: tuple[str, ...]) -> None:
        children = SUBCMD.findall(_available_block(run(binary, list(prefix))))
        for child in children:
            path = (*prefix, child)
            tree.add(path)
            walk(binary, path)

    for binary in binaries:
        walk(binary, ())
    return tree


def _available_block(text: str) -> str:
    lines = text.splitlines()
    try:
        start = next(i for i, l in enumerate(lines) if AVAILABLE.search(l))
    except StopIteration:
        return ""
    out = []
    for line in lines[start + 1:]:
        if line and not line.startswith("  "):
            break
        out.append(line)
    return "\n".join(out)


def command_lines(path: Path, text: str) -> list[str]:
    """Return the lines that actually contain a devdb invocation."""
    suffix = path.suffix
    if suffix == ".md":
        chunks = [
            m.group(2)
            for m in FENCE.finditer(text)
            if m.group(1) in COMMAND_FENCES
        ]
        chunks += [m.group(1) for m in INLINE_CODE.finditer(text)]
        lines: list[str] = []
        for chunk in chunks:
            lines.extend(chunk.splitlines())
        return lines
    if suffix == ".plist":
        # The argv is a sequence of <string> elements; find the one naming the
        # binary and join it with the arguments that follow it.
        values = PLIST_STRING.findall(text)
        lines = []
        for i, v in enumerate(values):
            if "devdb" in v:
                lines.append(" ".join([v, *values[i + 1 : i + 4]]))
        return lines
    if suffix in {".cron", ".service"}:
        return [line for line in text.splitlines() if "devdb" in line]
    return []


def nouns_after(line: str) -> list[str] | None:
    """Extract the noun run following `devdb` on this line, or None.

    Several `devdb` matches can occur on one line — a cron entry has the
    binary inside a path (`/usr/local/bin/devdb`) *and* a directory named
    `devdb-or-target-repo` in its `cd`. The first match that yields a real
    noun run wins; a match inside a path prefix yields nothing and is skipped.

    Two tokenisation rules earn their keep:

    - A flag's *value* is not a noun. `--severity high` must stop the noun run
      at `list`, not append `high`, or the resolver looks for a command that
      cannot exist. So the token after a flag is skipped, unless it is itself
      a flag (the `--flag --flag` form).
    - The run ends at the first token that is not a bare lowercase word —
      a path, `<id>`, quoted text, `$VAR`, a redirect. Anything after the
      command's own arguments is not part of the command path.
    """
    for m in DEVDB.finditer(line):
        # The DEVDB pattern ends in a lookahead, so m.end() is the end of the
        # binary name — advance past the separator to reach the arguments.
        rest = line[m.end() :].lstrip(" \t")
        nouns = _noun_run(rest)
        if nouns:
            return nouns
    return None


def _noun_run(rest: str) -> list[str] | None:
    nouns: list[str] = []
    skip_next = False
    for token in rest.split():
        if skip_next and not token.startswith("-"):
            skip_next = False
            continue
        skip_next = False
        if token.startswith("-"):
            skip_next = True  # consume this flag's value
            continue
        if not NOUN.match(token):
            break
        nouns.append(token)
        if len(nouns) == 4:
            break
    return nouns or None


def _find_line(file_lines: list[str], needle: str) -> int | None:
    """Locate `needle` in the file, returning its 1-based line number.

    command_lines may hand back a normalised version of the line (the plist
    branch joins argv elements), so fall back to a substring match on the
    first token before giving up.
    """
    stripped = needle.strip()
    for i, line in enumerate(file_lines, start=1):
        if stripped and stripped in line:
            return i
    head = stripped.split(" devdb")[0][:40]
    for i, line in enumerate(file_lines, start=1):
        if head and head in line:
            return i
    return None


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--bin", action="append", default=[], help="devdb binary (repeatable)")
    ap.add_argument("--root", default=".")
    ap.add_argument("--skip-path", action="append", default=[])
    ap.add_argument(
        "--openclaw-bin",
        default=None,
        help="devdb-openclaw binary to walk (default: build it from ./cmd/devdb-openclaw)",
    )
    args = ap.parse_args()

    root = Path(args.root).resolve()
    skips = DEFAULT_SKIPS | set(args.skip_path)
    binaries = args.bin or ["devdb"]
    # The OpenClaw adapter ships its own binary and the docs name its verbs, so
    # walk it too. Resolution is explicit (see resolve_openclaw) because a
    # verdict that depends on where the binary happens to be installed cannot
    # be trusted: the same docs used to pass or fail on sibling presence alone.
    binaries, origin = resolve_openclaw(binaries, args.openclaw_bin, root)

    tree = build_tree(binaries)
    if not tree:
        print(f"error: no command tree from {binaries!r}", file=sys.stderr)
        return 2

    tracked = subprocess.run(
        ["git", "ls-files"], cwd=root, capture_output=True, text=True, check=True
    ).stdout.split()

    dead: list[str] = []
    checked = 0
    for rel in tracked:
        path = root / rel
        if path.suffix not in DOC_SUFFIXES or rel in skips:
            continue
        text = path.read_text(errors="replace")
        # command_lines concatenates fence chunks, so its own line numbering
        # does not match the file. Search the real text for the line that
        # produced the finding so file:line is where a reader should look.
        file_lines = text.splitlines()
        for line in command_lines(path, text):
            nouns = nouns_after(line)
            if not nouns:
                continue
            checked += 1
            # Resolve the longest matching prefix first: `hub sync` is valid
            # even though neither `hub` nor `sync` is a top-level command.
            if any(tuple(nouns[:n]) in tree for n in range(len(nouns), 0, -1)):
                continue
            lineno = _find_line(file_lines, line) or 0
            dead.append(f"{rel}:{lineno}: devdb {' '.join(nouns)}")

    if dead:
        print(f"{len(dead)} dead verb(s) in docs ({checked} invocations checked):")
        for d in dead:
            print(f"  {d}")
        return 1
    print(f"ok: every devdb verb named in the docs resolves ({checked} invocations checked)")
    if origin != "unresolved":
        print(f"adapter verbs checked via: {origin}")
    else:
        print(
            "warning: devdb-openclaw could not be resolved (no Go build, no "
            "sibling, no --openclaw-bin); its verbs in the docs are unchecked"
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
