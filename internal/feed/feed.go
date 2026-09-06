// Package feed reads the Hold Court feed contract (DESIGN.md "Feed contract
// v0"): a directory of JSON documents, one per hold, owned and written by an
// adapter. Hold Court treats the directory as read-only.
package feed

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	DecisionContextMD string    `json:"decision_context_md,omitempty"`
	Author            string    `json:"author,omitempty"`
	ID                string    `json:"id"`
	Source            string    `json:"source"`
	Repo              string    `json:"repo"`
	PR                int       `json:"pr"`
	URL               string    `json:"url"`
	Class             string    `json:"class"`
	Title             string    `json:"title"`
	Question          string    `json:"question"`
	ReviewBodyMD      string    `json:"review_body_md"`
	Verdict           string    `json:"verdict"`
	HeadSHA           string    `json:"head_sha"`
	HeldAt            time.Time `json:"held_at"`
	Resolved          bool      `json:"resolved"`
	ResolvedReason    string    `json:"resolved_reason"`
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
	holds, _, err := Scan(dir)
	return holds, err
}

// VersionLength is the length of the hex version string Scan returns.
const VersionLength = 16

// EmptyVersion is the version Scan reports for a feed with no documents.
var EmptyVersion = hex.EncodeToString(sha256.New().Sum(nil))[:VersionLength]

// Scan is ScanDir plus a version of the feed: a hex digest over the name
// and contents of every scanned file, in directory order. The version
// changes whenever an adapter writes anything different into the feed,
// including a rewrite that keeps a file's size and modification time, and
// stays the same when a scan finds identical files, so it is safe to show
// as "the data version" and to compare across server restarts. It costs
// nothing beyond the reads ScanDir already does.
func Scan(dir string) ([]*Hold, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("feed: scan dir %s: %w", dir, err)
	}

	digest := sha256.New()
	var holds []*Hold
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path) //nolint:gosec // entry.Name() is an OS-returned directory entry, not external input
		if err != nil {
			return nil, "", fmt.Errorf("feed: read %s: %w", path, err)
		}
		h, err := ParseHold(data)
		if err != nil {
			return nil, "", fmt.Errorf("feed: %s: %w", path, err)
		}
		holds = append(holds, h)
		// Length-prefix each field so (name, contents) pairs cannot collide
		// by shifting bytes between them.
		_ = binary.Write(digest, binary.BigEndian, uint64(len(entry.Name())))
		digest.Write([]byte(entry.Name()))
		_ = binary.Write(digest, binary.BigEndian, uint64(len(data)))
		digest.Write(data)
	}

	sort.Slice(holds, func(i, j int) bool { return holds[i].ID < holds[j].ID })
	return holds, hex.EncodeToString(digest.Sum(nil))[:VersionLength], nil
}
