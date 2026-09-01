package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen_CreatesSchema(t *testing.T) {
	s := openTestStore(t)

	tables := []string{"reads", "threads", "prefs"}
	for _, table := range tables {
		var name string
		err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestIsUnread_NoRowIsUnread(t *testing.T) {
	s := openTestStore(t)

	unread, err := s.IsUnread("operator", "some-hold")
	if err != nil {
		t.Fatalf("IsUnread returned error: %v", err)
	}
	if !unread {
		t.Error("expected hold with no reads row to be unread")
	}
}

func TestMarkRead_ThenIsRead(t *testing.T) {
	s := openTestStore(t)

	if err := s.MarkRead("operator", "some-hold", time.Now()); err != nil {
		t.Fatalf("MarkRead returned error: %v", err)
	}

	unread, err := s.IsUnread("operator", "some-hold")
	if err != nil {
		t.Fatalf("IsUnread returned error: %v", err)
	}
	if unread {
		t.Error("expected hold to be read after MarkRead")
	}
}

func TestDifferentHoldIDIsUnreadEvenIfOldWasRead(t *testing.T) {
	s := openTestStore(t)

	if err := s.MarkRead("operator", "hold-abc-oldsha", time.Now()); err != nil {
		t.Fatalf("MarkRead returned error: %v", err)
	}

	unread, err := s.IsUnread("operator", "hold-abc-newsha")
	if err != nil {
		t.Fatalf("IsUnread returned error: %v", err)
	}
	if !unread {
		t.Error("a new hold id (new head_sha) must be unread even though the old id was read")
	}
}

func TestToggleRead_FlipsState(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()

	if err := s.ToggleRead("operator", "some-hold", now); err != nil {
		t.Fatalf("ToggleRead (1st) returned error: %v", err)
	}
	unread, err := s.IsUnread("operator", "some-hold")
	if err != nil {
		t.Fatalf("IsUnread returned error: %v", err)
	}
	if unread {
		t.Error("expected read after first toggle from initial unread state")
	}

	if err := s.ToggleRead("operator", "some-hold", now); err != nil {
		t.Fatalf("ToggleRead (2nd) returned error: %v", err)
	}
	unread, err = s.IsUnread("operator", "some-hold")
	if err != nil {
		t.Fatalf("IsUnread returned error: %v", err)
	}
	if !unread {
		t.Error("expected unread after second toggle")
	}
}

func TestUsersHaveIndependentReadState(t *testing.T) {
	s := openTestStore(t)

	if err := s.MarkRead("alice", "some-hold", time.Now()); err != nil {
		t.Fatalf("MarkRead returned error: %v", err)
	}

	unread, err := s.IsUnread("bob", "some-hold")
	if err != nil {
		t.Fatalf("IsUnread returned error: %v", err)
	}
	if !unread {
		t.Error("bob's read state must be independent of alice's")
	}
}
