package server

// KeyBinding is one row of the vim keybinding table from DESIGN.md
// ("Keybindings (vim grammar, non-negotiable)"). It doubles as the content
// of the in-app `?` cheatsheet overlay, so the table has exactly one
// source of truth.
type KeyBinding struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

// Keybindings is the exact vim keybinding table from DESIGN.md. Changing a
// row here changes both the `?` overlay and this package's contract test —
// do not edit without updating DESIGN.md first.
var Keybindings = []KeyBinding{
	{"j / k", "next / previous hold in list"},
	{"gg / G", "first / last hold"},
	{"Ctrl-d / Ctrl-u", "half-page scroll in reading pane"},
	{"Enter or l", "open selected hold (focus reading pane)"},
	{"h", "back to list"},
	{"Tab / Shift-Tab", "cycle folders"},
	{"/", "filter/search holds; n/N next/prev match"},
	{"p", "rule: proceed"},
	{"c", "rule: request changes"},
	{"x", "rule: close"},
	{"d", "rule: discuss"},
	{"i", "annotate (note field; Esc returns to normal mode)"},
	{"u", "toggle read/unread"},
	{"o", "open PR on GitHub"},
	{"s", "save pending rulings"},
	{"?", "key cheatsheet overlay"},
}
