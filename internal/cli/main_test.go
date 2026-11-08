package cli

import (
	"os"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/analytics"
)

// TestMain disables missed-call telemetry for the whole package run.
//
// Every test here drives the real Execute() codepath, and most of them assert
// on failures on purpose (bad flags, unknown verbs, missing arguments). Without
// this gate each assertion writes a row into missed_cli_calls in whichever
// ledger the invocation resolved to, which is how ~90% of the recorded "misses"
// in the fleet came to be test fixtures rather than agent mistakes. Telemetry
// is the DX signal; polluting it from the test suite makes the signal unusable.
//
// The gate is a package-level flag rather than an os.Args[0] sniff: the test
// binary's own name is not a reliable signal, and a suite that has to reason
// about how it was built will eventually get it wrong.
func TestMain(m *testing.M) {
	analytics.Enabled = false
	// The env var covers the tests that shell out to a built binary, whose
	// process the in-process flag cannot reach.
	if err := os.Setenv(analytics.EnvTelemetry, "off"); err != nil {
		panic("cli tests: cannot disable telemetry: " + err.Error())
	}
	// Pin the model stamp too. The golden fixtures carry
	// "model_id": "unknown", so an ambient DEVDB_MODEL_ID — set by an agent
	// shell or a CI runner that exports it for real work — leaks into the
	// fixture and fails two M13 golden tests for a reason that has nothing to
	// do with the change under test. Same class of bug as the telemetry gate:
	// a suite must not depend on the host it happens to run on.
	if err := os.Unsetenv("DEVDB_MODEL_ID"); err != nil {
		panic("cli tests: cannot unset DEVDB_MODEL_ID: " + err.Error())
	}
	os.Exit(m.Run())
}

// TestTelemetryDisabledForSuite pins the gate itself, so removing the TestMain
// line or the RecordMissedCall early-return fails here instead of quietly
// resuming the pollution.
func TestTelemetryDisabledForSuite(t *testing.T) {
	if analytics.Enabled {
		t.Fatal("analytics.Enabled is true under go test; the suite would write missed_cli_calls rows")
	}
}
