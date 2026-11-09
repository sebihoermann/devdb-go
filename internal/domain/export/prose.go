package export

import (
	"fmt"
	"strings"
	"time"
)

// defaultMaxChars caps a free-text excerpt so one verbose ledger row cannot
// swamp the report.
const defaultMaxChars = 400

// sanitize prepares free text taken from the ledger for embedding in the
// report.
//
// Ledger notes are written by agents and humans alike, and they contain
// markdown, control characters, and the occasional code fence. Embedded
// verbatim, one feedback note carrying a fence would swallow the rest of the
// document, and a note carrying a line of control characters would corrupt
// the terminal it prints to. So: normalize newlines, replace control
// characters with spaces, collapse runs of blank lines, and defuse fences.
//
// Fences are escaped rather than removed because the text is quoted, not
// executed: the reader should still see the backticks the author wrote.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	out = strings.ReplaceAll(out, "```", "\\`\\`\\`")
	return strings.TrimSpace(out)
}

// truncate shortens text to max runes and marks that it was cut. The cut is
// rune-safe, so a multi-byte character is never sliced in half.
func truncate(s string, max int) string {
	if max <= 0 {
		max = defaultMaxChars
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := max - 1
	for cut > 0 && r[cut] == ' ' {
		cut--
	}
	return strings.TrimSpace(string(r[:cut])) + "…"
}

// quote renders ledger text as a blockquote, so a reader can see where the
// report's own voice ends and the ledger's words begin. A note that starts a
// line with a quote marker is escaped so it cannot open a nested quote.
func quote(text string, max int) []string {
	text = sanitize(text)
	if text == "" {
		return nil
	}
	text = truncate(text, max)
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.HasPrefix(line, ">") {
			line = "\\" + line
		}
		if line == "" {
			out = append(out, ">")
			continue
		}
		out = append(out, "> "+line)
	}
	return out
}

// plural renders a count with the right noun form: plural(1, "item") is
// "1 item" and plural(3, "item") is "3 items". The plural form is given
// explicitly because English does not pluralize by rule — "feedback" has no
// plural, and "finding" becomes "findings".
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, pluralForm)
}

// sharePhrase renders how many of a set satisfy a condition, in a phrase
// that reads after a clause about the whole set: "it is high or critical",
// "all of them are high or critical", "2 of them are high or critical".
func sharePhrase(n, total int, predicate string) string {
	switch {
	case n == total && total == 1:
		return "it is " + predicate
	case n == total:
		return "all of them " + verb(total, "is", "are") + " " + predicate
	default:
		return fmt.Sprintf("%d of them %s %s", n, verb(n, "is", "are"), predicate)
	}
}

// driftPhrase renders how many architecture notes drifted from the files
// they name, in a phrase that reads after a clause about the whole set.
func driftPhrase(n, total int) string {
	if total == 1 {
		return "it has drifted from the file it describes"
	}
	if n == total {
		return "all of them have drifted from the files they describe"
	}
	return fmt.Sprintf("%d of them have drifted from the files they describe", n)
}

// severityRank orders severities from critical down to unset.
func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 5
	case "high":
		return 4
	case "med", "medium":
		return 3
	case "low":
		return 2
	case "info":
		return 1
	}
	return 0
}

// noun returns the noun alone, agreeing with n, without the count. It is what
// lets a sentence put the count before the noun: "3 open tasks" rather than
// "3 tasks open".
func noun(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// joinAnd renders a list as prose: "a", "a and b", "a, b and c".
func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}

// before reports whether one ledger timestamp is earlier than another.
//
// It parses both, because RFC3339 stamps with and without fractional
// seconds do not order correctly as plain strings: "...:00.5Z" sorts before
// "...:00Z" lexicographically even though it is the later of the two, and
// the ledger writes timestamps both ways.
func before(a, b string) bool {
	ta, oka := parseTimestamp(a)
	tb, okb := parseTimestamp(b)
	if oka && okb {
		return ta.Before(tb)
	}
	return a < b
}

// parseTimestamp parses a ledger timestamp, reporting whether it could.
func parseTimestamp(ts string) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// date renders a ledger timestamp as a calendar date. Ledger timestamps are
// RFC3339, so the leading ten characters are already the date; the parse is
// only there to reject values that are not timestamps at all.
func date(ts string) string {
	if ts == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// dateOf renders the oldest of a set of timestamps as a date, and the empty
// string when there are none.
func dateOf(timestamps ...string) string {
	oldest := ""
	for _, ts := range timestamps {
		if ts == "" {
			continue
		}
		if oldest == "" || before(ts, oldest) {
			oldest = ts
		}
	}
	return date(oldest)
}

// id8 shortens a row id to the eight characters devdb prints everywhere else.
func id8(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// severityLabel renders a severity for prose, with the unset severity named
// so a row with no severity is not read as a low one.
func severityLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unrated"
	}
	return s
}

// bullet renders one list item.
func bullet(format string, args ...any) string {
	return "- " + fmt.Sprintf(format, args...)
}

// sentence renders a full sentence, avoiding a stranded "is" or a doubled
// period when the subject is missing.
func sentence(format string, args ...any) string {
	s := fmt.Sprintf(format, args...)
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "!") && !strings.HasSuffix(s, "?") {
		s += "."
	}
	return s
}

// verb returns the verb form that agrees with n.
func verb(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// nowUTC is the current time, used only for facts that are relative to now
// (a reminder that has passed its due date). Nothing that merely records
// when something happened uses it: those read the ledger's own timestamps,
// so a report regenerated from an unchanged ledger is unchanged.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// severityIn reports whether a severity is one of the given values.
func severityIn(sev string, set []string) bool {
	s := strings.ToLower(strings.TrimSpace(sev))
	for _, x := range set {
		if s == strings.ToLower(strings.TrimSpace(x)) {
			return true
		}
	}
	return false
}

// trimTrailingBlank drops the trailing empty lines a view may have left, so
// the document does not accumulate blank runs between sections.
func trimTrailingBlank(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[:end]
}
