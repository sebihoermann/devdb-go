package grasscutter_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/grasscutter"
	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return testutil.GrassFixture(t, name)
}

func initGrassRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	for rel, content := range files {
		target := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(content, "fixture:") {
			data, err := os.ReadFile(fixturePath(t, strings.TrimPrefix(content, "fixture:")))
			if err != nil {
				t.Fatal(err)
			}
			content = string(data)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	} {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	return repo
}

func TestDiscoverDeadFunction(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"dead_code.py": "fixture:dead_code.py",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	candidates, counts, err := grasscutter.Discover(repo, db, []string{"."}, []string{"dead"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["dead"] < 1 {
		t.Fatalf("expected dead candidates, counts=%v", counts)
	}
	found := false
	for _, c := range candidates {
		if strings.Contains(c.Title, "dead_function") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing dead_function in %+v", candidates)
	}
}

func TestDiscoverNoDeadWhenAllUsed(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"all_used.py": "def func_a():\n    return 1\n\ndef func_b():\n    return func_a() + 1\n\nfunc_b()\n",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	_, counts, err := grasscutter.Discover(repo, db, []string{"."}, []string{"dead"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["dead"] != 0 {
		t.Fatalf("expected no dead functions, counts=%v", counts)
	}
}

func TestDiscoverInlinable(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"inlinable.py": "fixture:inlinable.py",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	candidates, counts, err := grasscutter.Discover(repo, db, []string{"."}, []string{"inlinable"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["inlinable"] < 1 {
		t.Fatalf("expected inlinable candidates, counts=%v", counts)
	}
	found := false
	for _, c := range candidates {
		if strings.Contains(c.Title, "inline") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing inline candidate in %+v", candidates)
	}
}

func TestDiscoverSprawlAboveThreshold(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"huge.py": strings.Repeat("x = 1\n", 501),
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	candidates, counts, err := grasscutter.Discover(repo, db, []string{"."}, []string{"sprawl"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["sprawl"] != 1 {
		t.Fatalf("sprawl count=%v", counts)
	}
	if candidates[0].Principle != "sprawl" {
		t.Fatalf("got %+v", candidates[0])
	}
}

func TestDiscoverDuplication(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"duplicated_a.py": "fixture:duplicated.py",
		"duplicated_b.py": "fixture:duplicated_b.py",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	_, counts, err := grasscutter.Discover(repo, db, []string{"."}, []string{"duplication"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["duplication"] < 1 {
		t.Fatalf("expected duplication, counts=%v", counts)
	}
}

func TestRunDryRunNoPersist(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"dead_code.py": "fixture:dead_code.py",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	res, err := grasscutter.Run(db, repo, []string{"."}, []string{"dead"}, true, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.RunID != "" || res.Persisted {
		t.Fatalf("dry run should not persist: %+v", res)
	}
	if len(res.Candidates) < 1 {
		t.Fatal("expected candidates")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_runs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected no review runs, got %d", n)
	}
}

func TestRunPersistsGrassCutterReview(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"dead_code.py": "fixture:dead_code.py",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	res, err := grasscutter.Run(db, repo, []string{"."}, []string{"dead"}, false, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.RunID == "" {
		t.Fatal("expected run id")
	}
	var tier string
	if err := db.QueryRow(`SELECT tier FROM review_runs WHERE id=?`, res.RunID).Scan(&tier); err != nil {
		t.Fatal(err)
	}
	if tier != "grass-cutter" {
		t.Fatalf("tier=%q", tier)
	}
	var findings int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_findings WHERE run_id=?`, res.RunID).Scan(&findings); err != nil {
		t.Fatal(err)
	}
	if findings < 1 {
		t.Fatal("expected persisted findings")
	}
}

func TestLazyImportCountsAsReference(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"lazy_lib.py":  "fixture:lazy_lib.py",
		"lazy_user.py": "fixture:lazy_user.py",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	candidates, _, err := grasscutter.Discover(repo, db, []string{"."}, []string{"dead"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if strings.Contains(c.Title, "helper") {
			t.Fatalf("helper should not be dead: %+v", c)
		}
	}
	foundUnused := false
	for _, c := range candidates {
		if strings.Contains(c.Title, "unused_in_lib") {
			foundUnused = true
		}
	}
	if !foundUnused {
		t.Fatal("expected unused_in_lib dead candidate")
	}
}

func TestPytestTestFunctionsExemptFromDead(t *testing.T) {
	repo := initGrassRepo(t, map[string]string{
		"tests/test_many.py": "fixture:tests_many.py",
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	candidates, _, err := grasscutter.Discover(repo, db, []string{"."}, []string{"dead"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if strings.Contains(c.Title, "test_item") {
			t.Fatalf("test_item should be exempt: %+v", c)
		}
	}
	found := false
	for _, c := range candidates {
		if strings.Contains(c.Title, "helper_only_used_here") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected helper_only_used_here dead candidate")
	}
}

func TestManyTestsDoesNotHitCap(t *testing.T) {
	lines := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf("def test_item_%02d():\n    assert True\n\n", i))
	}
	body := strings.Join(lines, "") + "def orphan():\n    return 1\n"
	repo := initGrassRepo(t, map[string]string{
		"tests/test_bulk.py": body,
	})
	db, _ := testutil.TempDB(t)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	res, err := grasscutter.Run(db, repo, []string{"."}, []string{"dead"}, false, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_findings WHERE run_id=?`, res.RunID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 1 || count >= 50 {
		t.Fatalf("expected capped findings, got %d", count)
	}
}
