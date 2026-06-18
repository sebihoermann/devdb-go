package app

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sebihoermann/devdb-go/internal/config"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// Context holds shared runtime dependencies for command handlers.
type Context struct {
	Project config.Project
	DB      *sql.DB
	Out     *output.Writer
	ModelID string
}

// Open opens or creates the project database connection.
func Open(repo, dbPath string, jsonOut bool) (*Context, error) {
	proj, err := config.Resolve(repo, dbPath)
	if err != nil {
		return nil, err
	}
	modelID := os.Getenv("DEVDB_MODEL_ID")
	if modelID == "" {
		modelID = "unknown"
	}

	var db *sql.DB
	if _, err := os.Stat(proj.DBPath); os.IsNotExist(err) {
		db = nil
	} else if err != nil {
		return nil, err
	} else {
		db, err = storage.Open(proj.DBPath)
		if err != nil {
			return nil, err
		}
	}

	return &Context{
		Project: proj,
		DB:      db,
		Out:     output.New(jsonOut),
		ModelID: modelID,
	}, nil
}

// InitDB creates .devdb and applies Go-native migrations.
func (c *Context) InitDB() error {
	if err := os.MkdirAll(filepath.Dir(c.Project.DBPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Project.DevDBDir, 0o755); err != nil {
		return err
	}
	if c.DB != nil {
		_ = c.DB.Close()
	}
	db, err := storage.Open(c.Project.DBPath)
	if err != nil {
		return err
	}
	c.DB = db
	return migrate.RunAll(db)
}

// RequireDB ensures a database is open and migrated.
func (c *Context) RequireDB() error {
	if c.DB != nil {
		kind, _, err := storage.DetectSchema(c.DB)
		if err != nil {
			return err
		}
		if kind == storage.SchemaPython {
			return fmt.Errorf("legacy Python database at %s — run: devdb import python-db", c.Project.DBPath)
		}
		if kind == storage.SchemaGo || kind == storage.SchemaUnknown {
			return migrate.RunAll(c.DB)
		}
		return fmt.Errorf("unrecognized database schema at %s", c.Project.DBPath)
	}
	if _, err := os.Stat(c.Project.DBPath); os.IsNotExist(err) {
		return fmt.Errorf("database not initialized — run: devdb init")
	}
	db, err := storage.Open(c.Project.DBPath)
	if err != nil {
		return err
	}
	c.DB = db
	return c.RequireDB()
}

// Close releases the database handle.
func (c *Context) Close() error {
	if c.DB == nil {
		return nil
	}
	return c.DB.Close()
}
