// Package group lets a curator (today, the mayor) attach a proposed
// grouping and one proposed ruling to a hold ahead of operator review: an
// optional, read-only overlay Hold Court reads and never writes — the same
// posture as the ruling package's on_ruling hook. The mayor writes the file
// directly; it is not part of the MPR adapter/export pipeline.
package group

import (
	"encoding/json"
	"fmt"
	"os"
)

// Proposal is one curator-proposed grouping for a hold: which group it
// belongs to, and the ruling the curator suggests for the whole group.
// Action is a plain string, not ruling.Action: an entry whose Action isn't
// one of the four known rulings still carries Group/Note for display, and
// it is on the caller (the server/UI) to leave it out of any bulk action —
// one bad entry must not take the group action down for the rest of it.
type Proposal struct {
	Group  string `json:"group"`
	Action string `json:"action"`
	Note   string `json:"note"`
}

// Key builds the "repo#pr" string a groups file indexes proposals by.
func Key(repo string, pr int) string {
	return fmt.Sprintf("%s#%d", repo, pr)
}

// Read parses path as a JSON object of Key() -> Proposal. A missing file is
// not an error: it means the curator-groupings feature is inactive, the
// same posture as OnRuling being unset, so Read returns an empty map and a
// nil error. Malformed JSON is an error — nothing is silently dropped.
func Read(path string) (map[string]Proposal, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the operator's own config value, not external input
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Proposal{}, nil
		}
		return nil, fmt.Errorf("group: read %s: %w", path, err)
	}

	proposals := map[string]Proposal{}
	if err := json.Unmarshal(data, &proposals); err != nil {
		return nil, fmt.Errorf("group: read %s: %w", path, err)
	}
	return proposals, nil
}
