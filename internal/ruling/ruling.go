// Package ruling writes maintainer rulings out to disk (DESIGN.md "Rulings
// out") and runs the optional on_ruling hook. Hold Court never executes a
// ruling itself; it is the bench, not the bailiff.
package ruling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Action is a maintainer's verdict on a hold.
type Action string

// The four rulings a maintainer may hand down.
const (
	Proceed Action = "proceed"
	Changes Action = "changes"
	Close   Action = "close"
	Discuss Action = "discuss"
)

func (a Action) valid() bool {
	switch a {
	case Proceed, Changes, Close, Discuss:
		return true
	default:
		return false
	}
}

// safeHoldID rejects a hold ID that isn't a single clean path component, so
// a caller-supplied ID (handleSaveRulings takes hold_id straight from the
// request body) can never traverse dir via "../" segments when joined into
// a rulings-file path.
func safeHoldID(id string) error {
	if id == "" || id != filepath.Base(id) || id == "." || id == ".." {
		return fmt.Errorf("ruling: invalid hold id %q", id)
	}
	return nil
}

// Ruling is the JSON document written to rulings/<hold-id>.json.
type Ruling struct {
	HoldID  string    `json:"hold_id"`
	Action  Action    `json:"action"`
	Note    string    `json:"note"`
	RuledBy string    `json:"ruled_by"`
	RuledAt time.Time `json:"ruled_at"`
}

// Result is the JSON document a ruling's consumer writes back to
// rulings/<hold-id>.result.json once it has acted on the ruling.
type Result struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// Write validates r and saves it to dir/<hold_id>.json, creating dir if
// necessary.
func Write(dir string, r Ruling) error {
	if err := safeHoldID(r.HoldID); err != nil {
		return fmt.Errorf("ruling: write: %w", err)
	}
	if !r.Action.valid() {
		return fmt.Errorf("ruling: write: invalid action %q", r.Action)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("ruling: write: marshal: %w", err)
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("ruling: write: mkdir %s: %w", dir, err)
	}

	path := filepath.Join(dir, r.HoldID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("ruling: write %s: %w", path, err)
	}
	return nil
}

// RunHook runs the configured on_ruling hook, if any, piping r's JSON
// encoding to its stdin. A nil or empty cmd is a no-op: the hook is optional.
func RunHook(cmd []string, r Ruling) error {
	if len(cmd) == 0 {
		return nil
	}

	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("ruling: run hook: marshal: %w", err)
	}

	c := exec.Command(cmd[0], cmd[1:]...) //nolint:gosec // cmd is the operator's own on_ruling config, not external input
	c.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("ruling: on_ruling hook %v: %w (stderr: %s)", cmd, err, stderr.String())
	}
	return nil
}

// Read reads dir/<holdID>.json, if present. found is false (with a nil
// error) when the hold has not been ruled on yet.
func Read(dir, holdID string) (r *Ruling, found bool, err error) {
	if err := safeHoldID(holdID); err != nil {
		return nil, false, fmt.Errorf("ruling: read: %w", err)
	}
	path := filepath.Join(dir, holdID+".json")
	data, err := os.ReadFile(path) //nolint:gosec // holdID is validated by safeHoldID above
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("ruling: read %s: %w", path, err)
	}

	var ruling Ruling
	if err := json.Unmarshal(data, &ruling); err != nil {
		return nil, false, fmt.Errorf("ruling: read %s: %w", path, err)
	}
	return &ruling, true, nil
}

// ReadResult reads dir/<holdID>.result.json, if present. found is false
// (with a nil error) when the consumer has not yet reported back.
func ReadResult(dir, holdID string) (result *Result, found bool, err error) {
	if err := safeHoldID(holdID); err != nil {
		return nil, false, fmt.Errorf("ruling: read result: %w", err)
	}
	path := filepath.Join(dir, holdID+".result.json")
	data, err := os.ReadFile(path) //nolint:gosec // holdID is validated by safeHoldID above
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("ruling: read result %s: %w", path, err)
	}

	var res Result
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, false, fmt.Errorf("ruling: read result %s: %w", path, err)
	}
	return &res, true, nil
}
