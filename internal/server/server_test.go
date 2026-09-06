package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/quad341/hold-court/internal/ruling"
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
	if err := os.WriteFile(filepath.Join(feedDir, "hold1.json"), []byte(holdJSON), 0o600); err != nil {
		t.Fatalf("write feed fixture: %v", err)
	}

	rulingsDir := t.TempDir()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

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

// fixtureHoldID is the hold id baked into newTestHandler's and
// newHoldFixtureHandler's feed fixture.
const fixtureHoldID = "gastownhall-gascity-5795-a1b2c3"

// newHoldFixtureHandler is like newTestHandler but exposes the rulings
// directory (so a test can seed ruling/result fixtures before the handler
// computes holdState) and lets the caller control the hold's Resolved flag,
// which only matters for the stood-down branch.
func newHoldFixtureHandler(t *testing.T, resolved bool) (http.Handler, string) {
	t.Helper()

	feedDir := t.TempDir()
	holdJSON := fmt.Sprintf(`{
  "id": %q,
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
  "resolved": %v,
  "resolved_reason": ""
}`, fixtureHoldID, resolved)
	if err := os.WriteFile(filepath.Join(feedDir, "hold1.json"), []byte(holdJSON), 0o600); err != nil {
		t.Fatalf("write feed fixture: %v", err)
	}

	rulingsDir := t.TempDir()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h, err := New(Config{
		FeedDir:    feedDir,
		RulingsDir: rulingsDir,
		Store:      st,
		User:       "operator",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h, rulingsDir
}

func writeRulingFixture(t *testing.T, rulingsDir string) {
	t.Helper()
	r := ruling.Ruling{
		HoldID:  fixtureHoldID,
		Action:  ruling.Proceed,
		RuledBy: "operator",
		RuledAt: time.Date(2026, 9, 1, 16, 0, 0, 0, time.UTC),
	}
	if err := ruling.Write(rulingsDir, r); err != nil {
		t.Fatalf("ruling.Write: %v", err)
	}
}

func writeResultFixture(t *testing.T, rulingsDir string) {
	t.Helper()
	path := filepath.Join(rulingsDir, fixtureHoldID+".result.json")
	if err := os.WriteFile(path, []byte(`{"status":"executed","summary":"merged as-is"}`), 0o600); err != nil {
		t.Fatalf("write result fixture: %v", err)
	}
}

func getIndexBody(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Body.String()
}

func TestServeHTTP_HoldStateRuled(t *testing.T) {
	h, rulingsDir := newHoldFixtureHandler(t, false)
	writeRulingFixture(t, rulingsDir)

	body := getIndexBody(t, h)
	if !strings.Contains(body, `"state":"ruled"`) {
		t.Errorf("expected ruled hold to render state \"ruled\"; body=%s", body)
	}
}

func TestServeHTTP_HoldStateExecuted(t *testing.T) {
	h, rulingsDir := newHoldFixtureHandler(t, false)
	writeRulingFixture(t, rulingsDir)
	writeResultFixture(t, rulingsDir)

	body := getIndexBody(t, h)
	if !strings.Contains(body, `"state":"executed"`) {
		t.Errorf("expected ruled+executed hold to render state \"executed\"; body=%s", body)
	}
}

func TestServeHTTP_HoldStateStoodDown(t *testing.T) {
	h, _ := newHoldFixtureHandler(t, true)

	body := getIndexBody(t, h)
	if !strings.Contains(body, `"state":"stood-down"`) {
		t.Errorf("expected unruled+resolved hold to render state \"stood-down\"; body=%s", body)
	}
}

func TestHandleSetRead_RequiresJSONContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{"missing content-type", "", http.StatusUnsupportedMediaType},
		{"wrong content-type", "text/plain", http.StatusUnsupportedMediaType},
		{"exact json accepted", "application/json", http.StatusNoContent},
		{"json with charset accepted", "application/json; charset=utf-8", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)

			req := httptest.NewRequest(http.MethodPost, "/api/holds/"+fixtureHoldID+"/read", strings.NewReader(`{"unread":false}`))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if got := w.Result().StatusCode; got != tt.wantStatus {
				t.Errorf("status = %d, want %d", got, tt.wantStatus)
			}
		})
	}
}

