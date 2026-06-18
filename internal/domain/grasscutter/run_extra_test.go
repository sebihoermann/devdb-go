package grasscutter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestRunUsesDefaultScopePaths(t *testing.T) {
	repo := t.TempDir()
	content, err := os.ReadFile(testutil.GrassFixture(t, "dead_code.py"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dead_code.py"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.InitGitRepo(t, repo)

	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, repo, nil, []string{"dead"}, false, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.RunID == "" || !strings.Contains(res.Summary, "found") {
		t.Fatalf("run result: %+v", res)
	}
}
