package architecture_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// TestSentinelErrorsMatchWrapping ensures the sentinel errors surface through
// errors.Is / errors.As when callers migrate away from string matching.
// Regression for review finding 86b1ba54.
func TestSentinelErrorsMatchWrapping(t *testing.T) {
	t.Run("ErrInvalidTopic is the bare sentinel", func(t *testing.T) {
		if err := architecture.ValidateTopic("Bad Topic"); !errors.Is(err, architecture.ErrInvalidTopic) {
			t.Fatalf("err=%v does not match ErrInvalidTopic", err)
		}
	})

	t.Run("ErrNoteNotFound matches via errors.Is", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		_, _, _, err := architecture.Verify(db, "does-not-exist")
		if err == nil {
			t.Fatal("expected note not found")
		}
		if !errors.Is(err, architecture.ErrNoteNotFound) {
			t.Fatalf("err=%v does not match ErrNoteNotFound", err)
		}
	})

	t.Run("ErrMissingSource matches both errors.Is and errors.As", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		_, err := architecture.Add(db, "ok-topic", "body", []string{"does/not/exist.go"}, "medium", "test")
		if err == nil {
			t.Fatal("expected missing source")
		}
		if !errors.Is(err, architecture.ErrMissingSource) {
			t.Fatalf("err=%v does not match ErrMissingSource", err)
		}
		var missing *architecture.MissingSourceError
		if !errors.As(err, &missing) {
			t.Fatalf("err=%v does not unwrap to *MissingSourceError", err)
		}
		if missing.Path != "does/not/exist.go" {
			t.Fatalf("path=%q", missing.Path)
		}
		// The user-facing message stays stable.
		if !strings.HasPrefix(err.Error(), "missing source path: ") {
			t.Fatalf("err=%v lost message prefix", err)
		}
	})
}

// TestInvalidTopicErrorIsActionable verifies that topic validation produces
// an actionable error message that names the offending input, explains the
// grammar, and provides at least one valid example. Regression for feedback
// 1ad878e5.
func TestInvalidTopicErrorIsActionable(t *testing.T) {
	t.Run("natural-language topic from feedback 1ad878e5", func(t *testing.T) {
		const natural = "ssh_log_analyzer service dispatch and Webmin extension seam"
		err := architecture.ValidateTopic(natural)
		if err == nil {
			t.Fatal("expected error for natural-language topic")
		}
		if !errors.Is(err, architecture.ErrInvalidTopic) {
			t.Fatalf("err=%v does not match ErrInvalidTopic", err)
		}
		var ite *architecture.InvalidTopicError
		if !errors.As(err, &ite) {
			t.Fatalf("err=%v does not unwrap to *InvalidTopicError", err)
		}
		if ite.Topic != natural {
			t.Fatalf("topic=%q want %q", ite.Topic, natural)
		}
		msg := err.Error()
		for _, fragment := range []string{
			natural,                       // echoes the bad input
			"kebab-case",                  // grammar
			"3-40",                        // grammar (length range)
			"lowercase",                   // grammar (charset)
			"example:",                    // actionable example follows
			"ssh-log-analyzer",            // concrete example
		} {
			if !strings.Contains(msg, fragment) {
				t.Errorf("error message missing %q\nfull message:\n%s", fragment, msg)
			}
		}
	})

	t.Run("per-failure reasons are distinct and specific", func(t *testing.T) {
		cases := []struct {
			topic    string
			contains string
		}{
			{"", "empty"},
			{"ab", "too short"},
			{strings.Repeat("a", 42), "too long"},
			{"misc", "banned"},
			{"general", "banned"},
			{"1abc", "lowercase letter"},
			{"Misc", "lowercase letter"},
			{"Camel", "lowercase letter"},
			{"has space", "uppercase letters, spaces"},
		}
		for _, tc := range cases {
			err := architecture.ValidateTopic(tc.topic)
			if err == nil {
				t.Errorf("topic %q: expected error", tc.topic)
				continue
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("topic %q: error %q missing %q", tc.topic, err.Error(), tc.contains)
			}
		}
	})

	t.Run("valid topic still returns nil", func(t *testing.T) {
		for _, topic := range []string{"ssh-log-analyzer", "auth-boundary", "m3-event-bus"} {
			if err := architecture.ValidateTopic(topic); err != nil {
				t.Errorf("topic %q rejected: %v", topic, err)
			}
		}
	})
}

