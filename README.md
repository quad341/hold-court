# Hold Court

[![CI](https://github.com/quad341/hold-court/actions/workflows/ci.yml/badge.svg)](https://github.com/quad341/hold-court/actions/workflows/ci.yml) [![Go version](https://img.shields.io/github/go-mod/go-version/quad341/hold-court)](go.mod) [![Go Reference](https://pkg.go.dev/badge/github.com/quad341/hold-court.svg)](https://pkg.go.dev/github.com/quad341/hold-court) [![License](https://img.shields.io/github/license/quad341/hold-court)](LICENSE)

A decision bench for maintainers. When an automated review pipeline holds a PR
for a human — an ambiguity worth a ruling, a policy call, a guard nobody should
relax silently — those holds pile up in mail nobody reads. Hold Court turns
them into something you can actually work: an inbox of holds, each reduced to
the one question it actually asks, with the prepared review alongside and a
ruling a keypress away.

Reads like email. Rules like a bench.

**Status: early.** Being built in the open; the design is in
[DESIGN.md](DESIGN.md). Companion to
[maintainer-pr-review](https://github.com/quad341/maintainer-pr-review), whose
hold queue is the first feed source — but the feed contract is deliberately
tool-agnostic.

## Shape

- One binary: `hold-court serve` opens a local web UI. No daemon ceremony.
- mutt-inspired three-pane UI: folders (hold classes), list (unread bold),
  reading pane (the question, the prepared review, the discussion).
- Keyboard-first: j/k to move, one key to rule, n to annotate.
- Holds arrive via a small JSON feed contract; rulings leave as JSON plus an
  optional on-ruling hook, so any automation (including an AI agent) can
  execute what you decide.
- SQLite for discussion threads and read-state. Pure Go, no cgo — builds for
  macOS and Linux with `go build`.

## License

MIT
