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