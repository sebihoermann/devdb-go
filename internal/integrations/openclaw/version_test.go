package openclaw

import (
	"bytes"
	"strings"
	"testing"
)

// TestVersionVerbReportsBuildRevision guards the reason the version verb
// exists. It used to print a hardcoded "dev", which made an installed
// adapter indistinguishable from a stale one - the same trap that let
// /var/lib/hermes/bin/devdb sit eight commits behind HEAD unnoticed.
func TestVersionVerbReportsBuildRevision(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "abc1234"

	var out bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, "abc1234") {
		t.Fatalf("version output %q does not report the build revision", got)
	}
}

// TestVersionVerbDefaultsToDev keeps the plain-go-build case honest: the
// revision is "dev" until a release build injects it, but it is still
// reported rather than omitted.
func TestVersionVerbDefaultsToDev(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "dev"

	var out bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); !strings.Contains(got, "dev") {
		t.Fatalf("version output %q does not report the default", got)
	}
}
