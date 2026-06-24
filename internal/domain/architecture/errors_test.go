package architecture_test

import (
	"errors"
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