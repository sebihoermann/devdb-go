package architecture

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors that callers (CLI, tests) can match with errors.Is.
// Wrapping preserves the message format callers used to see, so existing
// user-facing output stays stable while the matcher becomes structural.
var (
	// ErrInvalidTopic indicates the supplied topic does not match the
	// kebab-case rules or is on the banned list. Use errors.As with
	// *InvalidTopicError to recover the offending input and reason.
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

// InvalidTopicError carries the offending topic and the specific reason it
// failed validation, so callers can render an actionable message instead of
// a bare "invalid topic" string.
type InvalidTopicError struct {
	Topic  string
	Reason string
}

func (e *InvalidTopicError) Error() string {
	const grammar = "expected: kebab-case identifier, 3-40 characters, lowercase letters/digits/hyphens, starts with a letter"
	const example = "example: ssh-log-analyzer"
	return fmt.Sprintf(
		"invalid topic %q: %s\n%s\n%s",
		e.Topic, e.Reason, grammar, example,
	)
}

// Unwrap makes errors.Is(err, ErrInvalidTopic) continue to match callers
// that depend on the sentinel, even though the user-facing message is now
// structured.
func (e *InvalidTopicError) Unwrap() error {
	return ErrInvalidTopic
}

// topicReason inspects an offending topic and returns a human-readable
// reason explaining why it failed validation.
func topicReason(topic string) string {
	switch {
	case topic == "":
		return "topic is empty"
	case bannedTopics[topic]:
		return fmt.Sprintf("%q is on the banned topic list", topic)
	case len(topic) < 3:
		return fmt.Sprintf("topic is too short (%d chars; minimum is 3)", len(topic))
	case len(topic) > 40:
		return fmt.Sprintf("topic is too long (%d chars; maximum is 40)", len(topic))
	case !strings.HasPrefix(topic, strings.ToLower(topic[:1])) || topic[0] < 'a' || topic[0] > 'z':
		return "topic must start with a lowercase letter"
	default:
		return "topic contains uppercase letters, spaces, or other characters outside [a-z0-9-]"
	}
}