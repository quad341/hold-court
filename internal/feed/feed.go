// Package feed reads the Hold Court feed contract (DESIGN.md "Feed contract
// v0"): a directory of JSON documents, one per hold, owned and written by an
// adapter. Hold Court treats the directory as read-only.
package feed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Hold is one held PR awaiting a ruling, as described by a single
// feed/<id>.json document.
type Hold struct {
	ID             string    `json:"id"`
	Source         string    `json:"source"`
	Repo           string    `json:"repo"`
	PR             int       `json:"pr"`
	URL            string    `json:"url"`
	Class          string    `json:"class"`
	Title          string    `json:"title"`
	Question       string    `json:"question"`
	ReviewBodyMD   string    `json:"review_body_md"`
	Verdict        string    `json:"verdict"`
	HeadSHA        string    `json:"head_sha"`
	HeldAt         time.Time `json:"held_at"`
	Resolved       bool      `json:"resolved"`
	ResolvedReason string    `json:"resolved_reason"`
}

// ParseHold decodes a single feed document. An "id" field is required.
func ParseHold(data []byte) (*Hold, error) {
	var h Hold
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("feed: parse hold: %w", err)
	}
	if h.ID == "" {
		return nil, fmt.Errorf("feed: parse hold: missing required field %q", "id")
	}
	return &h, nil
}

// ScanDir reads every *.json file directly inside dir and parses it as a
// Hold, returning them sorted by ID. Non-JSON files are ignored. dir is
// treated as read-only, per the feed contract.
func ScanDir(dir string) ([]*Hold, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("feed: scan dir %s: %w", dir, err)
	}

	var holds []*Hold
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path) //nolint:gosec // entry.Name() is an OS-returned directory entry, not external input
		if err != nil {
			return nil, fmt.Errorf("feed: read %s: %w", path, err)
		}
		h, err := ParseHold(data)
		if err != nil {
			return nil, fmt.Errorf("feed: %s: %w", path, err)
		}
		holds = append(holds, h)
	}

	sort.Slice(holds, func(i, j int) bool { return holds[i].ID < holds[j].ID })
	return holds, nil
}
