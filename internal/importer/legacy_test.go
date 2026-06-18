package importer_test

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func legacyPythonDB(t *testing.T) string {
	t.Helper()
	return testutil.LegacyPythonDBPath(t)
}
