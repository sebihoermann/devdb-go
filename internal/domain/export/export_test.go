package export_test

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/export"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// seedLedger fills a fresh ledger with a scenario that exercises every view:
// an in-flight plan item, a blocked one, open feedback, an open finding, a
// drifted architecture note, a task, a reminder, and a verification run.
func seedLedger(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := testutil.TempDB(t)
	execs := []string{
		`INSERT INTO plans(id,slug,title,body,status,created_at,model_id)
		 VALUES ('p1','ledger-export','Prose ledger report export','A report.','active','2026-11-01T10:00:00Z','m')`,
		`INSERT INTO milestones(id,plan_id,number,title,body,status,created_at,model_id)
		 VALUES ('m1','p1',1,'Renderer','Domain layer.','in_progress','2026-11-01T10:05:00Z','m')`,
		`INSERT INTO plan_items(id,plan_id,milestone_id,item_number,title,body,
		 status,approval_status,created_at,model_id)
		 VALUES ('i1','p1','m1',1,'Domain renderer package','Build it.','in_progress','none','2026-11-01T10:10:00Z','m')`,
		`INSERT INTO plan_items(id,plan_id,milestone_id,item_number,title,body,
		 status,approval_status,created_at,model_id)
		 VALUES ('i2','p1','m1',2,'Prose views over the ledger','Narrate it.','done','none','2026-11-01T10:11:00Z','m')`,
		`INSERT INTO plan_items(id,plan_id,milestone_id,item_number,title,body,
		 status,approval_status,created_at,model_id)
		 VALUES ('i3','p1','m1',3,'Blocked item','Waiting.','in_progress','none','2026-11-01T10:12:00Z','m')`,
		`INSERT INTO status_log(id,plan_item_id,status,note,created_at,model_id)
		 VALUES ('s1','i3','in_progress','blocked on the upstream API','2026-11-02T09:00:00Z','m')`,
		`INSERT INTO feedback(id,role,category,severity,note,context,status,created_at,model_id)
		 VALUES ('f1','user','correctness','high','The export swallows the --view flag.','While piping','open','2026-11-03T08:00:00Z','m')`,
		`INSERT INTO feedback(id,role,category,severity,note,context,status,created_at,model_id)
		 VALUES ('f2','model','dx','low','Docs could explain the report.','','open','2026-11-04T08:00:00Z','m')`,
		`INSERT INTO review_runs(id,scope_paths,tier,started_at,finished_at,git_sha,
		 files_total,files_reviewed,summary,model_id)
		 VALUES ('rr1','src','default','2026-11-01T12:00:00Z','2026-11-01T12:05:00Z','abc123',1,1,'one run','m')`,
		`INSERT INTO review_findings(id,run_id,file_path,line_start,line_end,principle,
		 title,recommendation,severity,confidence,effort,status,created_at,model_id)
		 VALUES ('rf1','rr1','src/a.go',10,12,'correctness','Unchecked error',
		 'Handle the error from the query.','high','high','S','open','2026-11-05T08:00:00Z','m')`,
		`INSERT INTO repo_files(path,language,kind,lines,content_hash,size_bytes,last_seen_at)
		 VALUES ('src/a.go','go','code',100,'h1',100,'2026-11-01T10:00:00Z')`,
		`INSERT INTO architecture_notes(id,topic,body,source_paths,source_hashes,
		 confidence,status,last_verified_at,created_at,updated_at,model_id)
		 VALUES ('a1','cli-grammar','The grammar is noun then verb.','["src/a.go"]','{}','medium',
		 'active','2026-11-01T10:00:00Z','2026-11-01T10:00:00Z','2026-11-01T10:00:00Z','m')`,
		`INSERT INTO tasks(id,title,body,status,priority,due_at,approval_status,created_at,model_id)
		 VALUES ('t1','Review the report','Read it.','open','high','2020-01-01T00:00:00Z','none','2026-11-01T10:00:00Z','m')`,
		`INSERT INTO reminders(id,title,body,due_at,file_path,plan_item_id,status,
		 snooze_until,created_at,model_id)
		 VALUES ('r1','Re-verify the module','','2020-01-01T00:00:00Z','src/a.go','','open','',
		 '2026-11-01T10:00:00Z','m')`,
		`INSERT INTO verification_runs(id,command,scope,status,git_sha,exit_code,output,
		 notes,started_at,finished_at,dismissed_at,dismissed_reason,created_at,model_id)
		 VALUES ('v1','go test ./...','golang/','passed','abc123',0,'',' ','2026-11-06T10:00:00Z',
		 '2026-11-06T10:05:00Z','','','2026-11-06T10:00:00Z','m')`,
	}
	for _, stmt := range execs {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
	}
	return db
}

