package output

import (
	"bytes"
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