// TestValidateSourcePathsInventoryAware verifies that source-path validation
// distinguishes (a) paths indexed in repo_files, (b) paths that exist on disk
// but are not indexed, and (c) paths that are absent from both. Regression for
// feedback 90afa468.
func TestValidateSourcePathsInventoryAware(t *testing.T) {
	db, dbPath := testutil.TempDB(t)
	repoRoot := filepath.Dir(dbPath)

	// Index one path; write one unindexed file to disk; reference one
	// path that is absent from both.
	indexed := "src/indexed.go"
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES (?, 'code', 'h', datetime('now'))`, indexed); err != nil {
		t.Fatal(err)
	}
	unindexed := "src/unindexed.py"
	if err := os.MkdirAll(filepath.Join(repoRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, unindexed), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ghost := "src/does-not-exist.py"

	t.Run("indexed path passes", func(t *testing.T) {
		if err := architecture.ValidateSourcePaths(db, repoRoot, []string{indexed}); err != nil {
			t.Fatalf("expected nil for indexed path, got %v", err)
		}
	})

	t.Run("existing-but-unindexed path produces UnindexedSourceError", func(t *testing.T) {
		err := architecture.ValidateSourcePaths(db, repoRoot, []string{unindexed})
		if err == nil {
			t.Fatal("expected unindexed-source error")
		}
		if !errors.Is(err, architecture.ErrUnindexedSource) {
			t.Fatalf("err=%v does not match ErrUnindexedSource", err)
		}
		var ue *architecture.UnindexedSourceError
		if !errors.As(err, &ue) {
			t.Fatalf("err=%v does not unwrap to *UnindexedSourceError", err)
		}
		if ue.Path != unindexed {
			t.Fatalf("path=%q want %q", ue.Path, unindexed)
		}
		if ue.RepoRoot != repoRoot {
			t.Fatalf("repoRoot=%q want %q", ue.RepoRoot, repoRoot)
		}
		msg := err.Error()
		for _, fragment := range []string{unindexed, "inventory", "devdb inventory scan"} {
			if !strings.Contains(msg, fragment) {
				t.Errorf("error message missing %q\nfull: %s", fragment, msg)
			}
		}
	})

	t.Run("truly missing path produces MissingSourceError", func(t *testing.T) {
		err := architecture.ValidateSourcePaths(db, repoRoot, []string{ghost})
		if err == nil {
			t.Fatal("expected missing-source error")
		}
		if !errors.Is(err, architecture.ErrMissingSource) {
			t.Fatalf("err=%v does not match ErrMissingSource", err)
		}
		var me *architecture.MissingSourceError
		if !errors.As(err, &me) {
			t.Fatalf("err=%v does not unwrap to *MissingSourceError", err)
		}
		if me.Path != ghost {
			t.Fatalf("path=%q want %q", me.Path, ghost)
		}
	})

	t.Run("mixed list short-circuits at first bad path", func(t *testing.T) {
		err := architecture.ValidateSourcePaths(db, repoRoot, []string{indexed, ghost, unindexed})
		if err == nil {
			t.Fatal("expected error")
		}
		var me *architecture.MissingSourceError
		if !errors.As(err, &me) || me.Path != ghost {
			t.Fatalf("expected first-fail on %q, got %v", ghost, err)
		}
	})

	t.Run("empty repoRoot skips filesystem check", func(t *testing.T) {
		err := architecture.ValidateSourcePaths(db, "", []string{unindexed})
		if err == nil {
			t.Fatal("expected missing-source error when repoRoot is empty")
		}
		var me *architecture.MissingSourceError
		if !errors.As(err, &me) {
			t.Fatalf("err=%v does not unwrap to *MissingSourceError", err)
		}
	})
}

// TestUnindexedSourceErrorMessageWithoutRepoRoot covers the edge case where
// callers omit the repo root but still want a useful error.
func TestUnindexedSourceErrorMessageWithoutRepoRoot(t *testing.T) {
	err := &architecture.UnindexedSourceError{Path: "src/foo.go"}
	msg := err.Error()
	for _, fragment := range []string{"src/foo.go", "inventory", "devdb inventory scan"} {
		if !strings.Contains(msg, fragment) {
			t.Errorf("message missing %q\nfull: %s", fragment, msg)
		}
	}
}