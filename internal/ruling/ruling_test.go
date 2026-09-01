package ruling

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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
	data, err := os.ReadFile(path)
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
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
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

	data, err := os.ReadFile(outPath)
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
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
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
	if err := os.WriteFile(path, []byte(resultJSON), 0o644); err != nil {
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
