package architecture_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestCountStaleDoesNotHang(t *testing.T) {
	db, _ := testutil.TempDB(t)
	done := make(chan error, 1)
	go func() { _, err := architecture.CountStale(db); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("CountStale hung")
	}
}

func TestValidateTopicErrors(t *testing.T) {
	cases := []string{"", "ab", "Misc", "general", "UPPER", "has space"}
	for _, topic := range cases {
		if err := architecture.ValidateTopic(topic); err == nil {
			t.Fatalf("expected error for topic %q", topic)
		}
	}
	if err := architecture.ValidateTopic("valid-topic"); err != nil {
		t.Fatalf("valid topic rejected: %v", err)
	}
}

func TestAddListVerifyUpdate(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES ('src/a.go', 'code', 'h1', datetime('now'))`); err != nil {
		t.Fatal(err)
	}

	if _, err := architecture.Add(db, "misc", "bad", []string{"src/a.go"}, "high", "test"); err == nil {
		t.Fatal("expected invalid topic error")
	}
	if _, err := architecture.Add(db, "entry", "body", []string{"missing.go"}, "high", "test"); err == nil {
		t.Fatal("expected missing source path")
	}

	id, err := architecture.Add(db, "entry-point", "main entry", []string{"src/a.go"}, "high", "test")
	if err != nil {
		t.Fatal(err)
	}

	notes, err := architecture.List(db, architecture.ListFilter{
		TopicSubstr: "entry", TouchingPath: "src/a.go", Status: "active", Limit: 5,
	})
	if err != nil || len(notes) != 1 {
		t.Fatalf("list: %d err=%v", len(notes), err)
	}

	status, ok, _, err := architecture.Verify(db, id)
	if err != nil || !ok || status != "ok" {
		t.Fatalf("verify fresh: status=%s ok=%v err=%v", status, ok, err)
	}

	newBody := "updated body"
	updatedID, found, err := architecture.Update(db, id[:8], &newBody, nil, nil)
	if err != nil || !found || updatedID != id {
		t.Fatalf("update: id=%s found=%v err=%v", updatedID, found, err)
	}
	note, err := architecture.Get(db, id)
	if err != nil || note.Body != newBody {
		t.Fatalf("get after update: %+v err=%v", note, err)
	}

	md, err := architecture.RenderMarkdown(db)
	if err != nil || !strings.Contains(md, "entry-point") {
		t.Fatalf("markdown missing topic: %q", md)
	}
}

func TestVerifyAllFreshAndStale(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files (path, kind, content_hash, last_seen_at) VALUES ('pkg/a.go', 'source', 'hash1', datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := architecture.Add(db, "stale-note", "stale body", []string{"pkg/a.go"}, "medium", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE repo_files SET content_hash='hash2' WHERE path='pkg/a.go'`); err != nil {
		t.Fatal(err)
	}
	if _, err := architecture.Add(db, "fresh-note", "fresh body", []string{"pkg/a.go"}, "medium", "test"); err != nil {
		t.Fatal(err)
	}
	res, err := architecture.VerifyAll(db)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verified != 1 || res.Stale != 1 {
		t.Fatalf("verify all: %+v want verified=1 stale=1", res)
	}

	staleList, err := architecture.List(db, architecture.ListFilter{Stale: true})
	if err != nil || len(staleList) < 1 {
		t.Fatalf("stale list: %d err=%v", len(staleList), err)
	}
}

func TestVerifyNotFound(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := architecture.Get(db, "missing"); err == nil {
		t.Fatal("expected error for missing note")
	}
}

func TestRenderMarkdownStaleSection(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES ('src/a.go','code','h1',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	id, err := architecture.Add(db, "stale-topic", "stale content", []string{"src/a.go"}, "medium", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE repo_files SET content_hash='h2' WHERE path='src/a.go'`); err != nil {
		t.Fatal(err)
	}
	md, err := architecture.RenderMarkdown(db)
	if err != nil || !strings.Contains(md, "Stale notes") {
		t.Fatalf("markdown=%q err=%v", md, err)
	}
	_, ok, _, err := architecture.Verify(db, id)
	if err != nil || ok {
		t.Fatalf("verify stale note: ok=%v err=%v", ok, err)
	}
}

func TestUpdateNotFound(t *testing.T) {
	db, _ := testutil.TempDB(t)
	body := "x"
	_, _, err := architecture.Update(db, "missing", &body, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing note prefix")
	}
}

func TestMissingSourceError(t *testing.T) {
	err := &architecture.MissingSourceError{Path: "foo.go"}
	if err.Error() != "missing source path: foo.go" {
		t.Fatalf("error=%q", err.Error())
	}
}
