package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// busyErr is a test double that mimics modernc.org/sqlite Error's Code()
// method so isBusyError can recognize it without the test importing the
// driver type directly.
type busyErr struct {
	code int
}

func (e *busyErr) Error() string { return fmt.Sprintf("synthetic sqlite error code=%d", e.code) }
func (e *busyErr) Code() int     { return e.code }

func TestOpenAndWithTx(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := WithTx(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE tx_test (id INTEGER)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='tx_test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("commit failed: n=%d err=%v", n, err)
	}

	err = WithTx(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE tx_rollback (id INTEGER)`)
		if err != nil {
			return err
		}
		return errors.New("rollback me")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='tx_rollback'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rollback failed: n=%d err=%v", n, err)
	}
}

func TestNowUTC(t *testing.T) {
	ts := NowUTC()
	if ts == "" {
		t.Fatal("empty timestamp")
	}
}

func TestIsBusyErrorRecognizesBusyAndLockedCodes(t *testing.T) {
	t.Run("SQLITE_BUSY via coded error", func(t *testing.T) {
		if !isBusyError(&busyErr{code: 5}) {
			t.Fatal("expected isBusyError=true for code 5")
		}
	})
	t.Run("SQLITE_LOCKED via coded error", func(t *testing.T) {
		if !isBusyError(&busyErr{code: 6}) {
			t.Fatal("expected isBusyError=true for code 6")
		}
	})
	t.Run("other code is not busy", func(t *testing.T) {
		if isBusyError(&busyErr{code: 19}) {
			t.Fatal("expected isBusyError=false for code 19")
		}
	})
	t.Run("string fallback matches wrapped messages", func(t *testing.T) {
		if !isBusyError(errors.New("PRAGMA journal_mode = WAL: database is locked (5) (SQLITE_BUSY)")) {
			t.Fatal("expected isBusyError=true for wrapped busy message")
		}
	})
	t.Run("unrelated error is not busy", func(t *testing.T) {
		if isBusyError(errors.New("constraint failed")) {
			t.Fatal("expected isBusyError=false for constraint error")
		}
	})
	t.Run("nil is not busy", func(t *testing.T) {
		if isBusyError(nil) {
			t.Fatal("expected isBusyError=false for nil")
		}
	})
	t.Run("wrapped coded error is still busy", func(t *testing.T) {
		wrapped := fmt.Errorf("while opening db: %w", &busyErr{code: 5})
		if !isBusyError(wrapped) {
			t.Fatal("expected isBusyError=true for wrapped busy code")
		}
	})
}

func TestWithBusyRetryRetriesBusyThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	want := errors.New("boom")
	err := withBusyRetry(func() error {
		if calls.Add(1) <= 2 {
			return &busyErr{code: 5}
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped sentinel %v, got %v", want, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestWithBusyRetryDoesNotRetryNonBusy(t *testing.T) {
	var calls atomic.Int32
	want := errors.New("constraint failed")
	err := withBusyRetry(func() error {
		calls.Add(1)
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected sentinel %v, got %v", want, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls.Load())
	}
}

func TestWithBusyRetryExhaustsAttempts(t *testing.T) {
	var calls atomic.Int32
	busy := &busyErr{code: 5}
	err := withBusyRetry(func() error {
		calls.Add(1)
		return busy
	})
	if !isBusyError(err) {
		t.Fatalf("expected busy error after exhaustion, got %v", err)
	}
	if calls.Load() != int32(busyRetryAttempts) {
		t.Fatalf("expected %d calls, got %d", busyRetryAttempts, calls.Load())
	}
}

// TestWithTxRetriesBusyTransaction wraps WithTx around a function that fails
// with SQLITE_BUSY on its first call and succeeds on its second; the second
// attempt must observe an empty database (rollback from the first attempt),
// proving the retry does not double-apply the side effect.
func TestWithTxRetriesBusyTransaction(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE counter (n INTEGER)`); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	err = WithTx(db, func(tx *sql.Tx) error {
		if calls.Add(1) == 1 {
			return &busyErr{code: 5}
		}
		_, err := tx.Exec(`INSERT INTO counter(n) VALUES (1)`)
		return err
	})
	if err != nil {
		t.Fatalf("WithTx should have retried and succeeded: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", calls.Load())
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM counter`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("counter=%d err=%v (retry must roll back the first attempt)", n, err)
	}
}

// TestConcurrentWritesDoNotError simulates close-together writes against a
// single database (the scenario from feedback 06c27df5). All writers must
// either succeed or surface a controlled final error — no panics, no
// orphaned transactions.
func TestConcurrentWritesDoNotError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE writes (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT)`); err != nil {
		t.Fatal(err)
	}

	const goroutines = 8
	done := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			done <- WithTx(db, func(tx *sql.Tx) error {
				_, err := tx.Exec(`INSERT INTO writes(body) VALUES (?)`, "writer-"+filepath.Base(dir))
				return err
			})
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		if err := <-done; err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM writes`).Scan(&rows); err != nil || rows != goroutines {
		t.Fatalf("rows=%d err=%v (want %d)", rows, err, goroutines)
	}
}

// TestJitterBusyDelaySpreads verifies that retry sleeps are not deterministic
// — without jitter, three parallel processes retry in lockstep and exhaust
// the budget together (the failure mode behind feedback bbec89e6).
func TestJitterBusyDelaySpreads(t *testing.T) {
	base := 100 * time.Millisecond
	seen := map[time.Duration]bool{}
	for range 50 {
		seen[jitterBusyDelay(base)] = true
	}
	// 50 samples should yield many distinct delays. If jitter is missing,
	// every sample lands on the same value and len(seen)==1.
	if len(seen) < 5 {
		t.Fatalf("jitter not applied: %d distinct delays out of 50", len(seen))
	}
	// Sanity: jitter must keep each sample inside [base/2, 3*base/2).
	for d := range seen {
		if d < base/2 || d >= base+base/2 {
			t.Fatalf("jitter out of bounds: base=%v sample=%v", base, d)
		}
	}
}

// TestParallelSubprocessesContendForDB simulates the cross-process failure
// mode from feedback bbec89e6: three devdb CLI invocations write to the
// same development.db at the same time, contending on PRAGMA setup and
// transaction commit. With retry + jitter all writers must eventually
// succeed; without them at least one would surface SQLITE_BUSY.
//
// The test spawns the test binary as a subprocess (one per writer) so each
// process holds its own *sql.DB and contends on the file lock — the same
// shape as parallel `devdb plan acceptance meet` invocations.
func TestParallelSubprocessesContendForDB(t *testing.T) {
	if os.Getenv("DEVDB_PARALLEL_HARNESS") == "1" {
		runParallelWriteHarness(t)
		return
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shared.db")
	seed, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(`CREATE TABLE writes (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT)`); err != nil {
		seed.Close()
		t.Fatal(err)
	}
	seed.Close()

	const writers = 3
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestParallelSubprocessesContendForDB$")
			cmd.Env = append(os.Environ(),
				"DEVDB_PARALLEL_HARNESS=1",
				fmt.Sprintf("DEVDB_PARALLEL_DB=%s", dbPath),
				fmt.Sprintf("DEVDB_PARALLEL_WRITER=%d", n),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs[n] = fmt.Errorf("writer %d: %v\n%s", n, err, out)
			}
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	verify, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var rows int
	if err := verify.QueryRow(`SELECT COUNT(*) FROM writes`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != writers {
		t.Fatalf("rows=%d want %d (one per subprocess)", rows, writers)
	}
}

func runParallelWriteHarness(t *testing.T) {
	dbPath := os.Getenv("DEVDB_PARALLEL_DB")
	writer := os.Getenv("DEVDB_PARALLEL_WRITER")
	if dbPath == "" {
		t.Fatal("DEVDB_PARALLEL_DB unset")
	}
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := WithTx(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO writes(body) VALUES (?)`, fmt.Sprintf("writer-%s", writer))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