func TestRenderEmptyLedger(t *testing.T) {
	db, _ := testutil.TempDB(t)
	md, err := export.RenderMarkdown(db, export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "The ledger is empty") {
		t.Fatalf("empty ledger report missing the empty statement:\n%s", md)
	}
	for _, heading := range []string{
		"## Summary", "## Delivery", "## Quality",
		"## Work waiting on people", "## Architecture", "## Feedback",
	} {
		if !strings.Contains(md, heading) {
			t.Fatalf("report missing the %q view", heading)
		}
	}
}

func TestRenderNarratesTheLedger(t *testing.T) {
	db := seedLedger(t)
	md, err := export.RenderMarkdown(db, export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	must := []string{
		"**Posture:",
		"need a decision",
		"Prose ledger report",
		"Blocked item",
		"blocked on the upstream",
		"Unchecked error",
		"The export swallows",
		"Review the report",
		"Re-verify the module",
		"cli-grammar",
		"go test ./...",
	}
	for _, want := range must {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q:\n%s", want, md)
		}
	}
	if strings.Count(md, "```") != 0 {
		t.Errorf("report contains an unescaped code fence:\n%s", md)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	db := seedLedger(t)
	first, err := export.RenderMarkdown(db, export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := export.RenderMarkdown(db, export.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("two renders of an unchanged ledger differ:\n%s\n---\n%s", first, second)
	}
}

func TestResolveViews(t *testing.T) {
	got, err := export.ResolveViews([]string{"quality", "summary"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "summary" || got[1] != "quality" {
		t.Fatalf("views not in canonical order: %v", got)
	}
	got, err = export.ResolveViews([]string{"summary,summary"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "summary" {
		t.Fatalf("duplicate views not collapsed: %v", got)
	}
	if _, err := export.ResolveViews([]string{"bogus"}); err == nil {
		t.Fatal("expected an error for an unknown view")
	} else if !strings.Contains(err.Error(), "known: summary") {
		t.Fatalf("unknown view error does not name the known views: %v", err)
	}
}

func TestViewSelectionNarrowsTheDocument(t *testing.T) {
	db := seedLedger(t)
	md, err := export.RenderMarkdown(db, export.Options{Views: []string{"feedback"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "## Feedback") {
		t.Fatalf("selected view missing:\n%s", md)
	}
	if strings.Contains(md, "## Delivery") {
		t.Fatalf("unselected view rendered:\n%s", md)
	}
	if strings.Contains(md, "## Contents") {
		t.Fatalf("single-view report should not carry a table of contents:\n%s", md)
	}
}

func TestFenceInLedgerTextDoesNotBreakTheDocument(t *testing.T) {
	db, _ := testutil.TempDB(t)
	stmt := `INSERT INTO feedback(id,role,category,severity,note,status,created_at,model_id)
		 VALUES ('f1','user','dx','med','Broken fence:` + "\n```\n" + `not a fence','open','2026-11-03T08:00:00Z','m')`
	if _, err := db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
	md, err := export.RenderMarkdown(db, export.Options{Views: []string{"feedback"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(md, "```") != 0 {
		t.Fatalf("a fence in a ledger note reached the document:\n%s", md)
	}
}

func TestLimitCapsTheDetailAndSaysSo(t *testing.T) {
	db, _ := testutil.TempDB(t)
	for i := 0; i < 12; i++ {
		stmt := `INSERT INTO feedback(id,role,category,severity,note,status,created_at,model_id)
			 VALUES (?,'user','dx','low','note','open','2026-11-03T08:00:00Z','m')`
		if _, err := db.Exec(stmt, fmt.Sprintf("f%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	md, err := export.RenderMarkdown(db, export.Options{Views: []string{"feedback"}, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "not shown") {
		t.Fatalf("truncation is not announced:\n%s", md)
	}
	if strings.Count(md, "### f") != 3 {
		t.Fatalf("limit did not cap the detail rows:\n%s", md)
	}
	full, err := export.RenderMarkdown(db, export.Options{Views: []string{"feedback"}, Limit: -1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(full, "### f") != 12 {
		t.Fatalf("an unlimited limit did not render every row:\n%s", full)
	}
}
