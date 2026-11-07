package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LockHolder describes a process that currently has the ledger database
// (or its WAL/SHM sidecars) open. Used by `devdb doctor` to name the
// culprit when a write fails with a persistent SQLITE_BUSY, instead of
// leaving agents to guess which parallel process holds the lock.
type LockHolder struct {
	PID     int    `json:"pid"`
	Cmdline string `json:"cmdline"`
	File    string `json:"file"`
}

// ProbeWriteLock attempts a short write transaction against dbPath and
// returns nil when the ledger is writable. A returned error wrapping a
// busy condition means another process holds the write lock; callers can
// then use LockHolders to name it.
func ProbeWriteLock(dbPath string, timeout time.Duration) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	// Short busy_timeout: the probe should fail fast, not wait 5s.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", timeout.Milliseconds())); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	// Touch a row so the transaction actually takes the write lock.
	if _, err := tx.Exec("CREATE TABLE IF NOT EXISTS _lock_probe (x INTEGER)"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Rollback(); err != nil {
		return err
	}
	return nil
}

// LockHolders scans /proc for processes with the database file (or its
// -wal/-shm sidecars) open. Linux-only; returns nil on other platforms
// or when /proc is unavailable. Best-effort: unreadable /proc entries
// are skipped silently.
func LockHolders(dbPath string) []LockHolder {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil
	}
	targets := map[string]bool{
		abs:          true,
		abs + "-wal": true,
		abs + "-shm": true,
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil // not Linux or /proc unavailable
	}
	var holders []LockHolder
	self := os.Getpid()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(e.Name(), "%d", &pid); err != nil || pid == self {
			continue
		}
		fds, err := os.ReadDir(filepath.Join("/proc", e.Name(), "fd"))
		if err != nil {
			continue // process exited or no permission
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join("/proc", e.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			if !targets[link] {
				continue
			}
			cmdline, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
			cmd := strings.ReplaceAll(strings.TrimRight(string(cmdline), "\x00"), "\x00", " ")
			holders = append(holders, LockHolder{PID: pid, Cmdline: strings.TrimSpace(cmd), File: filepath.Base(link)})
			break // one match per process is enough
		}
	}
	return holders
}
