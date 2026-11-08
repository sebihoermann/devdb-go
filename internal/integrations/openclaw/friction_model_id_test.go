package openclaw

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
)

// frictionWorkspace builds the minimal tree the friction scanner reads.
func frictionWorkspace(t *testing.T) (workspace, repo string) {
	t.Helper()
	root := t.TempDir()
	workspace = filepath.Join(root, "workspace")
	repo = filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "2026-06-22.md"),
		[]byte("# Day\n- friction: scanning wrote an unstamped row\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace, repo
}

func initFrictionDB(t *testing.T, repo string) {
	t.Helper()
	ctx, err := app.Open(repo, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
}

// TestFrictionScanStampsModelID is the acceptance check for the OpenClaw
// finding: a row created through the documented path carries a real model id.
// The defect was composite — the write path worked only when the environment
// carried DEVDB_MODEL_ID, and the read path hid the column entirely — so a
// caller could not tell a stamped row from an unstamped one.
func TestFrictionScanStampsModelID(t *testing.T) {
	workspace, repo := frictionWorkspace(t)
	initFrictionDB(t, repo)

	var out bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workspace", workspace, "--repo", repo, "--json",
		"--model-id", "vendor/live-model-9", "friction", "scan"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scan failed: %v\n%s", err, out.String())
	}

	var result FrictionResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("scan output is not JSON: %v\n%s", err, out.String())
	}
	if result.Created != 1 {
		t.Fatalf("result=%+v, want 1 created", result)
	}
	if result.ModelID != "vendor/live-model-9" {
		t.Fatalf("result.model_id=%q, want vendor/live-model-9", result.ModelID)
	}

	ctx, err := app.Open(repo, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	rows, err := feedback.List(ctx.DB, feedback.ListFilter{Status: "all"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if rows[0].ModelID != "vendor/live-model-9" {
		t.Fatalf("stored model_id=%q, want vendor/live-model-9", rows[0].ModelID)
	}
}

// TestFrictionScanModelIDFromEnv covers the scripted path: an exported
// DEVDB_MODEL_ID stamps rows with no flag on the command line.
func TestFrictionScanModelIDFromEnv(t *testing.T) {
	t.Setenv("DEVDB_MODEL_ID", "env/friction-model")
	workspace, repo := frictionWorkspace(t)
	initFrictionDB(t, repo)

	var out bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workspace", workspace, "--repo", repo, "--json", "friction", "scan"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scan failed: %v\n%s", err, out.String())
	}

	ctx, err := app.Open(repo, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	rows, err := feedback.List(ctx.DB, feedback.ListFilter{Status: "all"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if rows[0].ModelID != "env/friction-model" {
		t.Fatalf("stored model_id=%q, want env/friction-model", rows[0].ModelID)
	}
}

// TestFrictionScanWarnsWhenModelIDUnresolved pins the visible-failure half: an
// unstamped scan still records "unknown" (honest) but says so on stderr, so it
// cannot pass for a fully attributed run.
func TestFrictionScanWarnsWhenModelIDUnresolved(t *testing.T) {
	t.Setenv("DEVDB_MODEL_ID", "")
	workspace, repo := frictionWorkspace(t)
	initFrictionDB(t, repo)

	var out bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workspace", workspace, "--repo", repo, "friction", "scan"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scan failed: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("model_id=unknown")) {
		t.Fatalf("output does not report the unresolved model id:\n%s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("--model-id")) {
		t.Fatalf("output does not say how to fix it:\n%s", out.String())
	}
}
