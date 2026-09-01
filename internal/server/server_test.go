package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/quad341/hold-court/internal/store"
)

var wantKeybindings = []KeyBinding{
	{"j / k", "next / previous hold in list"},
	{"gg / G", "first / last hold"},
	{"Ctrl-d / Ctrl-u", "half-page scroll in reading pane"},
	{"Enter or l", "open selected hold (focus reading pane)"},
	{"h", "back to list"},
	{"Tab / Shift-Tab", "cycle folders"},
	{"/", "filter/search holds; n/N next/prev match"},
	{"p", "rule: proceed"},
	{"c", "rule: request changes"},
	{"x", "rule: close"},
	{"d", "rule: discuss"},
	{"i", "annotate (note field; Esc returns to normal mode)"},
	{"u", "toggle read/unread"},
	{"o", "open PR on GitHub"},
	{"s", "save pending rulings"},
	{"?", "key cheatsheet overlay"},
}

func TestKeybindings_MatchesDesignSpec(t *testing.T) {
	if !reflect.DeepEqual(Keybindings, wantKeybindings) {
		t.Errorf("Keybindings does not match DESIGN.md spec table (non-negotiable).\ngot:  %+v\nwant: %+v", Keybindings, wantKeybindings)
	}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	feedDir := t.TempDir()
	holdJSON := `{
  "id": "gastownhall-gascity-5795-a1b2c3",
  "source": "maintainer-pr-review",
  "repo": "gastownhall/gascity",
  "pr": 5795,
  "url": "https://github.com/gastownhall/gascity/pull/5795",
  "class": "ambiguous-needs-discussion",
  "title": "Push-tier relaxation",
  "question": "Should the push tier guard relax for release branches?",
  "review_body_md": "The guard currently blocks all force pushes.",
  "verdict": "fix-merge",
  "head_sha": "abc123",
  "held_at": "2026-09-01T15:00:00Z",
  "resolved": false,
  "resolved_reason": ""
}`
	if err := os.WriteFile(filepath.Join(feedDir, "hold1.json"), []byte(holdJSON), 0o644); err != nil {
		t.Fatalf("write feed fixture: %v", err)
	}

	rulingsDir := t.TempDir()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	h, err := New(Config{
		FeedDir:    feedDir,
		RulingsDir: rulingsDir,
		Store:      st,
		User:       "operator",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestServeHTTP_RootRendersThreePanes(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := w.Body.String()
	for _, marker := range []string{`id="pane-folders"`, `id="pane-list"`, `id="pane-reading"`} {
		if !strings.Contains(body, marker) {
			t.Errorf("response body missing pane marker %s", marker)
		}
	}

	if !strings.Contains(body, "Push-tier relaxation") {
		t.Error("response body missing hold title")
	}
	if !strings.Contains(body, "Should the push tier guard relax for release branches?") {
		t.Error("response body missing hold question")
	}
}

func TestServeHTTP_UnreadHoldListedBold(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `class="unread"`) {
		t.Error("expected never-read hold to be marked unread in the list")
	}
}
