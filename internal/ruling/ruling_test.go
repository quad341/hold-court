package ruling

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/quad341/hold-court/internal/feed"
)

func TestWrite_CreatesFileWithSchema(t *testing.T) {
	dir := t.TempDir()
	r := Ruling{
		HoldID:  "gastownhall-gascity-5795-a1b2c3",
		Action:  Proceed,
		Note:    "looks fine, ship it",
		RuledBy: "operator",
		RuledAt: time.Date(2026, 9, 1, 16, 20, 0, 0, time.UTC),
	}

	if err := Write(dir, r); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	path := filepath.Join(dir, "gastownhall-gascity-5795-a1b2c3.json")
	data, err := os.ReadFile(path) //nolint:gosec // reads back the fixture this test just wrote
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}

	var got Ruling
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal written ruling: %v", err)
	}
	if got.HoldID != r.HoldID || got.Action != r.Action || got.Note != r.Note || got.RuledBy != r.RuledBy {
		t.Errorf("written ruling = %+v, want %+v", got, r)
	}
	if !got.RuledAt.Equal(r.RuledAt) {
		t.Errorf("RuledAt = %v, want %v", got.RuledAt, r.RuledAt)
	}
}

// TestWrite_SnapshotsJevWouldWhenPresent covers hc-o5t: when the hold being
// ruled on carries jev's logged pick, Write must snapshot it into the
// written ruling so the operator's decision and jev's logged pick are both
// visible side by side in the outcome record. This is a raw snapshot only;
// Write does not compute or assert agreement.
func TestWrite_SnapshotsJevWouldWhenPresent(t *testing.T) {
	dir := t.TempDir()
	r := Ruling{
		HoldID:  "some-hold",
		Action:  Proceed,
		Note:    "looks fine, ship it",
		RuledBy: "operator",
		RuledAt: time.Now(),
		JevWould: &feed.JevWould{
			Category:    "fix-merge",
			Probability: 0.62,
			Confidence:  0.81,
		},
	}

	if err := Write(dir, r); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "some-hold.json")) //nolint:gosec // reads back the fixture this test just wrote
	if err != nil {
		t.Fatalf("expected file: %v", err)
	}

	var got Ruling
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal written ruling: %v", err)
	}
	if got.JevWould == nil {
		t.Fatal("JevWould = nil, want snapshot preserved")
	}
	if *got.JevWould != *r.JevWould {
		t.Errorf("JevWould = %+v, want %+v", got.JevWould, r.JevWould)
	}
}

// TestWrite_NoJevWouldKeyWhenAbsent covers hc-o5t: a hold with no logged jev
// pick must not gain a jev_would key at all, keeping the written ruling
// identical to today's schema when jev has nothing to say.
func TestWrite_NoJevWouldKeyWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	r := Ruling{
		HoldID:  "some-hold",
		Action:  Proceed,
		Note:    "looks fine, ship it",
		RuledBy: "operator",
		RuledAt: time.Now(),
	}

	if err := Write(dir, r); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "some-hold.json")) //nolint:gosec // reads back the fixture this test just wrote
	if err != nil {
		t.Fatalf("expected file: %v", err)
	}
	if strings.Contains(string(data), "jev_would") {
		t.Errorf("expected no jev_would key when jev has no logged pick, got: %s", data)
	}
}

func TestWrite_RejectsInvalidAction(t *testing.T) {
	dir := t.TempDir()
	r := Ruling{
		HoldID:  "some-hold",
		Action:  Action("bogus"),
		RuledBy: "operator",
		RuledAt: time.Now(),
	}

	if err := Write(dir, r); err == nil {
		t.Fatal("expected error for invalid action, got nil")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected no file written for invalid ruling, found %d entries", len(entries))
	}
}

