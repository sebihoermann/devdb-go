package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	defaultMetadataDir = ".devdb"
	defaultMetadataDB  = "metadata.db"
	defaultRegistry    = ".devdb-projects"

	// EnvMetadataDB and EnvRegistry override the compiled defaults for the hub
	// database and the project registry respectively.
	EnvMetadataDB = "DEVDB_METADATA_DB"
	EnvRegistry   = "DEVDB_REGISTRY"
	// EnvConfig names an explicit config file, bypassing the search order.
	EnvConfig = "DEVDB_CONFIG"
)

// FileConfig is the on-disk devdb config (JSON). Both fields are optional;
// omitted fields fall through to env, then to the $HOME defaults.
type FileConfig struct {
	// MetadataDB is the hub metadata database path.
	MetadataDB string `json:"metadata_db,omitempty"`
	// Registry is the project registry file path.
	Registry string `json:"registry,omitempty"`
}

// systemConfigFile is the lowest-priority config location. It is the last
// candidate so a per-user config can point at a shared hub without editing /etc.
func systemConfigFile() string {
	return filepath.Join(string(filepath.Separator), "etc", "devdb", "config.json")
}

// ConfigSearchPathsForTest returns the config search order computed from
// explicit roots rather than the ambient environment, so the order (and the
// presence of the system path) is verifiable on any host. An empty root omits
// that candidate; production order comes from configSearchPaths.
func ConfigSearchPathsForTest(xdg, home, sys string) []string {
	var paths []string
	if xdg != "" {
		paths = append(paths, filepath.Join(xdg, "devdb", "config.json"))
	}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".config", "devdb", "config.json"))
	}
	if sys != "" {
		paths = append(paths, sys)
	}
	return paths
}

// configSearchPaths returns the candidate config files in priority order.
//
// Order matters: a per-user config beats the system config so a single user can
// point at a neutral shared hub without editing /etc. DEVDB_CONFIG bypasses the
// search order entirely: when set it is the only candidate, which is what keeps
// probes and tests hermetic on a host that has a real system config.
func configSearchPaths() []string {
	if v := os.Getenv(EnvConfig); v != "" {
		return []string{v}
	}
	home, _ := os.UserHomeDir()
	return ConfigSearchPathsForTest(os.Getenv("XDG_CONFIG_HOME"), home, systemConfigFile())
}

// LoadFileConfig reads the first readable config file found. A missing file is
// not an error (returns a zero FileConfig); a malformed file is.
func LoadFileConfig() (FileConfig, error) {
	for _, p := range configSearchPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) {
				continue
			}
			return FileConfig{}, err
		}
		var cfg FileConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return FileConfig{}, err
		}
		return cfg, nil
	}
	return FileConfig{}, nil
}

// resolveHubPath applies the shared precedence: explicit flag, env var,
// config file value, then the caller-supplied default.
func resolveHubPath(flag, envName, configValue string) string {
	if flag != "" {
		return expandHome(flag)
	}
	if v := os.Getenv(envName); v != "" {
		return expandHome(v)
	}
	if configValue != "" {
		return expandHome(configValue)
	}
	return ""
}

// ResolveMetadataDB returns the hub database path from flag, env, config, or default.
func ResolveMetadataDB(flag string) string {
	cfg, _ := LoadFileConfig()
	if p := resolveHubPath(flag, EnvMetadataDB, cfg.MetadataDB); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(defaultMetadataDir, defaultMetadataDB)
	}
	return filepath.Join(home, defaultMetadataDir, defaultMetadataDB)
}

// ResolveRegistry returns the project registry path from flag, env, config, or default.
func ResolveRegistry(flag string) string {
	cfg, _ := LoadFileConfig()
	if p := resolveHubPath(flag, EnvRegistry, cfg.Registry); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultRegistry
	}
	return filepath.Join(home, defaultRegistry)
}
