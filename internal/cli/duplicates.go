package cli

import (
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/dedupe"
)

// guardDuplicate refuses an add when an equivalent row already exists.
// Callers wire --allow-duplicate into allow; when the guard trips nothing is
// written, so a retry cannot silently create a second copy.
func guardDuplicate(ctx *app.Context, allow bool, kind string, q dedupe.Query) error {
	if allow {
		return nil
	}
	match, err := dedupe.Find(ctx.DB, q)
	if err != nil {
		return err
	}
	if match == nil {
		return nil
	}
	return &CLIError{
		Code:       ExitInvalidValue,
		Message:    fmt.Sprintf("duplicate %s: %q already exists as %s", kind, strings.TrimSpace(q.Text), match.ID),
		Suggestion: "pass --allow-duplicate to add it anyway",
		Kind:       "duplicate_entry",
	}
}
