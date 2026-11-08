package storage_test

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// seedItem inserts the plan_items parent the acceptance FK requires.
func seedItem(t *testing.T, db *sql.DB, itemID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO plan_items(id, title, status, created_at)
		VALUES (?, 'parallel meet item', 'planned', '2026-01-01T00:00:00Z')`, itemID); err != nil {
		t.Fatal(err)
	}
}

// TestConcurrentAcceptanceMeetsNeverBusy is the regression for the one write
// path that skipped the retry: MeetAcceptance ran a bare Exec, so eight agents
// closing criteria at once hit SQLITE_BUSY while a single agent never did. The
// old workaround was a documented rule to meet criteria sequentially — a
// property of the bug, not of the data.
//
// Each meeter gets its own connection to the same file, because contention
// between goroutines sharing one pooled handle is not the failure users hit.
func TestConcurrentAcceptanceMeetsNeverBusy(t *testing.T) {
	seed, path := testutil.TempDB(t)
	seedItem(t, seed, "item-parallel")

	const meeters = 16
	ids := make([]string, meeters)
	for i := 0; i < meeters; i++ {
		id := fmt.Sprintf("acc-parallel-%02d", i)
		if _, err := seed.Exec(`
			INSERT INTO plan_item_acceptance(id, plan_item_id, criterion, status, created_at, updated_at, ordinal)
			VALUES (?, 'item-parallel', ?, 'open', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', ?)`,
			id, fmt.Sprintf("criterion %d", i), i+1); err != nil {
			t.Fatal(err)
		}
		ids[i] = id
	}

	var wg sync.WaitGroup
	errs := make([]error, meeters)
	start := make(chan struct{})
	for i := 0; i < meeters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := storage.Open(path)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = db.Close() }()
			// Disable the driver's own lock wait so a collision surfaces
			// immediately: the test then proves the retry loop, not the
			// 5s busy_timeout that would otherwise mask a missing retry.
			if _, err := db.Exec("PRAGMA busy_timeout = 0"); err != nil {
				errs[i] = err
				return
			}
			<-start // release together to maximise contention
			_, errs[i] = planning.MeetAcceptance(db, ids[i], "parallel evidence", "test")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			if storage.IsBusy(err) {
				t.Fatalf("meeter %d hit SQLITE_BUSY after retries: %v", i, err)
			}
			t.Fatalf("meeter %d failed: %v", i, err)
		}
	}

	var met int
	if err := seed.QueryRow(`
		SELECT COUNT(*) FROM plan_item_acceptance
		WHERE plan_item_id='item-parallel' AND status='met'`).Scan(&met); err != nil {
		t.Fatal(err)
	}
	if met != meeters {
		t.Fatalf("%d of %d criteria marked met", met, meeters)
	}

	// Evidence must survive, not just the status.
	var withEvidence int
	if err := seed.QueryRow(`
		SELECT COUNT(*) FROM plan_item_acceptance
		WHERE plan_item_id='item-parallel' AND evidence='parallel evidence'`).Scan(&withEvidence); err != nil {
		t.Fatal(err)
	}
	if withEvidence != meeters {
		t.Fatalf("%d of %d criteria recorded evidence", withEvidence, meeters)
	}
}

// TestConcurrentMeetsOnSameRowNeverBusy covers the single-row case: eight
// agents meeting one criterion must all succeed rather than lock each other
// out, and the row must end up met.
func TestConcurrentMeetsOnSameRowNeverBusy(t *testing.T) {
	seed, path := testutil.TempDB(t)
	seedItem(t, seed, "item-shared")
	if _, err := seed.Exec(`
		INSERT INTO plan_item_acceptance(id, plan_item_id, criterion, status, created_at, updated_at, ordinal)
		VALUES ('acc-shared', 'item-shared', 'one criterion', 'open', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 1)`); err != nil {
		t.Fatal(err)
	}

	const meeters = 16
	var wg sync.WaitGroup
	errs := make([]error, meeters)
	start := make(chan struct{})
	for i := 0; i < meeters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := storage.Open(path)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = db.Close() }()
			<-start
			_, errs[i] = planning.MeetAcceptance(db, "acc-shared", fmt.Sprintf("evidence from %d", i), "test")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			if storage.IsBusy(err) {
				t.Fatalf("meeter %d hit SQLITE_BUSY after retries: %v", i, err)
			}
			t.Fatalf("meeter %d failed: %v", i, err)
		}
	}
	var status string
	if err := seed.QueryRow(`SELECT status FROM plan_item_acceptance WHERE id='acc-shared'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "met" {
		t.Fatalf("status = %q, want met", status)
	}
}
