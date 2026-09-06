package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func liveSnapshot(t *testing.T, h http.Handler) ([]holdJSON, string) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/holds", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Holds []holdJSON `json:"holds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Holds, w.Header().Get("ETag")
}

func TestLiveSnapshotRevalidatesAndShowsResultDetails(t *testing.T) {
	h, dir := newHoldFixtureHandler(t, false)
	before, etag := liveSnapshot(t, h)
	if len(before) != 1 || etag == "" || before[0].Revision == "" {
		t.Fatal("missing initial hold revision or ETag")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/holds", nil)
	req.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotModified {
		t.Fatalf("unchanged snapshot = %d", w.Code)
	}
	writeRulingFixture(t, dir)
	path := filepath.Join(dir, fixtureHoldID+".result.json")
	if err := os.WriteFile(path, []byte(`{"status":"failed","summary":"The PR head changed; no action taken."}`), 0o600); err != nil {
		t.Fatal(err)
	}
	after, newETag := liveSnapshot(t, h)
	if after[0].State != "ruled" || after[0].Result.Summary != "The PR head changed; no action taken." {
		t.Fatalf("failure must remain ruled and expose its explanation: %+v", after[0])
	}
	if newETag == etag || after[0].Revision == before[0].Revision || after[0].Ruling == nil {
		t.Fatal("new decision/result did not invalidate snapshot")
	}
}

func TestLiveResultReopensUnreadUntilThatRevisionIsRead(t *testing.T) {
	h, dir := newHoldFixtureHandler(t, false)
	writeRulingFixture(t, dir)
	holds, _ := liveSnapshot(t, h)
	markRead := func(revision string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/holds/"+fixtureHoldID+"/read", strings.NewReader(`{"revision":"`+revision+`"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("read failed: %s", w.Body.String())
		}
	}
	markRead(holds[0].ActivityRevision)
	read, _ := liveSnapshot(t, h)
	if read[0].Unread {
		t.Fatal("acknowledged hold still unread")
	}
	writeResultFixture(t, dir)
	updated, _ := liveSnapshot(t, h)
	if !updated[0].Unread || !updated[0].Updated {
		t.Fatal("new result must signal unread activity")
	}
	// A delayed acknowledgement of old content must not mark the new result read.
	markRead(holds[0].ActivityRevision)
	stillUpdated, _ := liveSnapshot(t, h)
	if !stillUpdated[0].Updated {
		t.Fatal("old acknowledgement swallowed new activity")
	}
	markRead(updated[0].ActivityRevision)
	final, _ := liveSnapshot(t, h)
	if final[0].Unread || final[0].Updated {
		t.Fatal("reading the new revision did not clear activity")
	}
}

func TestSaveRejectsStaleAndResolvedDecisions(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		h, dir := newHoldFixtureHandler(t, resolved)
		req := httptest.NewRequest(http.MethodPost, "/api/rulings", strings.NewReader(`[{"hold_id":"`+fixtureHoldID+`","action":"proceed","revision":"outdated"}]`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var results []rulingResponse
		if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].OK || results[0].Error == "" {
			t.Fatalf("stale/resolved decision accepted: %s", w.Body.String())
		}
		if _, err := os.Stat(filepath.Join(dir, fixtureHoldID+".json")); !os.IsNotExist(err) {
			t.Fatal("rejected ruling wrote a file")
		}
	}
}

func TestFeedSnapshotDoesNotShareMutableSlice(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(`{"id":"`+id+`"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cache := newFeedCache(dir, time.Hour)
	first, err := cache.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	first[0], first[1] = first[1], first[0]
	second, err := cache.snapshot()
	if err != nil || second[0].ID != "a" {
		t.Fatal("sorting one request changed another request's snapshot")
	}
}

func TestOwnDecisionDoesNotCreateIncomingActivityAndHistoryPersists(t *testing.T) {
	h, dir := newHoldFixtureHandler(t, false)
	before, _ := liveSnapshot(t, h)
	req := httptest.NewRequest(http.MethodPost, "/api/holds/"+fixtureHoldID+"/read", strings.NewReader(`{"revision":"`+before[0].ActivityRevision+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)
	writeRulingFixture(t, dir)
	after, _ := liveSnapshot(t, h)
	if after[0].Updated || after[0].ActivityRevision != before[0].ActivityRevision || after[0].Revision == before[0].Revision {
		t.Fatal("own decision must change displayed state but not incoming activity")
	}
	getHistory := func() []struct {
		Kind string `json:"kind"`
	} {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/holds/"+fixtureHoldID+"/history", nil))
		var entries []struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
			t.Fatal(err)
		}
		return entries
	}
	if len(getHistory()) != 2 {
		t.Fatal("expected a review and a decision")
	}
	liveSnapshot(t, h)
	if len(getHistory()) != 2 {
		t.Fatal("unchanged polling must not create history")
	}
	if err := os.WriteFile(filepath.Join(dir, fixtureHoldID+".thread.json"), []byte(`{"messages":[{"id":"reply-1","author":"agent","body":"A substantive answer","at":"2026-09-05T12:00:00Z"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	replied, _ := liveSnapshot(t, h)
	if !replied[0].Updated || len(replied[0].Thread) != 1 {
		t.Fatal("reply did not produce incoming activity")
	}
	if entries := getHistory(); len(entries) != 3 || entries[0].Kind != "discussion" {
		t.Fatalf("missing discussion history: %+v", entries)
	}
}

func TestFollowupQueueDoesNotCreateIncomingActivity(t *testing.T) {
	h, dir := newHoldFixtureHandler(t, false)
	writeRulingFixture(t, dir)
	resultPath := filepath.Join(dir, fixtureHoldID+".result.json")
	if err := os.WriteFile(resultPath, []byte(`{"status":"reply_ready","summary":"Answer"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := liveSnapshot(t, h)
	req := httptest.NewRequest(http.MethodPost, "/api/holds/"+fixtureHoldID+"/read", strings.NewReader(`{"revision":"`+before[0].ActivityRevision+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if err := os.WriteFile(resultPath, []byte(`{"status":"queued","summary":"Follow-up queued"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	after, _ := liveSnapshot(t, h)
	if after[0].Updated || after[0].ActivityRevision != before[0].ActivityRevision {
		t.Fatal("follow-up queue created incoming activity")
	}
	if err := os.WriteFile(resultPath, []byte(`{"status":"in_progress","summary":"Follow-up acknowledged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	acknowledged, _ := liveSnapshot(t, h)
	if !acknowledged[0].Updated {
		t.Fatal("agent acknowledgement did not create incoming activity")
	}
}
