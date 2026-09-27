package group

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRead_ValidFileParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "groups.json")
	data := `{
  "gastownhall/gascity#5795": {"group": "sjarmak-reviewing", "action": "close", "note": "sjarmak is reviewing these himself: close as yield to human reviewer"},
  "gastownhall/gascity#5800": {"group": "julian-own-prs", "action": "close", "note": "Julian's own PRs: close as maintainer-owned"}
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}

	key := Key("gastownhall/gascity", 5795)
	p, ok := got[key]
	if !ok {
		t.Fatalf("missing entry for %s", key)
	}
	if p.Group != "sjarmak-reviewing" || p.Action != "close" ||
		p.Note != "sjarmak is reviewing these himself: close as yield to human reviewer" {
		t.Errorf("proposal = %+v", p)
	}
}

func TestRead_MissingFileReturnsEmptyMapNilError(t *testing.T) {
	dir := t.TempDir()

	got, err := Read(filepath.Join(dir, "no-such-file.json"))
	if err != nil {
		t.Fatalf("Read returned error for missing file: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %+v", got)
	}
}

func TestRead_MalformedJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "groups.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := Read(path); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestKey_RoundTrips(t *testing.T) {
	tests := []struct {
		repo string
		pr   int
		want string
	}{
		{"gastownhall/gascity", 5795, "gastownhall/gascity#5795"},
		{"quad341/hold-court", 1, "quad341/hold-court#1"},
	}
	for _, tt := range tests {
		if got := Key(tt.repo, tt.pr); got != tt.want {
			t.Errorf("Key(%q, %d) = %q, want %q", tt.repo, tt.pr, got, tt.want)
		}
	}
}