func TestHandleSaveRulings_RequiresJSONContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{"missing content-type", "", http.StatusUnsupportedMediaType},
		{"wrong content-type", "text/plain", http.StatusUnsupportedMediaType},
		{"exact json accepted", "application/json", http.StatusOK},
		{"json with charset accepted", "application/json; charset=utf-8", http.StatusOK},
	}

	body := `[{"hold_id":"` + fixtureHoldID + `","action":"proceed","note":"Proceed with the proposed fix and verify it before merging."}]`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t)

			req := httptest.NewRequest(http.MethodPost, "/api/rulings", strings.NewReader(body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if got := w.Result().StatusCode; got != tt.wantStatus {
				t.Errorf("status = %d, want %d", got, tt.wantStatus)
			}
		})
	}
}

// captureLog redirects the standard library's global log output to a buffer
// for the duration of t, restoring it on cleanup. Safe as a plain
// bytes.Buffer here: ServeHTTP runs synchronously on the test goroutine, and
// feedCache.watch's background goroutine (started by New) only calls
// log.Printf on fsnotify setup failure, which the temp-dir fixtures in this
// file never trigger.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })
	return &buf
}

func TestHandleSetRead_QuietOnSuccess(t *testing.T) {
	h := newTestHandler(t)
	logBuf := captureLog(t)

	req := httptest.NewRequest(http.MethodPost, "/api/holds/"+fixtureHoldID+"/read", strings.NewReader(`{"unread":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Result().StatusCode; got != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", got, http.StatusNoContent)
	}
	if logBuf.Len() != 0 {
		t.Errorf("successful read-state change should be quiet; got log output: %q", logBuf.String())
	}
}

func TestHandleSaveRulings_QuietOnSuccess(t *testing.T) {
	h := newTestHandler(t)
	logBuf := captureLog(t)

	body := `[{"hold_id":"` + fixtureHoldID + `","action":"proceed","note":"Proceed with the proposed fix and verify it before merging."}]`
	req := httptest.NewRequest(http.MethodPost, "/api/rulings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
	if logBuf.Len() != 0 {
		t.Errorf("successful ruling write should be quiet; got log output: %q", logBuf.String())
	}
}

// A failed transport has already committed a response, just like net/http.
type failedPageWriter struct {
	header           http.Header
	committed        bool
	duplicateHeaders int
	writes           int
}

func (w *failedPageWriter) Header() http.Header { return w.header }
func (w *failedPageWriter) WriteHeader(_ int) {
	if w.committed {
		w.duplicateHeaders++
	}
	w.committed = true
}
func (w *failedPageWriter) Write(_ []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	w.writes++
	return 0, io.ErrClosedPipe
}

func TestIndexResponseFailures(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &server{cfg: Config{Store: db, RulingsDir: t.TempDir(), User: "operator"}, feed: newFeedCache(t.TempDir(), time.Hour)}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	t.Run("template failure is a clean 500", func(t *testing.T) {
		s.tmpl = template.Must(template.New("index.html.tmpl").Parse(`partial page{{.MissingField}}`))
		w := httptest.NewRecorder()
		s.handleIndex(w, request)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
		if strings.HasPrefix(w.Body.String(), "partial page") {
			t.Fatal("partial template leaked into error response")
		}
		if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
			t.Fatalf("content type = %q", got)
		}
	})
	t.Run("disconnect does not write a second response", func(t *testing.T) {
		s.tmpl = template.Must(template.New("index.html.tmpl").Parse(`complete page`))
		w := &failedPageWriter{header: make(http.Header)}
		s.handleIndex(w, request)
		if w.duplicateHeaders != 0 || w.writes != 1 {
			t.Fatalf("duplicate headers = %d, writes = %d", w.duplicateHeaders, w.writes)
		}
	})
}

func TestAllRulingsRequireResponse(t *testing.T) {
	h, dir := newHoldFixtureHandler(t, false)
	for _, action := range []string{"proceed", "changes", "close", "discuss"} {
		body := `[{"hold_id":"` + fixtureHoldID + `","action":"` + action + `","note":" \n\t"}]`
		req := httptest.NewRequest(http.MethodPost, "/api/rulings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if !strings.Contains(w.Body.String(), "Respond to the hold before saving") || strings.Contains(w.Body.String(), `"ok":true`) {
			t.Fatalf("blank %s accepted: %s", action, w.Body.String())
		}
		if _, err := os.Stat(filepath.Join(dir, fixtureHoldID+".json")); !os.IsNotExist(err) {
			t.Fatalf("blank %s wrote a ruling", action)
		}
	}
}
