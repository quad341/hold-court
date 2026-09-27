package server

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// extractEscapeHTML pulls the escapeHTML function's source verbatim out of
// the shipped static/app.js, so the test below exercises the real
// client-side code rather than a reimplementation of it.
func extractEscapeHTML(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("reading static/app.js: %v", err)
	}
	re := regexp.MustCompile(`(?s)\n\tfunction escapeHTML\(s\) \{.*?\n\t\}\n`)
	m := re.Find(src)
	if m == nil {
		t.Fatal("could not locate escapeHTML() in static/app.js — has it moved or been renamed?")
	}
	return string(m)
}

// TestEscapeHTML_SafeForDoubleQuotedAttribute runs the real, shipped
// escapeHTML() (extracted from static/app.js) under Node, with a minimal
// shim of the one browser behavior it depends on: document.createElement +
// textContent -> innerHTML encodes &, <, > only (standard, specified DOM
// behavior, not a browser quirk -- safe to shim exactly).
//
// renderFolders() and renderList() both place escapeHTML(...) inside
// double-quoted HTML attributes (data-folder-id ~182, data-hold-id ~214)
// built from external, curator/operator-authored text (see server.go's
// folderJSON{ID: "group:"+g}). Both call sites share this one function, so
// covering it once covers both. escapeHTML must encode the quote
// characters too, or a value containing one breaks out of the attribute --
// this is the round-1 review security blocker on hc-jgb.
func TestEscapeHTML_SafeForDoubleQuotedAttribute(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; skipping JS-level escapeHTML regression test")
	}

	fn := extractEscapeHTML(t)

	cases := []struct {
		name string
		in   string
	}{
		{"plain", "plain-name"},
		{"ampersand_lt_gt", "a & b < c > d"},
		{"double_quote_breakout", `x" onclick="alert(1)`},
		{"single_quote_breakout", `x' onmouseover='alert(1)`},
	}
	inputs := make([]string, len(cases))
	for i, c := range cases {
		inputs[i] = c.in
	}
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		t.Fatalf("marshal inputs: %v", err)
	}

	script := `
"use strict";
function fakeDiv() {
	var text = "";
	return {
		set textContent(v) { text = String(v); },
		get textContent() { return text; },
		get innerHTML() {
			return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
		}
	};
}
var document = { createElement: function () { return fakeDiv(); } };
` + fn + `
var inputs = ` + string(inputsJSON) + `;
console.log(JSON.stringify(inputs.map(escapeHTML)));
`

	cmd := exec.Command(node, "-e", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("node execution failed: %v\nstderr: %s", err, stderr.String())
	}

	var got []string
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("parsing node output %q: %v", stdout.String(), err)
	}
	if len(got) != len(cases) {
		t.Fatalf("expected %d outputs, got %d: %v", len(cases), len(got), got)
	}

	if got[0] != "plain-name" {
		t.Errorf("escapeHTML(%q) = %q, want unchanged", cases[0].in, got[0])
	}
	if want := "a &amp; b &lt; c &gt; d"; got[1] != want {
		t.Errorf("escapeHTML(%q) = %q, want %q", cases[1].in, got[1], want)
	}
	for _, i := range []int{2, 3} {
		escaped := got[i]
		if strings.ContainsAny(escaped, `"'`) {
			t.Errorf("escapeHTML(%q) = %q — still contains a raw quote character; unsafe inside data-folder-id=\"...\"/data-hold-id=\"...\" attributes (stored XSS via attribute breakout)", cases[i].in, escaped)
		}
		if !strings.Contains(escaped, "alert(1)") {
			t.Errorf("escapeHTML(%q) = %q — unexpectedly dropped surrounding text", cases[i].in, escaped)
		}
	}
}
