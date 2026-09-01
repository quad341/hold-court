package feed

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validHoldJSON = `{
  "id": "gastownhall-gascity-5795-a1b2c3",
  "source": "maintainer-pr-review",
  "repo": "gastownhall/gascity",
  "pr": 5795,
  "url": "https://github.com/gastownhall/gascity/pull/5795",
  "class": "ambiguous-needs-discussion",
  "title": "Push-tier relaxation",
  "question": "The one operative question, <= ~80 words.",
  "review_body_md": "... full prepared review, markdown ...",
  "verdict": "fix-merge",
  "head_sha": "abc123",
  "held_at": "2026-09-01T15:00:00Z",
  "resolved": false,
  "resolved_reason": ""
}`

func TestParseHold_ValidDocument(t *testing.T) {
	h, err := ParseHold([]byte(validHoldJSON))
	if err != nil {
		t.Fatalf("ParseHold returned error: %v", err)
	}
	if h.ID != "gastownhall-gascity-5795-a1b2c3" {
		t.Errorf("ID = %q", h.ID)
	}
	if h.Source != "maintainer-pr-review" {
		t.Errorf("Source = %q", h.Source)
	}
	if h.Repo != "gastownhall/gascity" {
		t.Errorf("Repo = %q", h.Repo)
	}
	if h.PR != 5795 {
		t.Errorf("PR = %d", h.PR)
	}
	if h.URL != "https://github.com/gastownhall/gascity/pull/5795" {
		t.Errorf("URL = %q", h.URL)
	}
	if h.Class != "ambiguous-needs-discussion" {
		t.Errorf("Class = %q", h.Class)
	}
	if h.Title != "Push-tier relaxation" {
		t.Errorf("Title = %q", h.Title)
	}
	if h.Question != "The one operative question, <= ~80 words." {
		t.Errorf("Question = %q", h.Question)
	}
	if h.ReviewBodyMD != "... full prepared review, markdown ..." {
		t.Errorf("ReviewBodyMD = %q", h.ReviewBodyMD)
	}
	if h.Verdict != "fix-merge" {
		t.Errorf("Verdict = %q", h.Verdict)
	}
	if h.HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q", h.HeadSHA)
	}
	wantHeldAt := time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC)
	if !h.HeldAt.Equal(wantHeldAt) {
		t.Errorf("HeldAt = %v, want %v", h.HeldAt, wantHeldAt)
	}
	if h.Resolved != false {
		t.Errorf("Resolved = %v", h.Resolved)
	}
	if h.ResolvedReason != "" {
		t.Errorf("ResolvedReason = %q", h.ResolvedReason)
	}
}

func TestParseHold_MissingID(t *testing.T) {
	_, err := ParseHold([]byte(`{"source": "x", "title": "no id here"}`))
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
}

func TestParseHold_InvalidJSON(t *testing.T) {
	_, err := ParseHold([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestScanDir_ListsAndParsesAll(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "b.json", `{"id":"b-hold","title":"B"}`)
	writeFixture(t, dir, "a.json", `{"id":"a-hold","title":"A"}`)

	holds, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir returned error: %v", err)
	}
	if len(holds) != 2 {
		t.Fatalf("len(holds) = %d, want 2", len(holds))
	}
	if holds[0].ID != "a-hold" || holds[1].ID != "b-hold" {
		t.Errorf("holds not sorted by ID: got [%s, %s]", holds[0].ID, holds[1].ID)
	}
}

func TestScanDir_IgnoresNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "hold.json", `{"id":"real-hold","title":"Real"}`)
	writeFixture(t, dir, "notes.txt", `not a hold`)

	holds, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir returned error: %v", err)
	}
	if len(holds) != 1 {
		t.Fatalf("len(holds) = %d, want 1", len(holds))
	}
	if holds[0].ID != "real-hold" {
		t.Errorf("ID = %q, want %q", holds[0].ID, "real-hold")
	}
}

func TestScanDir_EmptyDirReturnsEmptySlice(t *testing.T) {
	dir := t.TempDir()
	holds, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ScanDir returned error: %v", err)
	}
	if len(holds) != 0 {
		t.Fatalf("len(holds) = %d, want 0", len(holds))
	}
}

func TestScanDir_MalformedFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "broken.json", `{not valid json`)

	_, err := ScanDir(dir)
	if err == nil {
		t.Fatal("expected error for malformed feed file, got nil")
	}
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writeFixture(%s): %v", name, err)
	}
}
