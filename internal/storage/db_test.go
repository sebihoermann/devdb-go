package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// TestOpenAppliesBusyTimeoutBeforeJournalMode guards the PRAGMA ordering in
// openPragmas. journal_mode = WAL needs a brief exclusive lock to convert the
// journal; if it runs before busy_timeout is set it acquires that lock with a
// zero timeout and concurrent devdb processes fail at Open instead of waiting.
// See plan item 445210c7 and the openPragmas doc comment.
func TestOpenAppliesBusyTimeoutBeforeJournalMode(t *testing.T) {
	index := func(needle string) int {
		for i, p := range openPragmas {
			if strings.Contains(p, needle) {
				return i
			}
		}
		t.Fatalf("openPragmas is missing %q; got %v", needle, openPragmas)
		return -1
	}

	busy := index("busy_timeout")
	journal := index("journal_mode")
	foreignKeys := index("foreign_keys")

	if busy > journal {
		t.Errorf("busy_timeout (index %d) must be applied before journal_mode (index %d); order is %v", busy, journal, openPragmas)
	}
	if foreignKeys < journal {
		t.Errorf("foreign_keys is lock-free and belongs last; order is %v", openPragmas)
	}
}

// TestOpenAppliesPragmasToConnection proves the pragmas actually landed on the
// pooled connection rather than being accepted and discarded. A PRAGMA applied
// to a different connection than the one the pool later hands out would pass an
// ordering test and still leave writes unprotected.
func TestOpenAppliesPragmasToConnection(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pragmas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var timeoutMS int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&timeoutMS); err != nil {
		t.Fatal(err)
	}
	if timeoutMS != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeoutMS)
	}

	var journal string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(journal, "wal") {
		t.Errorf("journal_mode = %q, want wal", journal)
	}

	var foreignKeys int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

// TestParallelColdOpenContendsForDB exercises contention on the first Open of a
// database file, where journal_mode is still the default and the conversion to
// WAL must take a brief exclusive lock. TestParallelSubprocessesContendForDB
// cannot reach that window: it seeds the database first, and seeding converts
// the journal to WAL, so every later Open finds WAL established and the
// journal_mode pragma becomes a no-op needing no lock.
//
// Four subprocesses are released from a shared wall-clock deadline so they
// attempt the cold Open at the same instant rather than staggered by process
// startup skew, and each creates the table and inserts inside one transaction.
//
// Measured caveat: as of 2026-10-02 this test does not reproduce the
// "PRAGMA journal_mode = WAL: database is locked" failure from feedback
// bbec89e6 even with the openPragmas order reverted to its pre-fix state
// (0 failures in 10 synchronised runs). withBusyRetry's jittered backoff
// appears to absorb the conversion window on this machine. The ordering is
// still objectively wrong and is locked by
// TestOpenAppliesBusyTimeoutBeforeJournalMode; this test guards cold-open
// robustness generally rather than reproducing the original flake.
func TestParallelColdOpenContendsForDB(t *testing.T) {
	if os.Getenv("DEVDB_COLD_HARNESS") == "1" {
		runColdOpenHarness()
		return
	}

	dir := t.TempDir()
	// Deliberately does not exist: no seed process, so journal_mode is still
	// the default and the first Open has to convert it.
	dbPath := filepath.Join(dir, "cold.db")

	const writers = 4
	deadline := time.Now().Add(3 * time.Second).UnixNano()

	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestParallelColdOpenContendsForDB$")
			cmd.Env = append(os.Environ(),
				"DEVDB_COLD_HARNESS=1",
				fmt.Sprintf("DEVDB_COLD_DB=%s", dbPath),
				fmt.Sprintf("DEVDB_COLD_WRITER=%d", n),
				fmt.Sprintf("DEVDB_COLD_DEADLINE_NS=%d", deadline),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs[n] = fmt.Errorf("cold writer %d: %v\n%s", n, err, out)
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
		t.Fatalf("rows=%d want %d (one per cold subprocess)", rows, writers)
	}
}

func runColdOpenHarness() {
	dbPath := os.Getenv("DEVDB_COLD_DB")
	if dbPath == "" {
		fmt.Fprintln(os.Stderr, "DEVDB_COLD_DB unset")
		os.Exit(2)
	}
	if raw := os.Getenv("DEVDB_COLD_DEADLINE_NS"); raw != "" {
		if deadline, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if wait := time.Unix(0, deadline).Sub(time.Now()); wait > 0 {
				time.Sleep(wait)
			}
		}
	}

	db, err := Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(3)
	}
	defer db.Close()

	// The table is created here rather than seeded, so CREATE TABLE is part of
	// the contended work. IF NOT EXISTS keeps it safe against the other writers.
	if err := WithTx(db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS writes (id INTEGER PRIMARY KEY AUTOINCREMENT, body TEXT)`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO writes(body) VALUES (?)`, fmt.Sprintf("cold-%s", os.Getenv("DEVDB_COLD_WRITER")))
		return err
	}); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(4)
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
