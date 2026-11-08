package hub_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// readableProject returns a project root with a real Go-schema development.db
// in place. setupAcrossProject only creates the .devdb directory, which leaves
// Exists=false and makes every project read as unreadable — the assertion
// below needs one project that genuinely can be opened.
func readableProject(t *testing.T, dir, name string) string {
	t.Helper()
	root := setupAcrossProject(t, dir, name)
	db, path := testutil.TempDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	moveDB(t, path, root)
	return root
}

// An unreadable ledger must be named, not silently folded into empty counts.
// Before this, a project whose .devdb could not be opened was recorded with
// empty counts — indistinguishable from a project with nothing in it, so 8
// unreadable ledgers made the fleet "retained rows (live=432)" total read as
// complete. Same class of bug as the unregistered-repo blind spot that
// hub doctor --scan was built to expose.
//
// The setup is deliberately the production failure mode: register a project
// that exists, then remove its root before auditing. That is what a stale
// registry entry or a moved checkout looks like.
func TestAuditNamesUnreadableProjects(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	meta := filepath.Join(dir, "meta.db")

	root := readableProject(t, dir, "readable")
	if _, err := hub.Register(root, "readable", registry, meta); err != nil {
		t.Fatal(err)
	}

	stale := readableProject(t, dir, "stale")
	if _, err := hub.Register(stale, "stale", registry, meta); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stale); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{Registry: registry, Mode: "live"})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}

	if report.Unreadable["stale"] == "" {
		t.Fatalf("Unreadable = %v, want a reason for 'stale'", report.Unreadable)
	}
	if !strings.Contains(report.Unreadable["stale"], "does not exist") {
		t.Errorf("reason for 'stale' = %q, want it to say the path does not exist",
			report.Unreadable["stale"])
	}
	if _, bad := report.Unreadable["readable"]; bad {
		t.Errorf("'readable' is listed unreadable: %q", report.Unreadable["readable"])
	}
	// The readable project still reports data, so 'stale' having empty counts
	// genuinely means "could not look" rather than "nothing here".
	if len(report.ByProject["readable"]) == 0 {
		t.Errorf("readable project has no counts: %v", report.ByProject["readable"])
	}
}

// The map must stay empty when every project is readable, so JSON consumers
// see the field absent rather than null.
func TestAuditUnreadableEmptyWhenAllReadable(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := readableProject(t, dir, "solo")

	if _, err := hub.Register(root, "solo", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{Registry: registry, Mode: "live"})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(report.Unreadable) != 0 {
		t.Fatalf("Unreadable = %v, want empty", report.Unreadable)
	}
	if len(report.ByProject["solo"]) == 0 {
		t.Errorf("readable project has no counts: %v", report.ByProject["solo"])
	}
}

// The field must survive JSON marshalling with the name consumers will read.
// A struct field no marshaller emits is the same bug as not having it.
func TestAuditUnreadableJSONFieldName(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	root := readableProject(t, dir, "jsonproj")

	if _, err := hub.Register(root, "jsonproj", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	report, err := hub.Audit(hub.AuditOptions{Registry: registry, Mode: "live"})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	blob, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), `"unreadable"`) {
		t.Errorf("clean report should omit the field (omitempty), got: %s", blob)
	}
}
