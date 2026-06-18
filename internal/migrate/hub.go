package migrate

import "database/sql"

// HubMigrations defines the user-local metadata hub schema (M7).
var HubMigrations = []Migration{
	{
		Version:     1,
		Description: "go:hub registry and snapshots",
		Apply: func(tx *sql.Tx) error {
			return execStatements(tx, `
CREATE TABLE IF NOT EXISTS projects (
	alias TEXT PRIMARY KEY,
	root_path TEXT NOT NULL UNIQUE,
	registered_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_snapshots (
	alias TEXT PRIMARY KEY,
	root_path TEXT NOT NULL,
	synced_at TEXT NOT NULL,
	sync_status TEXT NOT NULL,
	snapshot_json TEXT NOT NULL,
	FOREIGN KEY (alias) REFERENCES projects(alias)
);
`)
		},
	},
}
