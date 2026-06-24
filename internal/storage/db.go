package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	_ "modernc.org/sqlite" // driver registration for database/sql
)

const (
	busyRetryAttempts  = 6
	busyRetryBaseDelay = 50 * time.Millisecond
	busyRetryMaxDelay  = 500 * time.Millisecond
)

// jitterBusyDelay spreads each retry sleep across [base/2, base+base/2] so
// contending processes do not retry in lockstep. Without jitter, three
// parallel `devdb plan acceptance meet` invocations collide on every PRAGMA
// or commit and exhaust the retry budget together.
func jitterBusyDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	return base/2 + time.Duration(rand.Int63n(int64(base)))
}

// busyCoder is the optional interface satisfied by errors that carry a
// SQLite result code. The modernc.org/sqlite driver exposes this via
// *sqlite.Error; tests inject their own implementations without taking a
// hard dependency on the driver type.
type busyCoder interface {
	Code() int
}

// isBusyError reports whether err is a transient SQLite lock error
// (SQLITE_BUSY code 5 or SQLITE_LOCKED code 6). Used by withBusyRetry to
// decide whether to retry; non-lock errors return immediately so callers
// do not get silently retried constraint violations.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	var c busyCoder
	if errors.As(err, &c) {
		return c.Code() == 5 || c.Code() == 6
	}
	// Fallback when wrapping hides the typed error or driver versions
	// surface the lock condition only through the message string.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked")
}

// withBusyRetry runs fn with bounded exponential backoff when fn returns a
// transient SQLite lock error. Other errors return immediately so callers
// do not get silently retried constraint violations. After
// busyRetryAttempts attempts the last error is returned, wrapped so callers
// can still match on errors.As. Sleep durations are jittered so contending
// processes do not retry in lockstep and exhaust the budget together.
//
// The retry covers the whole fn call, so any caller that wraps a transaction
// body (begin + fn(tx) + commit) gets commit-window retries for free. This
// is how `Open` covers PRAGMA setup and `WithTx` covers transaction commit.
func withBusyRetry(fn func() error) error {
	var err error
	delay := busyRetryBaseDelay
	for attempt := 0; attempt < busyRetryAttempts; attempt++ {
		err = fn()
		if !isBusyError(err) {
			return err
		}
		if attempt == busyRetryAttempts-1 {
			break
		}
		time.Sleep(jitterBusyDelay(delay))
		delay *= 2
		if delay > busyRetryMaxDelay {
			delay = busyRetryMaxDelay
		}
	}
	return fmt.Errorf("storage: SQLITE_BUSY after %d attempts: %w", busyRetryAttempts, err)
}

// Open connects to a SQLite database with devdb pragmas applied.
// Transient SQLITE_BUSY from PRAGMA setup is retried with bounded backoff
// (and jitter) so concurrent agent invocations do not require a manual retry.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if err := withBusyRetry(func() error {
			_, err := db.Exec(p)
			return err
		}); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	return db, nil
}

// WithTx runs fn inside a transaction with SQLITE_BUSY retry, rolling back
// on error. The whole transaction (Begin + fn + Commit) is retried on
// transient locks with bounded exponential backoff plus jitter, so callers
// do not duplicate side effects and contending processes do not retry in
// lockstep; if fn returns a non-busy error the retry loop exits immediately.
// Commit failures are caught by the same retry, so this helper covers the
// transaction commit window, not just PRAGMA setup.
func WithTx(db *sql.DB, fn func(*sql.Tx) error) error {
	return withBusyRetry(func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// NowUTC returns an RFC3339 timestamp in UTC.
func NowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
