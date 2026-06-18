package architecture

import (
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestListFiltersAndStaleRender(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES ('a.go','code','h1',datetime('now')), ('b.go','code','h2',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	id1, _ := Add(db, "fresh-topic", "fresh body", []string{"a.go"}, "high", "test")
	_, _ = Add(db, "stale-topic", "stale body", []string{"b.go"}, "low", "test")
	_, _ = db.Exec(`UPDATE repo_files SET content_hash='changed' WHERE path='b.go'`)

	staleList, err := List(db, ListFilter{Stale: true})
	if err != nil || len(staleList) == 0 {
		t.Fatalf("stale list=%d err=%v", len(staleList), err)
	}
	touching, err := List(db, ListFilter{TouchingPath: "a.go", Limit: 1})
	if err != nil || len(touching) != 1 {
		t.Fatalf("touching=%d err=%v", len(touching), err)
	}
	n, err := CountStale(db)
	if err != nil || n < 1 {
		t.Fatalf("count stale=%d err=%v", n, err)
	}
	md, err := RenderMarkdown(db)
	if err != nil || !strings.Contains(md, "Stale notes") {
		t.Fatalf("markdown=%q err=%v", md, err)
	}
	res, err := VerifyAll(db)
	if err != nil || res.Stale < 1 {
		t.Fatalf("verify all=%+v err=%v", res, err)
	}
	_, ok, _, err := Verify(db, id1[:8])
	if err != nil || !ok {
		t.Fatalf("verify fresh=%v err=%v", ok, err)
	}
}

func TestUpdateConfidenceAndSources(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES ('x.go','code','h',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	id, _ := Add(db, "update-me", "body", []string{"x.go"}, "medium", "test")
	conf := "high"
	_, found, err := Update(db, id, nil, []string{"x.go"}, &conf)
	if err != nil || !found {
		t.Fatalf("update conf: found=%v err=%v", found, err)
	}
	body := "new body"
	_, found, err = Update(db, id[:8], &body, nil, nil)
	if err != nil || !found {
		t.Fatalf("update body: found=%v err=%v", found, err)
	}
	_, found, err = Update(db, "zzzzzzzz", &body, nil, nil)
	if err == nil || found {
		t.Fatalf("missing update: found=%v err=%v", found, err)
	}
}

func TestContainsPathAndSortedKeys(t *testing.T) {
	if !containsPath([]string{"pkg/a.go", "other"}, "pkg/a.go") {
		t.Fatal("exact match")
	}
	if containsPath([]string{"pkg/a.go"}, "other") {
		t.Fatal("no match")
	}
	keys := sortedKeys(map[string][]Note{"b": {}, "a": {}})
	if len(keys) != 2 || keys[0] != "a" {
		t.Fatalf("keys=%v", keys)
	}
}

func TestGetMissingNote(t *testing.T) {
	db, _ := testutil.TempDB(t)
	_, err := Get(db, "zzzzzzzz")
	if err == nil {
		t.Fatalf("expected missing error")
	}
}

func TestSourceSnapshotNullHash(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES ('n.go','code',NULL,datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	hashes, err := sourceSnapshot(db, []string{"n.go"})
	if err != nil || hashes["n.go"] != "" {
		t.Fatalf("hashes=%v err=%v", hashes, err)
	}
}
