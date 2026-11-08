package hub_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
)

// hermeticConfigPath is a path that never exists. Pointing DEVDB_CONFIG at it
// makes configSearchPaths return exactly one unreadable candidate, so the hub
// resolves to the $HOME defaults regardless of what /etc/devdb/config.json says
// on the machine running the tests.
var hermeticConfigPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "devdb-hub-hermetic")
	if err != nil {
		panic("hub tests: cannot create temp dir: " + err.Error())
	}
	hermeticConfigPath = filepath.Join(dir, "absent-config.json")

	for k, v := range map[string]string{
		hub.EnvConfig:      hermeticConfigPath,
		"XDG_CONFIG_HOME":  "",
		hub.EnvMetadataDB:  "",
		hub.EnvRegistry:    "",
		"DEVDB_MODEL_ID":   "",
		"DEVDB_AGENT_ROLE": "",
		"DEVDB_SESSION_ID": "",
	} {
		if err := os.Setenv(k, v); err != nil {
			panic("hub tests: cannot set " + k + ": " + err.Error())
		}
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// TestConfigSearchPathsIncludesSystemPathLowest asserts the production search
// order keeps /etc last, so a per-user or XDG config still wins on a host that
// ships a system-wide config. This is the invariant the $HOME-default tests
// depend on, made explicit instead of implied.
func TestConfigSearchPathsIncludesSystemPathLowest(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	home := filepath.Join(t.TempDir(), "home")
	sys := filepath.Join(string(filepath.Separator), "etc", "devdb", "config.json")

	got := hub.ConfigSearchPathsForTest(xdg, home, sys)
	if len(got) != 3 {
		t.Fatalf("paths=%v want 3 entries", got)
	}
	if want := filepath.Join(xdg, "devdb", "config.json"); got[0] != want {
		t.Fatalf("paths[0]=%q want %q", got[0], want)
	}
	if want := filepath.Join(home, ".config", "devdb", "config.json"); got[1] != want {
		t.Fatalf("paths[1]=%q want %q", got[1], want)
	}
	if got[2] != sys {
		t.Fatalf("paths[2]=%q want system config %q", got[2], sys)
	}
}

// TestConfigSearchPathsOmitsEmptyRoots covers the seam's contract: an empty
// root drops that candidate rather than producing a bogus path.
func TestConfigSearchPathsOmitsEmptyRoots(t *testing.T) {
	if got := hub.ConfigSearchPathsForTest("", "", ""); len(got) != 0 {
		t.Fatalf("paths=%v want empty", got)
	}
	if got := hub.ConfigSearchPathsForTest("", "", "/etc/devdb/config.json"); len(got) != 1 {
		t.Fatalf("paths=%v want only the system entry", got)
	}
}
