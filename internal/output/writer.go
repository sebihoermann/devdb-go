package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
)

// Writer enforces the devdb stdout/stderr contract.
type Writer struct {
	JSON   bool
	Stdout io.Writer
	Stderr io.Writer
}

// New returns a writer using os stdout/stderr.
func New(jsonMode bool) *Writer {
	return &Writer{JSON: jsonMode, Stdout: os.Stdout, Stderr: os.Stderr}
}

// WriteResult prints a write verb result: bare id on stdout (human) or JSON object.
func (w *Writer) WriteResult(id string, meta map[string]any) error {
	if w.JSON {
		payload := map[string]any{"id": id}
		for k, v := range meta {
			payload[k] = v
		}
		return w.emitJSON(payload)
	}
	_, err := fmt.Fprintln(w.Stdout, id)
	return err
}

// PrintData emits structured read output.
func (w *Writer) PrintData(v any) error {
	if w.JSON {
		return w.emitJSON(stableJSONValue(v))
	}
	return w.printHuman(v)
}

func (w *Writer) emitJSON(v any) error {
	enc := json.NewEncoder(w.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// stableJSONValue replaces a top-level nil slice with an empty slice so
// list commands always emit [] for empty results, never null. This is the
// JSON contract every agent and downstream tool relies on (validated by
// `devdb feedback list --json` returning `[]` on an empty DB rather than
// `null`); the underlying domain layer can keep returning nil for "no
// rows matched" without breaking parsers that expect arrays.
//
// Top-level only — nested slices inside structs/maps are rare in devdb
// reads and out of scope; revisit if a struct field starts surfacing the
// same null-vs-[] confusion.
func stableJSONValue(v any) any {
	if v == nil {
		return v
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}

func (w *Writer) printHuman(v any) error {
	switch x := v.(type) {
	case string:
		_, err := fmt.Fprintln(w.Stdout, x)
		return err
	case HumanLines:
		for _, line := range x.Lines {
			if _, err := fmt.Fprintln(w.Stdout, line); err != nil {
				return err
			}
		}
		return nil
	default:
		// Same contract as the --json path: a list command must never print
		// "null" for an empty result — it prints "[]". Without this, an empty
		// result and a filter that matched nothing look identical, which
		// hides bugs like a status sentinel filtering on the literal "all".
		b, err := json.Marshal(stableJSONValue(v))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.Stdout, string(b))
		return err
	}
}

// Hint writes human-oriented detail to stderr.
func (w *Writer) Hint(format string, args ...any) {
	_, _ = fmt.Fprintf(w.Stderr, format+"\n", args...)
}

// HumanLines is compact multi-line human output.
type HumanLines struct {
	Lines []string
}

// MarshalJSON keeps HumanLines out of machine output. HumanLines is human
// prose ("no findings", "3 plans"), not a data shape: without this, --json
// emitted {"Lines": ["no findings"]}, so an agent iterating a list got an
// object instead of an array. It marshals as [] — an empty list is the
// honest machine reading of "there are no rows to report".
func (h HumanLines) MarshalJSON() ([]byte, error) {
	return []byte("[]"), nil
}

// EmptyText returns a HumanLines carrying a single phrase for an empty result,
// so a human-mode list says "no feedback" instead of printing an empty line.
//
// JSON mode is unaffected by construction: HumanLines marshals as [], so a
// command can call EmptyText on either branch and still emit a valid empty
// array. Callers should still guard on !ctx.Out.JSON where they want to skip
// building the phrase at all.
func EmptyText(phrase string) HumanLines {
	return HumanLines{Lines: []string{phrase}}
}