func TestRunHook_PipesJSONToStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook fixture is POSIX-only")
	}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "hook-out.json")
	scriptPath := filepath.Join(dir, "hook.sh")
	script := "#!/bin/sh\ncat > " + outPath + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil { //nolint:gosec // hook script must be executable
		t.Fatalf("write hook script: %v", err)
	}

	r := Ruling{
		HoldID:  "some-hold",
		Action:  Discuss,
		Note:    "needs a second opinion",
		RuledBy: "operator",
		RuledAt: time.Date(2026, 9, 1, 16, 20, 0, 0, time.UTC),
	}

	if err := RunHook([]string{scriptPath}, r); err != nil {
		t.Fatalf("RunHook returned error: %v", err)
	}

	data, err := os.ReadFile(outPath) //nolint:gosec // reads back the fixture this test's own hook script just wrote
	if err != nil {
		t.Fatalf("expected hook to write %s: %v", outPath, err)
	}

	var got Ruling
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("hook stdin was not valid ruling JSON: %v (data=%s)", err, data)
	}
	if got.HoldID != r.HoldID || got.Action != r.Action {
		t.Errorf("hook received ruling = %+v, want %+v", got, r)
	}
}

func TestRunHook_NoHookConfiguredIsNoOp(t *testing.T) {
	r := Ruling{HoldID: "some-hold", Action: Close, RuledBy: "operator", RuledAt: time.Now()}
	if err := RunHook(nil, r); err != nil {
		t.Fatalf("RunHook with no configured hook should be a no-op, got error: %v", err)
	}
}

func TestRunHook_NonZeroExitReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook fixture is POSIX-only")
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fail.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil { //nolint:gosec // hook script must be executable
		t.Fatalf("write hook script: %v", err)
	}

	r := Ruling{HoldID: "some-hold", Action: Close, RuledBy: "operator", RuledAt: time.Now()}
	if err := RunHook([]string{scriptPath}, r); err == nil {
		t.Fatal("expected error when hook exits non-zero, got nil")
	}
}

func TestReadResult_Found(t *testing.T) {
	dir := t.TempDir()
	holdID := "some-hold"
	resultJSON := `{"status":"executed","summary":"merged as-is"}`
	path := filepath.Join(dir, holdID+".result.json")
	if err := os.WriteFile(path, []byte(resultJSON), 0o600); err != nil {
		t.Fatalf("write result fixture: %v", err)
	}

	res, found, err := ReadResult(dir, holdID)
	if err != nil {
		t.Fatalf("ReadResult returned error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if res.Status != "executed" || res.Summary != "merged as-is" {
		t.Errorf("res = %+v", res)
	}
}

func TestReadResult_NotFound(t *testing.T) {
	dir := t.TempDir()
	res, found, err := ReadResult(dir, "no-such-hold")
	if err != nil {
		t.Fatalf("ReadResult returned error: %v", err)
	}
	if found {
		t.Fatalf("expected found=false, got result %+v", res)
	}
}

// TestRejectsPathTraversalHoldID guards handleSaveRulings' HTTP entry point:
// hold_id there comes straight from the request body, so Write, Read, and
// ReadResult must all refuse an ID that could walk a rulings-file path
// outside dir rather than silently joining it in.
func TestRejectsPathTraversalHoldID(t *testing.T) {
	dir := t.TempDir()
	escapeTarget := filepath.Join(filepath.Dir(dir), "escaped.json")
	defer func() { _ = os.Remove(escapeTarget) }()

	badIDs := []string{
		"../escaped",
		"a/../../escaped",
		"/etc/passwd",
		"",
		".",
		"..",
	}

	for _, id := range badIDs {
		r := Ruling{HoldID: id, Action: Proceed, RuledBy: "operator", RuledAt: time.Now()}
		if err := Write(dir, r); err == nil {
			t.Errorf("Write(%q): expected error, got nil", id)
		}
		if _, found, err := Read(dir, id); err == nil || found {
			t.Errorf("Read(%q): expected error, got found=%v err=%v", id, found, err)
		}
		if _, found, err := ReadResult(dir, id); err == nil || found {
			t.Errorf("ReadResult(%q): expected error, got found=%v err=%v", id, found, err)
		}
	}

	if _, err := os.Stat(escapeTarget); !os.IsNotExist(err) {
		t.Fatalf("traversal id escaped dir: %s exists", escapeTarget)
	}
}
