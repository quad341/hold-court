package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/quad341/hold-court/internal/store"
)

// secondFixtureHoldID is a second hold in the groups fixture feed, distinct
// from fixtureHoldID, so group-field and group-folder assertions can tell "in
// the group" and "not in the group" apart.
const secondFixtureHoldID = "gastownhall-gascity-5800-b2c3d4"

// newGroupsFixtureHandler builds a handler over a two-hold feed and a
// curator groups file that proposes a ruling for only one of them
// (fixtureHoldID), so tests can assert the other hold is left untouched.
func newGroupsFixtureHandler(t *testing.T) http.Handler {
	t.Helper()

	feedDir := t.TempDir()
	first := `{
  "id": "` + fixtureHoldID + `",
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
	second := `{
  "id": "` + secondFixtureHoldID + `",
  "source": "maintainer-pr-review",
  "repo": "gastownhall/gascity",
  "pr": 5800,
  "url": "https://github.com/gastownhall/gascity/pull/5800",
  "class": "ambiguous-needs-discussion",
  "title": "Unrelated PR",
  "question": "Should this land?",
  "review_body_md": "No grouping applies here.",
  "verdict": "fix-merge",
  "head_sha": "def456",
  "held_at": "2026-09-01T16:00:00Z",
  "resolved": false,
  "resolved_reason": ""
}`
	if err := os.WriteFile(filepath.Join(feedDir, "hold1.json"), []byte(first), 0o600); err != nil {
		t.Fatalf("write feed fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(feedDir, "hold2.json"), []byte(second), 0o600); err != nil {
		t.Fatalf("write feed fixture: %v", err)
	}

	groupsPath := filepath.Join(t.TempDir(), "groups.json")
	groupsJSON := `{
  "gastownhall/gascity#5795": {"group": "sjarmak-reviewing", "action": "close", "note": "sjarmak is reviewing these himself: close as yield to human reviewer"}
}`
	if err := os.WriteFile(groupsPath, []byte(groupsJSON), 0o600); err != nil {
		t.Fatalf("write groups fixture: %v", err)
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h, err := New(Config{
		FeedDir:    feedDir,
		RulingsDir: t.TempDir(),
		Store:      st,
		User:       "operator",
		GroupsFile: groupsPath,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func getHoldsDoc(t *testing.T, h http.Handler) holdsDocument {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/holds", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/holds status = %d: %s", w.Code, w.Body.String())
	}
	var doc holdsDocument
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode holds document: %v", err)
	}
	return doc
}

func TestHandleHolds_SetsGroupFieldsOnlyForGroupedHold(t *testing.T) {
	h := newGroupsFixtureHandler(t)
	doc := getHoldsDoc(t, h)

	byID := make(map[string]holdJSON, len(doc.Holds))
	for _, hold := range doc.Holds {
		byID[hold.ID] = hold
	}

	grouped, ok := byID[fixtureHoldID]
	if !ok {
		t.Fatalf("fixture hold %s missing from document", fixtureHoldID)
	}
	if grouped.Group != "sjarmak-reviewing" || grouped.ProposedAction != "close" ||
		grouped.ProposedNote != "sjarmak is reviewing these himself: close as yield to human reviewer" {
		t.Errorf("grouped hold = %+v, want group/proposed_action/proposed_note from groups file", grouped)
	}

	ungrouped, ok := byID[secondFixtureHoldID]
	if !ok {
		t.Fatalf("fixture hold %s missing from document", secondFixtureHoldID)
	}
	if ungrouped.Group != "" || ungrouped.ProposedAction != "" || ungrouped.ProposedNote != "" {
		t.Errorf("ungrouped hold carries group fields: %+v", ungrouped)
	}
}

func TestHandleHolds_NoGroupsFileLeavesGroupFieldsUnset(t *testing.T) {
	// newTestHandler configures no GroupsFile at all: the feature must be
	// inert (no error, no fields set), the same posture as an unset OnRuling.
	h := newTestHandler(t)
	doc := getHoldsDoc(t, h)

	for _, hold := range doc.Holds {
		if hold.Group != "" || hold.ProposedAction != "" || hold.ProposedNote != "" {
			t.Errorf("hold carries group fields with no GroupsFile configured: %+v", hold)
		}
	}
}

func TestBuildFolders_GroupFolderHasCorrectCountAndLabel(t *testing.T) {
	h := newGroupsFixtureHandler(t)
	doc := getHoldsDoc(t, h)

	for _, f := range doc.Folders {
		if f.ID == "group:sjarmak-reviewing" {
			if f.Count != 1 || f.Label != "sjarmak-reviewing" {
				t.Errorf("group folder = %+v, want count=1 label=sjarmak-reviewing", f)
			}
			return
		}
	}
	t.Fatalf("folders missing group:sjarmak-reviewing: %+v", doc.Folders)
}

func TestFilterByFolder_GroupPrefixReturnsOnlyMatchingHolds(t *testing.T) {
	views := []holdJSON{
		{ID: fixtureHoldID, Group: "sjarmak-reviewing"},
		{ID: secondFixtureHoldID},
	}

	got := filterByFolder(views, "group:sjarmak-reviewing")
	if len(got) != 1 || got[0].ID != fixtureHoldID {
		t.Fatalf("filterByFolder(group:sjarmak-reviewing) = %+v, want only %s", got, fixtureHoldID)
	}
}
