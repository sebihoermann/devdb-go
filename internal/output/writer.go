package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
		return w.emitJSON(v)
	}
	return w.printHuman(v)
}

func (w *Writer) emitJSON(v any) error {
	enc := json.NewEncoder(w.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
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
		b, err := json.Marshal(v)
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
