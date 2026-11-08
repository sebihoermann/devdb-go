package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildOpenclaw builds the OpenClaw adapter into a temp dir, mirroring
// buildDevdb. Returns "" (without failing) when the go toolchain is absent,
// so the caller can skip rather than report a false failure on a machine that
// cannot build anything.
func buildOpenclaw(t *testing.T) string {
	t.Helper()
	// The output path MUST be absolute: `go build -C <root> -o <relative>`
	// resolves the output against the module dir, which would drop the binary
	// into the repo root. -C isolates the *build*, not the -o target.
	out := filepath.Join(t.TempDir(), "devdb-openclaw")
	return buildBinary(t, out, "./cmd/devdb-openclaw")
}

// isolatedBin builds the devdb binary alone into a temp dir and asserts the
// adapter has no sibling there — the exact shape that produced the 7
// false-positive findings before the fix.
func isolatedBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "devdb")
	buildBinary(t, bin, "./cmd/devdb")
	if _, err := os.Stat(filepath.Join(dir, "devdb-openclaw")); !os.IsNotExist(err) {
		t.Fatalf("fixture must not contain an adapter sibling, stat err = %v", err)
	}
	return bin
}

// buildBinary builds pkg into out. It runs `go build` from a temp working
// directory via -C, not from the module root: with cwd at the module root,
// `go build` stages its output as `<pkgname>-go-tmp-umask` in that same
// directory and leaves a stray `devdb-openclaw` behind in the repo root after
// every test run. Verified with strace: the linker created the temp file
// cwd-relative even though -o was absolute. -C keeps the build inside a dir
// the test owns, so the repo stays clean.
func buildBinary(t *testing.T, out, pkg string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	root := moduleRoot(t)
	work := t.TempDir()
	cmd := exec.Command("go", "build", "-C", root, "-o", out, pkg)
	cmd.Dir = work
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, res)
	}
	// The assertion the fix exists for: nothing may be left in the module root.
	if leaked := filepath.Join(root, "devdb-openclaw"); fileExists(leaked) {
		t.Fatalf("go build leaked %s into the repo root; -C isolation is broken", leaked)
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runLint runs scripts/doc-verb-lint.py and returns combined output + exit code.
// It does not t.Fatal on a non-zero exit: the lint's exit code is the assertion.
func runLint(t *testing.T, root string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("python3", append([]string{filepath.Join(root, "scripts", "doc-verb-lint.py")}, args...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return string(out), ee.ExitCode()
		}
		t.Fatalf("run lint: %v\n%s", err, out)
	}
	return string(out), 0
}

// The defect this guards against: scripts/doc-verb-lint.py used to discover
// devdb-openclaw as a *sibling* of the binary under lint and only walked it
// when that file happened to exist. Whether a doc's verb resolved therefore
// depended on install layout, so the same docs passed or failed depending on
// directory contents. That surfaced as 7 false-positive dead verbs
// (docs/integrations/openclaw.md, skills/devdb/SKILL.md) whenever the lint ran
// with --bin pointing at a build with no adapter beside it.
//
// The lint now resolves the adapter explicitly, defaulting to building it from
// ./cmd/devdb-openclaw, so an isolated bin dir must stay clean.
func TestDocVerbLintResolvesOpenclawWithoutSibling(t *testing.T) {
	buildOpenclaw(t)
	// No adapter sibling and no --openclaw-bin: the exact shape that produced
	// the 7 findings.
	bin := isolatedBin(t)

	out, code := runLint(t, moduleRoot(t), "--bin", bin)
	if code != 0 {
		t.Fatalf("lint must exit 0 without an adapter sibling, got %d\n%s", code, out)
	}
	if !strings.Contains(out, "every devdb verb named in the docs resolves") {
		t.Fatalf("expected the ok line, got:\n%s", out)
	}
	// Coverage must be earned, not assumed: the output names how the adapter
	// was resolved. If resolution silently degrades, this test fails even
	// though the exit code stayed 0.
	if !strings.Contains(out, "adapter verbs checked via:") {
		t.Fatalf("adapter coverage not reported, got:\n%s", out)
	}
}

// An explicit --openclaw-bin path is an operator's answer and must be honored,
// but a path that does not exist is a usage error rather than a reason to
// silently drop adapter coverage — that silent skip is the same defect class
// the sibling lookup had.
func TestDocVerbLintRejectsMissingOpenclawBin(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	out, code := runLint(t, moduleRoot(t), "--bin", isolatedBin(t), "--openclaw-bin", missing)
	if code == 0 {
		t.Fatalf("a missing --openclaw-bin must fail, got exit 0\n%s", out)
	}
	if !strings.Contains(out, "--openclaw-bin") {
		t.Fatalf("error should name the flag, got:\n%s", out)
	}
}

// The gate still has to catch genuinely dead verbs, otherwise the fix above
// could have "resolved" the findings by making the lint stop looking. A
// fixture with a made-up verb must exit 1 and name the file and line.
func TestDocVerbLintStillCatchesDeadVerbs(t *testing.T) {
	root := moduleRoot(t)
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", "-q", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	doc := filepath.Join(dir, "DEAD.md")
	if err := os.WriteFile(doc, []byte("# fixture\n\n```bash\ndevdb totally-made-up-verb --flag\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "-C", dir, "add", ".")
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	out, code := runLint(t, root, "--bin", isolatedBin(t), "--root", dir)
	if code != 1 {
		t.Fatalf("a dead verb must exit 1, got %d\n%s", code, out)
	}
	if !strings.Contains(out, "devdb totally-made-up-verb") {
		t.Fatalf("finding should name the dead verb, got:\n%s", out)
	}
	if !strings.Contains(out, "DEAD.md:") {
		t.Fatalf("finding should carry file:line, got:\n%s", out)
	}
}

// Tokenisation is the lint's other correctness pillar. The four rules below
// were each a real defect at some point (documented in the script's own
// docstring): a flag's value is not a noun, a path match shadows the real
// invocation, prose inside a bash fence is still a command, and a noun run
// stops at the first non-word token. These run the Python tokenizer directly
// rather than reimplementing it, so the contract stays anchored to the code
// the lint actually executes.
func TestDocVerbLintTokenisation(t *testing.T) {
	root := moduleRoot(t)
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	probe := filepath.Join(t.TempDir(), "probe.py")
	script := `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("lint", sys.argv[1])
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
cases = json.load(sys.stdin)
out = {c: m.nouns_after(c) for c in cases}
print(json.dumps(out))
`
	if err := os.WriteFile(probe, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"devdb hub sync --json",
		`devdb feedback add "auth flow" --role model`,
		"devdb --repo /x status",
		"devdb hub register /path/to/repo --alias myapp",
		"devdb plan acceptance list --plan-item <id>",
		"devdb feedback list --severity high --category dx",
		"devdb",
		"devdb <id>",
		"  # a comment about devdb",
		// A path-shaped devdb match must not shadow the real invocation that
		// follows it on the same line (the cron-line shape).
		"cd /devdb-or-target-repo && /usr/local/bin/devdb hub sync",
		// A flag value that looks like a verb must not be collected.
		"devdb archive list --table feedback",
	}
	payload, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", probe, filepath.Join(root, "scripts", "doc-verb-lint.py"))
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(string(payload))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v\n%s", err, out)
	}
	var got map[string][]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode probe output: %v\n%s", err, out)
	}

	want := map[string][]string{
		"devdb hub sync --json":                                     {"hub", "sync"},
		`devdb feedback add "auth flow" --role model`:               {"feedback", "add"},
		"devdb --repo /x status":                                    {"status"},
		"devdb hub register /path/to/repo --alias myapp":            {"hub", "register"},
		"devdb plan acceptance list --plan-item <id>":               {"plan", "acceptance", "list"},
		"devdb feedback list --severity high --category dx":         {"feedback", "list"},
		"devdb archive list --table feedback":                       {"archive", "list"},
		"cd /devdb-or-target-repo && /usr/local/bin/devdb hub sync": {"hub", "sync"},
	}
	for _, c := range cases {
		if _, ok := want[c]; ok {
			if fmt.Sprint(got[c]) != fmt.Sprint(want[c]) {
				t.Errorf("nouns_after(%q) = %v, want %v", c, got[c], want[c])
			}
			continue
		}
		// The remaining cases must yield no command at all.
		if got[c] != nil {
			t.Errorf("nouns_after(%q) = %v, want nil", c, got[c])
		}
	}
}
