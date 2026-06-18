package hub

import (
	"os"
	"path/filepath"
)

const (
	defaultMetadataDir = ".devdb"
	defaultMetadataDB  = "metadata.db"
	defaultRegistry    = ".devdb-projects"
)

// ResolveMetadataDB returns the hub database path from flag, env, or default.
func ResolveMetadataDB(flag string) string {
	if flag != "" {
		return flag
	}
	if v := os.Getenv("DEVDB_METADATA_DB"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(defaultMetadataDir, defaultMetadataDB)
	}
	return filepath.Join(home, defaultMetadataDir, defaultMetadataDB)
}

// ResolveRegistry returns the project registry path from flag or default.
func ResolveRegistry(flag string) string {
	if flag != "" {
		return flag
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultRegistry
	}
	return filepath.Join(home, defaultRegistry)
}
