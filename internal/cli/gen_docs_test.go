package cli

import (
	"strings"
	"testing"
)

// TestDocVerbLintCommandTreeShape pins the heading contract that
// cmd/gen-docs relies on when it renders the command reference. The generator
// picks a heading level from tree depth, so a command that changes from a leaf
// to a group (or back) must not silently produce a broken outline.
//
// The levels are: # title, ## noun section, ### sub-noun group, #### verb,
// ##### sub-verb. A top-level leaf must be ## (never #### with no ## parent),
// and a verb under a noun must be ####.
func TestDocVerbLintCommandTreeShape(t *testing.T) {
	root := NewRootForDocs()

	var topLevelLeaves, topLevelGroups []string
	for _, c := range root.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if len(c.Commands()) > 0 {
			topLevelGroups = append(topLevelGroups, c.Name())
		} else {
			topLevelLeaves = append(topLevelLeaves, c.Name())
		}
	}

	// These are the commands a reader reaches for first, and all of them are
	// leaves. If one grows a subcommand, the generated reference's outline
	// changes shape and this test fails, forcing a conscious decision.
	for _, want := range []string{"init", "status", "report", "resume", "quality"} {
		found := false
		for _, got := range topLevelLeaves {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q is no longer a top-level leaf; the generator renders it as a group now", want)
		}
	}
	if len(topLevelLeaves) == 0 {
		t.Fatal("expected top-level leaf commands")
	}
	if len(topLevelGroups) == 0 {
		t.Fatal("expected top-level group commands")
	}
}

// TestExampleForDocsReturnsEmptyForUnknownKeys pins the generator's fallback:
// a verb with no curated example must render no example block rather than the
// "(no examples registered...)" placeholder that `devdb help --examples`
// deliberately shows an interactive user.
func TestExampleForDocsReturnsEmptyForUnknownKeys(t *testing.T) {
	if got := ExampleForDocs("definitely not a real verb"); got != "" {
		t.Errorf("unknown key should return empty, got %q", got)
	}
	if got := ExampleForDocs("plan reconcile"); !strings.Contains(got, "--apply") {
		t.Errorf("known key should carry its example block, got %q", got)
	}
}
