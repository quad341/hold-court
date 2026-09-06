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

## Run locally

Install Go 1.26.6 or newer (see [go.mod](go.mod)) and GNU Make, then run
this from the repository root:

```sh
make run
```

Open the localhost URL printed in the terminal. The server chooses a free
port; stop it with `Ctrl-C`. Go downloads dependencies on the first run.
No configuration file or separate build step is required.

By default, Hold Court reads holds from `./feed`, writes decisions to
`./rulings`, and stores read state in `./hold-court.db`. A missing feed
directory starts an empty inbox. Point it at your pipeline's feed to see
holds; the JSON format is in [the feed contract](DESIGN.md#feed-contract-v0).

Pass server flags through `ARGS`:

```sh
make run ARGS='-feed /path/to/feed -rulings /path/to/rulings -addr 127.0.0.1:8080'
make run ARGS='-help'
```

For persistent feed paths and an optional ruling hook, create
`holdcourt.toml` in the repository root:

```toml
feed = "/path/to/feed"
rulings = "/path/to/rulings"
# on_ruling = ["/path/to/consumer", "--some-option"]
```

Explicit command-line flags override the configuration file. Continue to
launch with `make run`; consumers receive each ruling as JSON on stdin.

To connect this checkout to the local MPR/Gas City workflow on Linux:

```sh
make connect-mpr CITY=/path/to/your/city TARGET=mayor
make run
```

`CITY` is the Gas City root containing `.gc` and its HQ Beads database, not
a particular repository name. It is required; setup discovers its issue prefix.
`TARGET` names the receiving agent/session in that city (default: `mayor`).

This is an **opt-in execution connection**: newly confirmed decisions enqueue
agent tasks. Existing trial rulings are never replayed. It installs user timers
for feed export and task/reply synchronization. Requires Python 3.11+, systemd,
`bd`, `gc`, authenticated `gh`, and an active target in the city's HQ database.
See [MPR setup and operation](adapters/mpr/README.md) before enabling it.

## Common tasks

Run `make` or `make help` to list the available commands.

| Command | Task |
| --- | --- |
| `make run` | Start the local web UI |
| `make build` | Build the `./hold-court` binary |
| `make install` | Install the binary into `GOBIN`, or `GOPATH/bin` by default |
| `make connect-mpr` | Enable MPR export and confirmed agent handoffs (`CITY=... TARGET=...`) |
| `make test-adapters` | Test export and handoff using isolated fixtures |
| `make test` | Run all Go tests |
| `make test-race` | Run tests with the race detector |
| `make screenshots` | Refresh the dark-theme README screenshots using synthetic data (requires Python Playwright) |
| `make test-browser` | Run browser regressions (requires Python Playwright and its Chromium browser) |
| `make vet` | Run Go's static checks |
| `make fmt` | Format Go source |
| `make fmt-check` | Check formatting without modifying source |
| `make tools` | Install the same golangci-lint version used by CI |
| `make lint` | Run golangci-lint |
| `make check` | Build, check formatting, vet, race-test, lint, and test adapters |
| `make clean` | Remove the built binary; keep feeds, rulings, and database |

For development, run `make tools` once and ensure your Go binary installation
directory (`GOBIN`, or `GOPATH/bin`) is on `PATH`. Then `make check` runs the
checks used by CI, including Python adapter tests. Race tests require cgo enabled and a C compiler; ordinary
builds and `make test` do not require cgo. You can override tool commands with
`GO` and `GOLANGCI_LINT`, for example `make lint GOLANGCI_LINT=/path/to/golangci-lint`.

The optional browser checks exercise live arrivals while typing, draft recovery,
save failures, and result updates using temporary data. Install Python's
`playwright` package in your development environment and its Chromium browser
(`python -m playwright install chromium`), then run `make test-browser`.
Use `PYTHON=/path/to/python` to select that environment.

## Working through holds

Folders stay on the left. The hold list sits **above** the reading pane and uses
the remaining width; titles wrap, with repository and PR number on a separate
line alongside the PR author. Missing authors are shown as unknown. The browser checks the feed and ruling results every five seconds.
The header shows connection status and an **Unread updates** button for incoming activity on submitted holds.
An adapter may refresh its source less frequently; the browser reflects the
latest files the adapter has written.

The page is a small shell; hold contents arrive from `/api/holds` as one
JSON document, gzip-compressed when the browser accepts it. This browser keeps
the last document in IndexedDB and shows it immediately on the next visit,
then revalidates against the server. The header shows the feed data version
and when this browser fetched it. When the feed on the server changes, the
page refetches and shows a dismissable notice naming the old and new versions
and the time. **Rebuild cache** drops the local copy, makes the server re-read
the feed directory, and shows the same notice. It only re-reads data, so it is
always safe; drafts and pending rulings are untouched.

**Inbox** contains holds awaiting their first saved decision, including holds
that have never been read. Saving any ruling, including Discuss, moves its row
to **Pending** immediately. Pending includes queued work, discussions, requests
for clarification, and failures; completed actions move to **Executed**.
Unsaved choices remain in Inbox.

**Unread updates** collects submitted holds with activity you have not read:
review changes, agent acknowledgements, replies, and completion. It is also a
folder on the left, and its count survives page reloads. New untouched holds
stay in Inbox; your own submissions do not count as incoming updates. Opening
an update removes it from this view while keeping the document open below.
A later reply brings it back, even after execution.

Arrivals preserve your selected hold, scroll position, and note. When the hold
you are reading changes, choose **Show update** when you want to load that
revision. Other changed holds get an Updated label; previously read holds
become unread when their review or result changes. Reading the new revision
acknowledges it. When a row moves out of the active folder after saving or
reading, its document stays open until you navigate away. Unsaved choices and notes are backed up in this browser's
local storage for this server URL. Save failures remain visible and keep drafts.

The visible search bar shows the active folder, query, match count, and fields
searched. Press `/` to focus it. **Summary fields** searches title, question,
repository, PR number, class, and author; it excludes review text and history.
Choose **Author** for a case-insensitive login substring search (an initial `@`
is optional), or type `author:login` for an exact login match. For example,
`author:alice timeout` finds Alice's holds whose summary fields contain
“timeout”. Choose **PR number** to match an exact number, or use `#6004` / `pr:6004`
in the search box. In Summary fields mode, combine with a repository name (for example,
`pr:6004 gastownhall/gascity`) or `author:login` to narrow the result.
Filters remain active when switching folders; **Clear** removes the
query. Draft notes survive filtering, including zero-result searches.

Drag the thick dividers to resize the folders and list; focused dividers also
support arrow keys and Home to reset. Sizes persist in this browser. The action
area stays at the bottom while review text scrolls above it. Clicking the selected
action again, **Clear choice**, or Escape outside the note removes an unsaved
choice and keeps your note. This sends nothing; it does not cancel an already
submitted task.

**History & discussion** separates the conversation from the current review.
It retains observed review versions, decisions, results, and replies with times.
Versions expand to show their original text; there is no automatic diff yet.
History starts when this server observes a hold; it cannot recover previously
overwritten source files. Your own saves do not count as incoming activity.
Agent acknowledgement, replies, and review changes do.

The Review tab puts decision evidence ahead of the proposed contributor message:
the synthesis's disagreement explanation, each reviewer's reasoning and fixes,
contract-change evidence, and relevant errors. Missing explanations or old/new
contract descriptions are explicitly marked; use Discuss to request what you
need. A verdict such as `fix-merge` alone does not explain what needs fixing.

Without an `on_ruling` hook, the app explicitly runs in **record-only** mode:
actions save local decisions only. With the MPR connection, Save previews the
PR, reviewed head, intended action, and annotations before sending. **Discuss** requests
analysis and a reply here. **Proceed with guidance** asks the agent to resolve the hold using your response
and the displayed evidence, then carry out that direction through its checks.
**Request author changes** and **Close PR** ask the agent to compose appropriate
messages from your intent, annotations, and review context. Every action requires a response to the hold: direction, reason, and conditions.
That response is guidance, not publication-ready text. The agent improves wording and tone;
verbatim delivery requires an explicit instruction. Unclear intent returns as
**Needs clarification**, with a question and the original ruling preserved.
See the [decision contract](docs/mpr-decision-flow.md) for execution and status
semantics. The MPR exporter excludes requests to split oversized PRs from the
human inbox while retaining their source artifacts.

## Screenshots

The search bar shows scope and counts above the stacked hold list and reader.
PR authors appear alongside repository details and below the reading title.

![Dark-theme inbox with author metadata, search scope, and stacked panes](docs/images/inbox.png)

History & discussion keeps previous decisions and agent replies beside the
current status, with the action area docked at the bottom.

![Dark-theme reading pane with decision history and an agent reply](docs/images/reading-pane.png)

`?` shows the key cheatsheet; `/` focuses search.

![Dark-theme keyboard cheatsheet overlay](docs/images/keys.png)

These screenshots use synthetic data and the dark theme. The app follows your
system theme automatically. Regenerate all three with `make screenshots` using
the same Python Playwright environment as `make test-browser` (override `PYTHON`
if needed). Keep these images and their descriptions current when changing the UI.

## Shape

- One binary: `make run` starts a local web UI. No daemon ceremony.
- mutt-inspired three-pane UI: folders (hold classes), list (unread bold),
  reading pane (the question, the prepared review, the discussion).
- Keyboard-first: j/k to move, one key to rule, i to annotate.
- Holds arrive via a small JSON feed contract; rulings leave as JSON plus an
  optional on-ruling hook, so any automation (including an AI agent) can
  execute what you decide.
- SQLite for discussion threads and read-state. Pure Go, no cgo — builds for
  macOS and Linux with `make build`.

## License

MIT
