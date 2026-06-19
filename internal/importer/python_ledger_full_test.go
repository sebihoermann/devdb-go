package importer_test

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func createFullPythonDB(t *testing.T, dir, name string) string {
	t.Helper()
	path := createMinimalPythonDB(t, dir, name)
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ts := "2020-01-01T00:00:00Z"
	extra := []string{
		`CREATE TABLE features (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT, commit_sha TEXT,
			branch TEXT, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE milestones (
			id TEXT PRIMARY KEY, plan_id TEXT NOT NULL, number INTEGER NOT NULL, title TEXT NOT NULL,
			body TEXT, status TEXT NOT NULL, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE plan_item_acceptance (
			id TEXT PRIMARY KEY, plan_item_id TEXT NOT NULL, ordinal INTEGER NOT NULL,
			criterion TEXT NOT NULL, status TEXT NOT NULL, evidence TEXT,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE plan_item_files (
			id TEXT PRIMARY KEY, plan_item_id TEXT NOT NULL, path TEXT NOT NULL, role TEXT NOT NULL,
			created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE status_log (
			id TEXT PRIMARY KEY, plan_item_id TEXT, status TEXT NOT NULL, note TEXT,
			created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE tasks (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, body TEXT, status TEXT NOT NULL,
			priority TEXT NOT NULL, due_at TEXT, approval_status TEXT NOT NULL,
			created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE reminders (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, body TEXT, due_at TEXT, file_path TEXT,
			plan_item_id TEXT, status TEXT NOT NULL, snooze_until TEXT,
			created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE approval_log (
			id TEXT PRIMARY KEY, entity_table TEXT NOT NULL, entity_id TEXT NOT NULL,
			action TEXT NOT NULL, note TEXT, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE scan_runs (
			id TEXT PRIMARY KEY, started_at TEXT NOT NULL, finished_at TEXT, git_sha TEXT,
			files_seen INTEGER, files_added INTEGER, files_changed INTEGER, files_removed INTEGER,
			model_id TEXT NOT NULL)`,
		`CREATE TABLE file_change_events (
			id TEXT PRIMARY KEY, scan_run_id TEXT NOT NULL, path TEXT NOT NULL, change_kind TEXT NOT NULL,
			old_hash TEXT, new_hash TEXT, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE architecture_notes (
			id TEXT PRIMARY KEY, topic TEXT NOT NULL, body TEXT NOT NULL, source_paths TEXT NOT NULL,
			source_hashes TEXT NOT NULL, confidence TEXT NOT NULL, status TEXT NOT NULL,
			last_verified_at TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE review_runs (
			id TEXT PRIMARY KEY, scope_paths TEXT NOT NULL, tier TEXT NOT NULL, started_at TEXT NOT NULL,
			finished_at TEXT, git_sha TEXT, files_total INTEGER, files_reviewed INTEGER, summary TEXT, model_id TEXT NOT NULL)`,
		`CREATE TABLE review_findings (
			id TEXT PRIMARY KEY, run_id TEXT NOT NULL, file_path TEXT, line_start INTEGER, line_end INTEGER,
			principle TEXT NOT NULL, title TEXT NOT NULL, recommendation TEXT NOT NULL, severity TEXT NOT NULL,
			confidence TEXT NOT NULL, effort TEXT NOT NULL, status TEXT NOT NULL, resolved_in_commit TEXT,
			source_hash TEXT, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE verification_inputs (
			id TEXT PRIMARY KEY, run_id TEXT NOT NULL, file_path TEXT NOT NULL, role TEXT NOT NULL,
			content_hash TEXT NOT NULL, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE verification_failures (
			id TEXT PRIMARY KEY, run_id TEXT NOT NULL, test_id TEXT, file_path TEXT, headline TEXT,
			message_excerpt TEXT, failure_kind TEXT, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE missed_cli_calls (
			id TEXT PRIMARY KEY, raw_argv TEXT NOT NULL, normalized_command TEXT, failure_kind TEXT,
			error_message TEXT, suggested_command TEXT, exit_code INTEGER, cwd TEXT, repo_root TEXT,
			model_id TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE sync_state (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE commit_archeology (
			id TEXT PRIMARY KEY, branch TEXT, sha TEXT NOT NULL, author TEXT, committed_at TEXT,
			subject TEXT, body TEXT, intent_tag TEXT, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`INSERT INTO features(id, title, created_at, model_id) VALUES ('feat1', 'Feature', '` + ts + `', 'test')`,
		`INSERT INTO milestones(id, plan_id, number, title, status, created_at, model_id)
			VALUES ('m1', 'p1', 1, 'M1', 'planned', '` + ts + `', 'test')`,
		`INSERT INTO plan_item_acceptance(id, plan_item_id, ordinal, criterion, status, created_at, updated_at, model_id)
			VALUES ('acc1', 'pi1', 1, 'done', 'met', '` + ts + `', '` + ts + `', 'test')`,
		`INSERT INTO feedback(id, role, note, status, created_at, model_id)
			VALUES ('fb_resolved', 'model', 'resolved-row', 'resolved', '` + ts + `', 'test')`,
		`INSERT INTO feedback(id, role, note, status, created_at, model_id)
			VALUES ('fb_deferred', 'model', 'deferred-row', 'deferred', '` + ts + `', 'test')`,
		`INSERT INTO plan_item_acceptance(id, plan_item_id, ordinal, criterion, status, evidence, created_at, updated_at, model_id)
			VALUES ('acc_wontfix', 'pi1', 2, 'criterion', 'wontfix', 'note', '` + ts + `', '` + ts + `', 'test')`,
		`INSERT INTO plan_item_files(id, plan_item_id, path, role, created_at, model_id)
			VALUES ('pf1', 'pi1', 'main.go', 'modify', '` + ts + `', 'test')`,
		`INSERT INTO status_log(id, plan_item_id, status, note, created_at, model_id)
			VALUES ('sl1', 'pi1', 'done', 'ok', '` + ts + `', 'test')`,
		`INSERT INTO tasks(id, title, status, priority, approval_status, created_at, model_id)
			VALUES ('task1', 'Task', 'done', 'med', 'none', '` + ts + `', 'test')`,
		`INSERT INTO reminders(id, title, status, created_at, model_id)
			VALUES ('rem1', 'Reminder', 'open', '` + ts + `', 'test')`,
		`INSERT INTO approval_log(id, entity_table, entity_id, action, created_at, model_id)
			VALUES ('al1', 'tasks', 'task1', 'approve', '` + ts + `', 'test')`,
		`INSERT INTO scan_runs(id, started_at, model_id) VALUES ('sr1', '` + ts + `', 'test')`,
		`INSERT INTO file_change_events(id, scan_run_id, path, change_kind, created_at, model_id)
			VALUES ('fce1', 'sr1', 'main.go', 'added', '` + ts + `', 'test')`,
		`INSERT INTO architecture_notes(id, topic, body, source_paths, source_hashes, confidence, status, last_verified_at, created_at, updated_at, model_id)
			VALUES ('an1', 'topic', 'body', '[]', '[]', 'high', 'active', '` + ts + `', '` + ts + `', '` + ts + `', 'test')`,
		`INSERT INTO review_runs(id, scope_paths, tier, started_at, model_id)
			VALUES ('rr1', '.', 'default', '` + ts + `', 'test')`,
		`INSERT INTO review_findings(id, run_id, principle, title, recommendation, severity, confidence, effort, status, created_at, model_id)
			VALUES ('rf1', 'rr1', 'kiss', 'title', 'fix', 'low', 'low', 'trivial', 'resolved', '` + ts + `', 'test')`,
		`INSERT INTO verification_inputs(id, run_id, file_path, role, content_hash, created_at, model_id)
			VALUES ('vi1', 'vr1', 'main.go', 'input', 'hash', '` + ts + `', 'test')`,
		`INSERT INTO verification_failures(id, run_id, headline, created_at, model_id)
			VALUES ('vf1', 'vr1', 'fail', '` + ts + `', 'test')`,
		`INSERT INTO missed_cli_calls(id, raw_argv, failure_kind, error_message, exit_code, cwd, repo_root, model_id, created_at)
			VALUES ('mc1', 'devdb status', 'unknown', 'err', 1, '/tmp', '/tmp', 'test', '` + ts + `')`,
		`INSERT INTO sync_state(key, value, updated_at) VALUES ('k', 'v', '` + ts + `')`,
		`INSERT INTO commit_archeology(id, sha, created_at, model_id)
			VALUES ('ca1', 'abc123', '` + ts + `', 'test')`,
	}
	for _, stmt := range extra {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("full legacy setup: %v", err)
		}
	}
	return path
}

func TestImportFullLegacyFixture(t *testing.T) {
	src := createFullPythonDB(t, t.TempDir(), "full.db")
	dst := filepath.Join(t.TempDir(), "go.db")
	result, err := importer.ImportPythonDB(src, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"features", "milestones", "plan_item_files", "status_log",
		"tasks", "reminders", "approval_log", "scan_runs", "file_change_events",
		"architecture_notes", "review_runs", "review_findings", "verification_inputs",
		"verification_failures", "missed_cli_calls", "sync_state", "commit_archeology",
	} {
		if result.Tables[table] != 1 {
			t.Fatalf("table %s rows=%d", table, result.Tables[table])
		}
	}
	if result.Tables["plan_item_acceptance"] != 2 {
		t.Fatalf("plan_item_acceptance rows=%d want 2", result.Tables["plan_item_acceptance"])
	}
	if result.Tables["feedback"] != 5 {
		t.Fatalf("feedback rows=%d want 5", result.Tables["feedback"])
	}
}
