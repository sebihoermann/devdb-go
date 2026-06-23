package openclaw

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/app"
)

func TestDoctorReportsAllReadinessChecks(t *testing.T) {
	workspace := t.TempDir()
	repo := t.TempDir()
	ctx, err := app.Open(repo, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	_ = ctx.Close()

	result := Doctor(Config{Workspace: workspace, Repo: repo}, func(name string) (string, error) {
		if name != "openclaw" {
			t.Fatalf("lookup=%s", name)
		}
		return filepath.Join("bin", name), nil
	})
	if !result.Ready || len(result.Checks) != 4 {
		t.Fatalf("result=%+v", result)
	}
	for _, check := range result.Checks {
		if !check.OK {
			t.Fatalf("check=%+v", check)
		}
	}
}

func TestDoctorAggregatesFailures(t *testing.T) {
	result := Doctor(Config{Workspace: filepath.Join(t.TempDir(), "missing"), Repo: t.TempDir()}, func(string) (string, error) {
		return "", fmt.Errorf("not installed")
	})
	if result.Ready || len(result.Checks) != 4 {
		t.Fatalf("result=%+v", result)
	}
}
