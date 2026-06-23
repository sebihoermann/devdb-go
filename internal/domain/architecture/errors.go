package architecture

import "errors"

// Sentinel errors that callers (CLI, tests) can match with errors.Is.
// Wrapping preserves the message format callers used to see, so existing
// user-facing output stays stable while the matcher becomes structural.
var (
	// ErrInvalidTopic indicates the supplied topic does not match the
	// kebab-case rules or is on the banned list.
	ErrInvalidTopic = errors.New("invalid topic")

	// ErrNoteNotFound indicates the requested architecture note id does
	// not resolve to an existing row.
	ErrNoteNotFound = errors.New("note not found")

	// ErrMissingSource indicates one of the supplied source paths is
	// not present in repo_files. Callers should use errors.As with
	// *MissingSourceError when they need the specific path.
	ErrMissingSource = errors.New("missing source path")
)

// MissingSourceError is the typed error returned for missing source paths.
// Callers can extract the offending path via errors.As.
type MissingSourceError struct {
	Path string
}

func (e *MissingSourceError) Error() string {
	return "missing source path: " + e.Path
}

// Is reports whether target is ErrMissingSource so callers can match with
// errors.Is. The structured path lives on the receiver.
func (e *MissingSourceError) Is(target error) bool {
	return target == ErrMissingSource
}