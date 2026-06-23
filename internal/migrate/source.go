package migrate

import "database/sql"

// SourceMigrations defines the Go-native project ledger schema.
var SourceMigrations = []Migration{
	{
		Version:     1,
		Description: "go:core ledger tables",
		Apply: func(tx *sql.Tx) error {
			return execStatements(tx, `
CREATE TABLE IF NOT EXISTS goals (
	id TEXT PRIMARY KEY,
	kind TEXT NOT NULL CHECK (kind IN ('goal','do','dont')),
	title TEXT NOT NULL,
	body TEXT,
	status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','done','wontfix')),
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS feedback (
	id TEXT PRIMARY KEY,
	role TEXT NOT NULL CHECK (role IN ('user','model','codebase')),
	category TEXT,
	severity TEXT CHECK (severity IN ('info','low','med','medium','high','critical')),
	note TEXT NOT NULL,
	context TEXT,
	status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
	proposed_fix TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);
CREATE INDEX IF NOT EXISTS idx_feedback_role ON feedback(role);
CREATE INDEX IF NOT EXISTS idx_feedback_status ON feedback(status);
CREATE INDEX IF NOT EXISTS idx_feedback_severity ON feedback(severity);

CREATE TABLE IF NOT EXISTS features (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT,
	commit_sha TEXT,
	branch TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS plans (
	id TEXT PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	title TEXT NOT NULL,
	body TEXT,
	status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','done','archived')),
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);
CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);

CREATE TABLE IF NOT EXISTS milestones (
	id TEXT PRIMARY KEY,
	plan_id TEXT NOT NULL,
	number INTEGER NOT NULL,
	title TEXT NOT NULL,
	body TEXT,
	status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','in_progress','done','wontfix')),
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (plan_id) REFERENCES plans(id),
	UNIQUE (plan_id, number)
);
CREATE INDEX IF NOT EXISTS idx_milestones_plan ON milestones(plan_id);

CREATE TABLE IF NOT EXISTS plan_items (
	id TEXT PRIMARY KEY,
	plan_id TEXT,
	milestone_id TEXT,
	item_number INTEGER,
	phase TEXT,
	step TEXT,
	title TEXT NOT NULL,
	body TEXT,
	source_doc TEXT,
	status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','in_progress','done','wontfix')),
	approval_status TEXT NOT NULL DEFAULT 'none',
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (plan_id) REFERENCES plans(id),
	FOREIGN KEY (milestone_id) REFERENCES milestones(id)
);
CREATE INDEX IF NOT EXISTS idx_plan_items_plan ON plan_items(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_items_status ON plan_items(status);

CREATE TABLE IF NOT EXISTS plan_item_acceptance (
	id TEXT PRIMARY KEY,
	plan_item_id TEXT NOT NULL,
	ordinal INTEGER NOT NULL,
	criterion TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','met')),
	evidence TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (plan_item_id) REFERENCES plan_items(id)
);
CREATE INDEX IF NOT EXISTS idx_acceptance_plan ON plan_item_acceptance(plan_item_id);

CREATE TABLE IF NOT EXISTS plan_item_files (
	id TEXT PRIMARY KEY,
	plan_item_id TEXT NOT NULL,
	path TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('create','modify','forbidden','touched')),
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (plan_item_id) REFERENCES plan_items(id)
);

CREATE TABLE IF NOT EXISTS status_log (
	id TEXT PRIMARY KEY,
	plan_item_id TEXT,
	status TEXT NOT NULL,
	note TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (plan_item_id) REFERENCES plan_items(id)
);
CREATE INDEX IF NOT EXISTS idx_status_log_plan_item ON status_log(plan_item_id);

CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	body TEXT,
	status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','done','wontfix')),
	priority TEXT NOT NULL DEFAULT 'med',
	due_at TEXT,
	approval_status TEXT NOT NULL DEFAULT 'none',
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS reminders (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	body TEXT,
	due_at TEXT,
	file_path TEXT,
	plan_item_id TEXT,
	status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','dismissed')),
	snooze_until TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS approval_log (
	id TEXT PRIMARY KEY,
	entity_table TEXT NOT NULL,
	entity_id TEXT NOT NULL,
	action TEXT NOT NULL,
	note TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS repo_files (
	path TEXT PRIMARY KEY,
	language TEXT,
	kind TEXT NOT NULL,
	lines INTEGER,
	content_hash TEXT,
	size_bytes INTEGER,
	last_seen_at TEXT NOT NULL,
	last_scan_run_id TEXT
);

CREATE TABLE IF NOT EXISTS scan_runs (
	id TEXT PRIMARY KEY,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	git_sha TEXT,
	files_seen INTEGER,
	files_added INTEGER,
	files_changed INTEGER,
	files_removed INTEGER,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS architecture_notes (
	id TEXT PRIMARY KEY,
	topic TEXT NOT NULL,
	body TEXT NOT NULL,
	source_paths TEXT NOT NULL,
	source_hashes TEXT NOT NULL,
	confidence TEXT NOT NULL DEFAULT 'medium',
	status TEXT NOT NULL DEFAULT 'active',
	last_verified_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS review_runs (
	id TEXT PRIMARY KEY,
	scope_paths TEXT NOT NULL,
	tier TEXT NOT NULL DEFAULT 'default',
	started_at TEXT NOT NULL,
	finished_at TEXT,
	git_sha TEXT,
	files_total INTEGER,
	files_reviewed INTEGER,
	summary TEXT,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS review_findings (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	file_path TEXT,
	line_start INTEGER,
	line_end INTEGER,
	principle TEXT NOT NULL,
	title TEXT NOT NULL,
	recommendation TEXT NOT NULL,
	severity TEXT NOT NULL,
	confidence TEXT NOT NULL,
	effort TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'open',
	resolved_in_commit TEXT,
	source_hash TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (run_id) REFERENCES review_runs(id)
);

CREATE TABLE IF NOT EXISTS verification_runs (
	id TEXT PRIMARY KEY,
	command TEXT NOT NULL,
	scope TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	git_sha TEXT NOT NULL,
	exit_code INTEGER,
	output TEXT,
	notes TEXT,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	dismissed_at TEXT,
	dismissed_reason TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);

CREATE TABLE IF NOT EXISTS verification_inputs (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	file_path TEXT NOT NULL,
	role TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (run_id) REFERENCES verification_runs(id)
);

CREATE TABLE IF NOT EXISTS verification_failures (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	test_id TEXT,
	file_path TEXT,
	headline TEXT NOT NULL,
	message_excerpt TEXT,
	failure_kind TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (run_id) REFERENCES verification_runs(id)
);

CREATE TABLE IF NOT EXISTS missed_cli_calls (
	id TEXT PRIMARY KEY,
	raw_argv TEXT NOT NULL,
	normalized_command TEXT,
	failure_kind TEXT NOT NULL,
	error_message TEXT NOT NULL,
	suggested_command TEXT,
	exit_code INTEGER NOT NULL,
	cwd TEXT NOT NULL,
	repo_root TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS archive_entries (
	id TEXT PRIMARY KEY,
	source_table TEXT NOT NULL,
	source_id TEXT NOT NULL,
	payload_json TEXT NOT NULL,
	archived_at TEXT NOT NULL,
	archive_reason TEXT
);

CREATE TABLE IF NOT EXISTS sync_state (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS file_change_events (
	id TEXT PRIMARY KEY,
	scan_run_id TEXT NOT NULL,
	path TEXT NOT NULL,
	change_kind TEXT NOT NULL,
	old_hash TEXT,
	new_hash TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown',
	FOREIGN KEY (scan_run_id) REFERENCES scan_runs(id)
);
`)
		},
	},
	{
		Version:     2,
		Description: "go:commit archeology",
		Apply: func(tx *sql.Tx) error {
			return execStatements(tx, `
CREATE TABLE IF NOT EXISTS commit_archeology (
	id TEXT PRIMARY KEY,
	branch TEXT,
	sha TEXT NOT NULL UNIQUE,
	author TEXT,
	committed_at TEXT,
	subject TEXT,
	body TEXT,
	intent_tag TEXT,
	created_at TEXT NOT NULL,
	model_id TEXT NOT NULL DEFAULT 'unknown'
);
CREATE INDEX IF NOT EXISTS idx_commit_archeology_branch ON commit_archeology(branch);
`)
		},
	},
	{
		Version:     3,
		Description: "go:plan item memory_ref",
		Apply: func(tx *sql.Tx) error {
			var exists int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('plan_items') WHERE name='memory_ref'`).Scan(&exists); err != nil {
				return err
			}
			if exists > 0 {
				return nil
			}
			return execStatements(tx, `
ALTER TABLE plan_items ADD COLUMN memory_ref TEXT;
`)
		},
	},
}
