package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/quad341/hold-court/internal/feed"
	"github.com/quad341/hold-court/internal/store"
)

var hexVersion = regexp.MustCompile(`^[0-9a-f]{16}$`)

func getHoldsDocument(t *testing.T, h http.Handler, method, path, ifNoneMatch string) (*httptest.ResponseRecorder, holdsDocument) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var doc holdsDocument
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return w, doc
}

func TestHandleHolds_VersionFollowsFeedAndETagFollowsEverything(t *testing.T) {
	feedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(feedDir, "hold.json"), []byte(`{"id":"`+fixtureHoldID+`","title":"First","held_at":"2026-09-01T15:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rulingsDir := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h, err := New(Config{FeedDir: feedDir, RulingsDir: rulingsDir, Store: st})
	if err != nil {
		t.Fatal(err)
	}

	w, first := getHoldsDocument(t, h, http.MethodGet, "/api/holds", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !hexVersion.MatchString(first.Version) {
		t.Fatalf("version = %q, want %d hex characters", first.Version, feed.VersionLength)
	}
	etag := w.Header().Get("ETag")
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) || strings.HasPrefix(etag, `W/`) {
		t.Fatalf("ETag %q is not a strong validator", etag)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	// Unchanged: 304 with no body, ETag still advertised.
	w, _ = getHoldsDocument(t, h, http.MethodGet, "/api/holds", etag)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("revalidation = %d with %d body bytes, want 304 and none", w.Code, w.Body.Len())
	}
	if w.Header().Get("ETag") != etag {
		t.Errorf("304 ETag = %q, want %q", w.Header().Get("ETag"), etag)
	}

	// A ruling changes the document (new ETag) but not the feed version:
	// the operator's own action must not read as a feed invalidation.
	writeRulingFixture(t, rulingsDir)
	w, ruled := getHoldsDocument(t, h, http.MethodGet, "/api/holds", etag)
	if w.Code != http.StatusOK {
		t.Fatalf("after ruling: status = %d, want 200", w.Code)
	}
	if ruled.Version != first.Version {
		t.Errorf("ruling changed feed version %q -> %q", first.Version, ruled.Version)
	}
	if w.Header().Get("ETag") == etag {
		t.Error("ruling did not change the ETag")
	}

	// A feed rewrite changes both. fsnotify may or may not have fired yet;
	// the interval rescan is what the client relies on, so wait for it.
	if err := os.WriteFile(filepath.Join(feedDir, "hold.json"), []byte(`{"id":"`+fixtureHoldID+`","title":"Second","held_at":"2026-09-01T15:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * feedRescanInterval)
	for {
		w, changed := getHoldsDocument(t, h, http.MethodGet, "/api/holds", "")
		if changed.Version != first.Version {
			if len(changed.Holds) != 1 || changed.Holds[0].Title != "Second" {
				t.Fatalf("new version did not carry new contents: %+v", changed.Holds)
			}
			if w.Header().Get("ETag") == etag {
				t.Error("feed change did not change the ETag")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("feed rewrite never changed the version")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// newColdCacheServer builds a server whose feed cache will not rescan on
// its own (no fsnotify watch, an hour-long interval), so a test can prove
// the rescan endpoint is what bypasses it.
func newColdCacheServer(t *testing.T) (*server, string) {
	t.Helper()
	feedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(feedDir, "one.json"), []byte(`{"id":"one","title":"One","held_at":"2026-09-01T15:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := &server{cfg: Config{Store: st, RulingsDir: t.TempDir(), User: "operator"}, feed: newFeedCache(feedDir, time.Hour)}
	return s, feedDir
}

func TestHandleRescan_BypassesFeedCache(t *testing.T) {
	s, feedDir := newColdCacheServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/holds", s.handleHolds)
	mux.HandleFunc("POST /api/holds/rescan", s.handleRescan)

	_, before := getHoldsDocument(t, mux, http.MethodGet, "/api/holds", "")
	if len(before.Holds) != 1 {
		t.Fatalf("holds = %d, want 1", len(before.Holds))
	}
	if err := os.WriteFile(filepath.Join(feedDir, "two.json"), []byte(`{"id":"two","title":"Two","held_at":"2026-09-02T15:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stale := getHoldsDocument(t, mux, http.MethodGet, "/api/holds", ""); len(stale.Holds) != 1 || stale.Version != before.Version {
		t.Fatalf("cold cache rescanned by itself: %d holds, version %q", len(stale.Holds), stale.Version)
	}

	w, rebuilt := getHoldsDocument(t, mux, http.MethodPost, "/api/holds/rescan", "")
	if w.Code != http.StatusOK {
		t.Fatalf("rescan status = %d: %s", w.Code, w.Body.String())
	}
	if len(rebuilt.Holds) != 2 || rebuilt.Version == before.Version || !hexVersion.MatchString(rebuilt.Version) {
		t.Fatalf("rescan did not re-read the feed: %d holds, version %q -> %q", len(rebuilt.Holds), before.Version, rebuilt.Version)
	}
	if w.Header().Get("ETag") == "" || w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("rescan response missing ETag/JSON headers: %v", w.Header())
	}
	if _, after := getHoldsDocument(t, mux, http.MethodGet, "/api/holds", ""); len(after.Holds) != 2 || after.Version != rebuilt.Version {
		t.Fatalf("subsequent GET disagrees with rescan: %d holds, version %q", len(after.Holds), after.Version)
	}

	// A rebuild always returns the full document, even to a client that
	// already has it: the point is to refill an emptied cache.
	w, _ = getHoldsDocument(t, mux, http.MethodPost, "/api/holds/rescan", w.Header().Get("ETag"))
	if w.Code != http.StatusOK || w.Body.Len() == 0 {
		t.Fatalf("rescan with matching If-None-Match = %d, want a full 200", w.Code)
	}
}
