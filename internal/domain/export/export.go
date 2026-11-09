// Package export renders a project's devdb ledger as a prose report meant
// for a human administrator.
//
// The report speaks before it counts. It opens with what state the project
// is in, says what is moving and what is stuck, and ends with the things a
// person has to decide. Counts appear because they carry the argument, not
// because a table of numbers is the report.
//
// The output is deterministic for a given ledger: it carries no wall-clock
// time unless the caller asks for one, and it renders dates absolutely
// rather than as "3 days ago". That is what makes the document safe to
// commit — regenerating it from an unchanged ledger produces an unchanged
// file, so a diff in the committed copy means the ledger really moved.
//
// Nothing in this package writes to the database. The export is a read.
package export

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// Options controls what the report contains.
type Options struct {
	// Views selects which views to render, by name. Empty renders every view
	// in ViewOrder. An unknown name is an error: a typo that silently dropped
	// a section would leave a report that looks complete and is not.
	Views []string

	// Limit caps how many rows a view narrates in detail. Zero means each
	// view's own default cap, a negative value removes the cap entirely, and
	// a positive value is the cap itself.
	Limit int

	// MaxChars caps the length of any free-text excerpt taken from the
	// ledger. Zero means defaultMaxChars.
	MaxChars int

	// RepoRoot is the repository the ledger belongs to.
	RepoRoot string
	// DBPath is the ledger file the report was read from.
	DBPath string
	// GitSHA and GitBranch describe the checked-out tree.
	GitSHA    string
	GitBranch string
	// SchemaKind and SchemaVersion describe the ledger schema.
	SchemaKind    string
	SchemaVersion int
	// GeneratedAt stamps a wall-clock time into the header. It is empty
	// unless asked for, so a committed report does not churn on every run.
	GeneratedAt string
}

// limit resolves the effective row cap for one section. The domain layer
// reads a limit of 0 as "no LIMIT clause", so an unlimited request is
// translated to 0 here.
func (o Options) limit(def int) int {
	if o.Limit < 0 {
		return 0
	}
	if o.Limit > 0 {
		return o.Limit
	}
	return def
}

// maxChars resolves the effective free-text cap.
func (o Options) maxChars() int {
	if o.MaxChars > 0 {
		return o.MaxChars
	}
	return defaultMaxChars
}

// View is one named section of the report.
type View struct {
	// Name selects the view from the command line.
	Name string
	// Title is the heading rendered above the view body.
	Title string
	// Draw renders the view body. It returns markdown lines; the heading,
	// spacing, and table of contents are added by the caller.
	Draw func(db *sql.DB, opts Options) ([]string, error)
}

// ViewOrder is the canonical order views render in, whatever order the
// caller named them in. A report that reordered its sections when a flag
// changed would not diff cleanly against the previous one.
var ViewOrder = []string{"summary", "delivery", "quality", "work", "architecture", "feedback"}

// views is the registry of renderable views.
var views = map[string]View{
	"summary":      {Name: "summary", Title: "Summary", Draw: drawSummary},
	"delivery":     {Name: "delivery", Title: "Delivery", Draw: drawDelivery},
	"quality":      {Name: "quality", Title: "Quality", Draw: drawQuality},
	"work":         {Name: "work", Title: "Work waiting on people", Draw: drawWork},
	"architecture": {Name: "architecture", Title: "Architecture", Draw: drawArchitecture},
	"feedback":     {Name: "feedback", Title: "Feedback", Draw: drawFeedback},
}

// ViewNames returns the canonical view names in render order.
func ViewNames() []string {
	out := make([]string, len(ViewOrder))
	copy(out, ViewOrder)
	return out
}

