// Package store persists discussion threads, read state, and preferences in
// a single SQLite file (DESIGN.md "Discussion + read state"). It uses a
// pure-Go driver: no cgo, so hold-court stays a single static binary.
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver with database/sql
)

const schema = `
CREATE TABLE IF NOT EXISTS threads (
	hold_id TEXT NOT NULL,
	author  TEXT NOT NULL,
	body_md TEXT NOT NULL,
	at      TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS reads (
	user         TEXT NOT NULL,
	hold_id      TEXT NOT NULL,
	last_read_at TIMESTAMP NOT NULL,
	PRIMARY KEY (user, hold_id)
);
CREATE TABLE IF NOT EXISTS prefs (
	user  TEXT NOT NULL,
	key   TEXT NOT NULL,
	value TEXT NOT NULL,
	PRIMARY KEY (user, key)
);
`

// Store wraps the SQLite database backing reads/threads/prefs.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite file at path and ensures the
// schema exists.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // modernc.org/sqlite: single-writer file, avoid SQLITE_BUSY

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close() // best-effort: we're already returning the schema error
		return nil, fmt.Errorf("store: create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// IsUnread reports whether hold_id has never been marked read by user: a
// hold is unread until a reads row exists for it, per DESIGN.md's email-like
// unread semantics.
func (s *Store) IsUnread(user, holdID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reads WHERE user = ? AND hold_id = ?`,
		user, holdID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("store: is unread: %w", err)
	}
	return count == 0, nil
}

// MarkRead records that user has read holdID as of at.
func (s *Store) MarkRead(user, holdID string, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO reads (user, hold_id, last_read_at) VALUES (?, ?, ?)
		 ON CONFLICT (user, hold_id) DO UPDATE SET last_read_at = excluded.last_read_at`,
		user, holdID, at,
	)
	if err != nil {
		return fmt.Errorf("store: mark read: %w", err)
	}
	return nil
}

// MarkUnread clears any read record, making holdID unread again for user.
func (s *Store) MarkUnread(user, holdID string) error {
	_, err := s.db.Exec(`DELETE FROM reads WHERE user = ? AND hold_id = ?`, user, holdID)
	if err != nil {
		return fmt.Errorf("store: mark unread: %w", err)
	}
	return nil
}

// ToggleRead flips holdID between read and unread for user (the `u` key).
func (s *Store) ToggleRead(user, holdID string, at time.Time) error {
	unread, err := s.IsUnread(user, holdID)
	if err != nil {
		return err
	}
	if unread {
		return s.MarkRead(user, holdID, at)
	}
	return s.MarkUnread(user, holdID)
}
