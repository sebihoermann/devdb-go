package openclaw

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureWorkspace(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverFixtureDeterministic(t *testing.T) {
	result, err := Discover(fixtureWorkspace(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != len(bootstrapDescriptions)+1 {
		t.Fatalf("files=%d", len(result.Files))
	}
	if result.Files[8].Path != "MEMORY.md" || !result.Files[8].Exists {
		t.Fatalf("memory=%+v", result.Files[8])
	}
	last := result.Files[len(result.Files)-1]
	if last.Path != "memory/2026-06-22.md" || last.Description != "Adapter day" {
		t.Fatalf("daily=%+v", last)
	}
}

func TestListHumanAndJSON(t *testing.T) {
	workspace := fixtureWorkspace(t)
	var human bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&human)
	cmd.SetArgs([]string{"--workspace", workspace, "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human.String(), "MEMORY.md") || !strings.Contains(human.String(), "memory/2026-06-22.md") {
		t.Fatalf("human output=%s", human.String())
	}

	var machine bytes.Buffer
	cmd = NewCommand()
	cmd.SetOut(&machine)
	cmd.SetArgs([]string{"--workspace", workspace, "--json", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var decoded Discovery
	if err := json.Unmarshal(machine.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Files) != len(bootstrapDescriptions)+1 {
		t.Fatalf("json files=%d", len(decoded.Files))
	}
}

func TestDiscoverMissingAndInvalidWorkspace(t *testing.T) {
	dir := t.TempDir()
	result, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != len(bootstrapDescriptions) {
		t.Fatalf("files=%d", len(result.Files))
	}
	for _, file := range result.Files {
		if file.Exists {
			t.Fatalf("unexpected file: %+v", file)
		}
	}
	if _, err := Discover(filepath.Join(dir, "missing")); err == nil || !strings.Contains(err.Error(), "workspace unavailable") {
		t.Fatalf("error=%v", err)
	}
}

func TestDiscoverDoesNotReadSymlinkOutsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Secret outside heading\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "MEMORY.md")); err != nil {
		t.Fatal(err)
	}
	result, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files[8].Exists || strings.Contains(result.Files[8].Description, "Secret") {
		t.Fatalf("symlink escaped workspace: %+v", result.Files[8])
	}
}
