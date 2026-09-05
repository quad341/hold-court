// Package store persists discussion threads, read state, and preferences in
// a single SQLite file (DESIGN.md "Discussion + read state"). It uses a
// pure-Go driver: no cgo, so hold-court stays a single static binary.
package store

import (
	"database/sql"
	"encoding/json"
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
CREATE TABLE IF NOT EXISTS hold_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	hold_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	revision TEXT NOT NULL,
	at TEXT NOT NULL,
	data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS hold_history_lookup ON hold_history(hold_id, kind, id);
`

// Store wraps the SQLite database backing reads/threads/prefs.
type Store struct {
	db *sql.DB
}

// HistoryEntry is an immutable observed review, decision, or consumer result.
type HistoryEntry struct {
	ID   int64           `json:"id"`
	Kind string          `json:"kind"`
	At   string          `json:"at"`
	Data json.RawMessage `json:"data"`
}

// Observe records a component only when its content changes. A repeated poll
// does not create an event, while reverting to an earlier version does.
func (s *Store) Observe(holdID, kind, revision string, data []byte) error {
	_, err := s.db.Exec(`INSERT INTO hold_history(hold_id,kind,revision,at,data)
		SELECT ?,?,?,?,? WHERE COALESCE((SELECT revision FROM hold_history
		WHERE hold_id=? AND kind=? ORDER BY id DESC LIMIT 1),'') != ?`,
		holdID, kind, revision, time.Now().UTC().Format(time.RFC3339Nano), string(data), holdID, kind, revision)
	return err
}

// History returns the complete observed history, newest first.
func (s *Store) History(holdID string) ([]HistoryEntry, error) {
	rows, err := s.db.Query(`SELECT id,kind,at,data FROM hold_history WHERE hold_id=? ORDER BY id DESC`, holdID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	entries := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		var data string
		if err := rows.Scan(&e.ID, &e.Kind, &e.At, &data); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		entries = append(entries, e)
	}
	return entries, rows.Err()
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

// ReadRevision returns the content revision last acknowledged by the user.
// Preferences keep this additive to existing read-state databases.
func (s *Store) ReadRevision(user, holdID string) (string, error) {
	var revision string
	err := s.db.QueryRow(`SELECT value FROM prefs WHERE user = ? AND key = ?`, user, "read-revision:"+holdID).Scan(&revision)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return revision, err
}

// MarkReadRevision records precisely the content the browser displayed. A
// concurrent new result must remain unread until the user sees that revision.
func (s *Store) MarkReadRevision(user, holdID, revision string, at time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO reads (user, hold_id, last_read_at) VALUES (?, ?, ?)
		ON CONFLICT (user, hold_id) DO UPDATE SET last_read_at = excluded.last_read_at`, user, holdID, at); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO prefs (user, key, value) VALUES (?, ?, ?)
		ON CONFLICT (user, key) DO UPDATE SET value = excluded.value`, user, "read-revision:"+holdID, revision); err != nil {
		return err
	}
	return tx.Commit()
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

// IncomingResultRevision retains the last observed consumer activity across a
// new queued decision, so sending a follow-up is not itself an incoming update.
func (s *Store) IncomingResultRevision(holdID string) (string, error) {
	var revision string
	err := s.db.QueryRow(`SELECT revision FROM hold_history
 WHERE hold_id = ? AND kind = 'result' AND json_extract(data, '$.status') != 'queued'
 ORDER BY id DESC LIMIT 1`, holdID).Scan(&revision)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return revision, err
}
