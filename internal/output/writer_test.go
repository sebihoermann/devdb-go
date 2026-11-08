package output

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestWriteResultHumanBareID(t *testing.T) {
	var out, errOut bytes.Buffer
	w := &Writer{JSON: false, Stdout: &out, Stderr: &errOut}
	if err := w.WriteResult("abc123", nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "abc123" {
		t.Fatalf("stdout = %q, want abc123", got)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", errOut.String())
	}
}

func TestWriteResultJSON(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: true, Stdout: &out}
	if err := w.WriteResult("abc123", map[string]any{"kind": "feedback"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id": "abc123"`) {
		t.Fatalf("unexpected json: %s", out.String())
	}
	if !strings.Contains(out.String(), `"kind": "feedback"`) {
		t.Fatalf("missing meta: %s", out.String())
	}
}

func TestPrintDataHumanLines(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: false, Stdout: &out}
	err := w.PrintData(HumanLines{Lines: []string{"line one", "line two"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "line one\nline two" {
		t.Fatalf("got %q", out.String())
	}
}

func TestPrintDataStringAndDefault(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: false, Stdout: &out}
	if err := w.PrintData("hello"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "hello" {
		t.Fatalf("string: %q", out.String())
	}
	out.Reset()
	if err := w.PrintData(map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"k"`) {
		t.Fatalf("map: %q", out.String())
	}
}

func TestNewAndHint(t *testing.T) {
	w := New(false)
	if w.JSON || w.Stdout == nil {
		t.Fatal("New defaults")
	}
	w2 := New(true)
	if !w2.JSON {
		t.Fatal("json mode")
	}
	var errOut bytes.Buffer
	w3 := &Writer{Stdout: &bytes.Buffer{}, Stderr: &errOut}
	w3.Hint("hint %s", "x")
	if !strings.Contains(errOut.String(), "hint x") {
		t.Fatalf("hint: %q", errOut.String())
	}
}

func TestWriteResultWithMeta(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: false, Stdout: &out}
	if err := w.WriteResult("id1", map[string]any{"extra": true}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "id1" {
		t.Fatalf("human meta ignored: %q", out.String())
	}
}

func TestPrintDataJSON(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: true, Stdout: &out}
	if err := w.PrintData([]string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"a"`) {
		t.Fatalf("json array: %q", out.String())
	}
}

// TestPrintDataJSONNilSliceEmitsEmptyArray verifies the JSON stability
// contract: list-style --json output must be `[]` when the result is
// empty, never `null`. Agents parsing devdb JSON depend on the array
// shape so they can iterate without a nil-guard. Closes feedback
// [654b7b3e].
func TestPrintDataJSONNilSliceEmitsEmptyArray(t *testing.T) {
	cases := []struct {
		name string
		in   any
	}{
		{"nil []string", []string(nil)},
		{"nil []int", []int(nil)},
		{"nil typed slice via interface", interface{}([]feedbackShape(nil))},
		{"non-nil empty slice still []", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			w := &Writer{JSON: true, Stdout: &out}
			if err := w.PrintData(tc.in); err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(out.String())
			if got != "[]" {
				t.Fatalf("nil/empty slice should marshal to %q, got %q", "[]", got)
			}
		})
	}
}

type feedbackShape struct {
	ID string `json:"id"`
}

// TestPrintDataHumanEmptySlicePrintsEmptyArray covers the default
// (non---json) output path: an empty list must print "[]", never "null" —
// same contract as the --json path, so an empty result and a filter that
// matched nothing are distinguishable. Regression for dogfood finding
// 9fa4900a983dafb402ae5ea45b7e3122.
func TestPrintDataHumanEmptySlicePrintsEmptyArray(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: false, Stdout: &out}
	var rows []map[string]any
	if err := w.PrintData(rows); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Fatalf("stdout = %q, want []", got)
	}
}

// TestEmptyTextHumanPrintsPhrase covers the empty-list phrasing helper: a
// human-mode list says "no feedback" instead of an empty line or a bare [].
// The writer's own empty-slice behaviour is unchanged (see
// TestPrintDataHumanEmptySlicePrintsEmptyArray) — EmptyText is an explicit
// opt-in from the command layer, so the stable JSON-ish shape stays the
// default for anything that does not ask for a phrase.
func TestEmptyTextHumanPrintsPhrase(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: false, Stdout: &out}
	if err := w.PrintData(EmptyText("no feedback")); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "no feedback" {
		t.Fatalf("stdout = %q, want %q", got, "no feedback")
	}
}

// TestEmptyTextJSONStaysEmptyArray guards the machine contract: even if a
// command hands EmptyText to the writer, --json must still emit [] so an
// agent iterating a list never receives prose. This holds by construction
// via HumanLines.MarshalJSON, not by every caller remembering to branch.
func TestEmptyTextJSONStaysEmptyArray(t *testing.T) {
	var out bytes.Buffer
	w := &Writer{JSON: true, Stdout: &out}
	if err := w.PrintData(EmptyText("no feedback")); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Fatalf("stdout = %q, want []", got)
	}
}

// TestStableJSONValuePassThrough covers the non-slice paths so the helper
// does not accidentally rewrite scalars, maps, or structs.
func TestStableJSONValuePassThrough(t *testing.T) {
	cases := []any{
		nil,
		"hello",
		42,
		map[string]any{"k": "v"},
		feedbackShape{ID: "abc"},
		[]string{"a", "b"},
	}
	for _, in := range cases {
		got := stableJSONValue(in)
		if !reflect.DeepEqual(got, in) {
			t.Fatalf("stableJSONValue changed value: in=%v got=%v", in, got)
		}
	}
}
