# Local MPR connection

From the checkout, on a Linux machine with user systemd:

```sh
make connect-mpr CITY=../gc-management TARGET=mayor
make run
```

Requires Python 3.11+, `bd`, `gc`, authenticated `gh`, MPR artifacts under
`CITY/.gc/maintainer-pr-review`, and an active Gas City target. This initial
adapter targets the management city's HQ database with the `gm` issue prefix.
Run it under the same account/environment as that city. Repository scope is
reused from an existing exporter config or discovered from notice metadata.
To specify scope explicitly, use the installer's `--repo owner/repo` options.

The command installs scripts and configuration under
`$XDG_DATA_HOME/hold-court/mpr` (default `~/.local/share/hold-court/mpr`), writes
local `holdcourt.toml`, and enables two user timers:

- `hold-court-mpr-feed.timer`: refresh the feed every five minutes.
- `hold-court-mpr-worker.timer`: dispatch requests and collect replies every
  fifteen seconds after the previous worker run finishes.

Restart `make run` after setup. Each subsequent Save previews the decisions and
asks for confirmation before enqueueing them. Setup never replays old ruling
files. A previously configured queue remains durable across reinstalls.
Only enable this when you want new confirmed choices delivered to the agent.
The [decision contract](../../docs/mpr-decision-flow.md) defines each action.

## Operation

The hook writes an atomic request to `requests/`; a separate worker creates a
deterministically named Beads task and routes it with `gc sling`. The worker
checks the current PR head and current decision before dispatch. The agent
receives the exact scope, held commit, note, and instructions for returning its
answer through Beads. Agent task execution and latency depend on the target;
queueing does not mean an action completed.

Inspect synchronization with:

```sh
systemctl --user status hold-court-mpr-feed.timer hold-court-mpr-worker.timer
journalctl --user -u hold-court-mpr-worker.service -n 50
```

The status shown by Hold Court includes the Beads task ID. Task comments and
notes become conversation messages. The agent sets metadata
`holdcourt.outcome` to `reply_ready`, `executed`, `needs_decision`, or `failed`.
Closing a task without that outcome is shown as needing your decision.
A worker error remains visible and is retried. Queue entries are retained for
history and reply correlation; do not remove them while tasks are active.

To stop dispatch and reply polling:

```sh
systemctl --user disable --now hold-court-mpr-worker.timer
systemctl --user stop hold-court-mpr-worker.service
```

Remove `on_ruling` and `consumer_description` from local `holdcourt.toml` and
restart `make run` to return the UI to record-only mode. Stopping the worker does
not cancel tasks already delivered. Existing queued requests remain and will
resume if the timer is re-enabled.

## Feed filtering and validation

The exporter classifies MPR notices against current GitHub state. It excludes
`skip-too-large` / `too-large` outcomes: the split request is already the
resolution, so these do not become liftable human holds. Previously exported
oversized holds move into the sibling `excluded-too-large` directory; original
MPR artifacts remain untouched. `status.json` reports counts and errors.

`make test-adapters` uses temporary fixtures and fake CLI responses. It checks
filtering, duplicate saves, changed heads, superseded decisions, acknowledgements,
replies, and late results. `make test-browser` covers the UI-to-queue path without
running a worker or sending an agent task. These checks do not establish that a
live agent has completed a real PR operation.