// KnownView reports whether name is a registered view.
func KnownView(name string) bool {
	_, ok := views[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// ResolveViews resolves the requested view names to the canonical order.
// Names may be comma-separated; duplicates collapse. An unknown name is an
// error naming the known views, so a typo is a message, not a shorter
// document.
func ResolveViews(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return ViewNames(), nil
	}
	wanted := map[string]bool{}
	for _, raw := range requested {
		for _, part := range strings.Split(raw, ",") {
			name := strings.ToLower(strings.TrimSpace(part))
			if name == "" {
				continue
			}
			if _, ok := views[name]; !ok {
				return nil, fmt.Errorf("unknown view %q (known: %s)", name, strings.Join(ViewOrder, ", "))
			}
			wanted[name] = true
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("no view selected (known: %s)", strings.Join(ViewOrder, ", "))
	}
	out := make([]string, 0, len(wanted))
	for _, name := range ViewOrder {
		if wanted[name] {
			out = append(out, name)
		}
	}
	return out, nil
}

// RenderMarkdown renders the report.
func RenderMarkdown(db *sql.DB, opts Options) (string, error) {
	selected, err := ResolveViews(opts.Views)
	if err != nil {
		return "", err
	}
	var lines []string
	lines = append(lines, header(opts)...)
	if len(selected) > 1 {
		lines = append(lines, contents(selected)...)
	}
	for _, name := range selected {
		v := views[name]
		body, err := v.Draw(db, opts)
		if err != nil {
			return "", fmt.Errorf("view %s: %w", name, err)
		}
		lines = append(lines, "## "+v.Title, "")
		lines = append(lines, trimTrailingBlank(body)...)
		lines = append(lines, "")
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

// header renders the document title and the provenance line.
func header(opts Options) []string {
	name := "project"
	if opts.RepoRoot != "" {
		name = filepath.Base(opts.RepoRoot)
	}
	lines := []string{fmt.Sprintf("# Ledger report — %s", name), ""}
	var meta []string
	if opts.GitBranch != "" {
		meta = append(meta, "branch `"+opts.GitBranch+"`")
	}
	if opts.GitSHA != "" {
		meta = append(meta, "commit `"+shortSHA(opts.GitSHA)+"`")
	}
	if opts.SchemaKind != "" {
		meta = append(meta, fmt.Sprintf("schema `%s` v%d", opts.SchemaKind, opts.SchemaVersion))
	}
	if opts.GeneratedAt != "" {
		meta = append(meta, "generated "+opts.GeneratedAt)
	}
	if len(meta) > 0 {
		lines = append(lines, "_"+strings.Join(meta, " · ")+"_", "")
	}
	if dbPath := relativeDBPath(opts.RepoRoot, opts.DBPath); dbPath != "" {
		lines = append(lines, fmt.Sprintf("Read from `%s`. Regenerate with `devdb ledger export`.", dbPath), "")
	}
	return lines
}

// relativeDBPath renders the ledger path relative to the repository root, so
// the header reads `.devdb/development.db` rather than an absolute path that
// says nothing to a reader of a committed document.
func relativeDBPath(repoRoot, dbPath string) string {
	if dbPath == "" {
		return ""
	}
	// The ledger always lives in a .devdb directory beside the repository.
	// Rendering only that keeps a committed report free of machine paths,
	// which change per checkout and would make the document churn.
	if parent := filepath.Base(filepath.Dir(dbPath)); parent == ".devdb" {
		return filepath.Join(parent, filepath.Base(dbPath))
	}
	if repoRoot != "" {
		if rel, err := filepath.Rel(repoRoot, dbPath); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return dbPath
}

// contents renders a table of contents for the selected views.
func contents(selected []string) []string {
	lines := []string{"## Contents", ""}
	for _, name := range selected {
		v := views[name]
		lines = append(lines, fmt.Sprintf("- [%s](#%s)", v.Title, anchor(v.Title)))
	}
	return append(lines, "")
}

// anchor converts a heading into the GitHub-flavored anchor a link to it
// uses: lower case, spaces to dashes, punctuation dropped.
func anchor(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// shortSHA trims a commit hash to the seven characters git shows by default.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
