package migrate

import (
	"reflect"
	"strings"
	"testing"
)

// TestSplitStatementsLineCommentSemicolon verifies that a semicolon inside a
// `--` line comment does NOT split a statement that lives on the same line.
// Regression for review finding 7b6b4b6f: splitStatements naive parser
// treated comment semicolons as statement terminators.
func TestSplitStatementsLineCommentSemicolon(t *testing.T) {
	script := `CREATE TABLE t (id INTEGER) -- inline comment with ; semicolon;` + "\n"
	parts := splitStatements(script)
	if len(parts) != 1 {
		t.Fatalf("parts=%d want 1: %q", len(parts), parts)
	}
	if !strings.Contains(parts[0], "-- inline comment with ; semicolon") {
		t.Fatalf("comment semicolon split the statement: %q", parts[0])
	}
}

// TestSplitStatementsSingleQuoteEscape verifies the SQL-standard `''` escape
// does not close the string literal — so a semicolon after a doubled quote is
// still inside the literal and does not split.
func TestSplitStatementsSingleQuoteEscape(t *testing.T) {
	script := `INSERT INTO t VALUES ('it''s;fine'), (1); INSERT INTO t VALUES (2);`
	parts := splitStatements(script)
	if len(parts) != 2 {
		t.Fatalf("parts=%d: %q", len(parts), parts)
	}
	if !strings.Contains(parts[0], "'it''s;fine'") {
		t.Fatalf("string literal broken by escape: %q", parts[0])
	}
	if !strings.Contains(parts[1], "INSERT INTO t VALUES (2)") {
		t.Fatalf("second statement missing: %q", parts[1])
	}
}

// TestSplitStatementsDoubleQuotedIdentifier verifies a semicolon inside a
// SQLite double-quoted identifier is treated as literal text, not a
// statement terminator.
func TestSplitStatementsDoubleQuotedIdentifier(t *testing.T) {
	script := `CREATE TABLE "weird;name" (id INTEGER); INSERT INTO "weird;name" VALUES (1);`
	parts := splitStatements(script)
	if len(parts) != 2 {
		t.Fatalf("parts=%d: %q", len(parts), parts)
	}
	if !strings.Contains(parts[0], `"weird;name"`) {
		t.Fatalf("double-quoted identifier split: %q", parts[0])
	}
	if !strings.Contains(parts[1], `INSERT INTO "weird;name"`) {
		t.Fatalf("second statement broken: %q", parts[1])
	}
}

// TestSplitStatementsDoubleQuotedIdentifierEscape covers the SQL-standard
// `""` escape inside a double-quoted identifier.
func TestSplitStatementsDoubleQuotedIdentifierEscape(t *testing.T) {
	script := `CREATE TABLE "odd""name" (id INTEGER); INSERT INTO "odd""name" VALUES (1);`
	parts := splitStatements(script)
	if len(parts) != 2 {
		t.Fatalf("parts=%d: %q", len(parts), parts)
	}
	if !strings.Contains(parts[0], `"odd""name"`) {
		t.Fatalf("escaped quote broke identifier: %q", parts[0])
	}
}

// TestSplitStatementsBlockComment covers the /* ... */ form. A semicolon
// inside a block comment is not a statement separator.
func TestSplitStatementsBlockComment(t *testing.T) {
	script := `CREATE TABLE t (id INTEGER) /* hold; tight */ ; INSERT INTO t VALUES (1);`
	parts := splitStatements(script)
	if len(parts) != 2 {
		t.Fatalf("parts=%d: %q", len(parts), parts)
	}
	if !strings.Contains(parts[0], "/* hold; tight */") {
		t.Fatalf("block comment broken: %q", parts[0])
	}
	if !strings.Contains(parts[1], "INSERT INTO t VALUES (1)") {
		t.Fatalf("second statement missing: %q", parts[1])
	}
}

// TestSplitStatementsMultilineComment verifies line comments span across newlines
// and that a semicolon inside one of those comment lines does not split.
func TestSplitStatementsMultilineComment(t *testing.T) {
	script := "-- line one\n-- line;two\nINSERT INTO t VALUES (1);"
	parts := splitStatements(script)
	if len(parts) != 1 {
		t.Fatalf("parts=%d want 1: %q", len(parts), parts)
	}
	if !strings.Contains(parts[0], "-- line;two") {
		t.Fatalf("multiline comment split: %q", parts[0])
	}
}

// TestSplitStatementsEdgeCombinations exercises tricky combinations in one
// script — comment semicolons, escaped quotes, and double-quoted identifiers
// all in flight — to confirm the state machine stays coherent. The leading
// line comment is glued to the next CREATE statement because the comment
// spans until the newline; the tail comment lives on its own.
func TestSplitStatementsEdgeCombinations(t *testing.T) {
	script := `-- top; comment
CREATE TABLE "a;b" (id INTEGER);
INSERT INTO "a;b" VALUES ('it''s;ok');
-- tail; comment
`
	parts := splitStatements(script)
	want := []string{
		"-- top; comment\nCREATE TABLE \"a;b\" (id INTEGER)",
		`INSERT INTO "a;b" VALUES ('it''s;ok')`,
		"-- tail; comment",
	}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("parts=%q want=%q", parts, want)
	}
}

// TestSplitStatementsEmptyAndTrailing covers the no-op cases — empty input,
// trailing semicolons, and whitespace-only fragments.
func TestSplitStatementsEmptyAndTrailing(t *testing.T) {
	cases := map[string][]string{
		"":                      nil,
		";":                     nil,
		";;":                    nil,
		"   ":                   nil,
		"SELECT 1;":             {"SELECT 1"},
		"SELECT 1; ":            {"SELECT 1"},
		"SELECT 1;\nSELECT 2;\n": {"SELECT 1", "SELECT 2"},
	}
	for in, want := range cases {
		got := splitStatements(in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("splitStatements(%q)=%q want=%q", in, got, want)
		}
	}
}