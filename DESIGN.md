# Hold Court — design

*Status: v1 design, agreed 2026-09-01. Iterating in the open; sections marked
(later) are declared non-goals for v1 so the first playable build ships fast.*

## The problem

Automated review pipelines (like
[maintainer-pr-review](https://github.com/quad341/maintainer-pr-review)) do the
bulk reading, but the interesting PRs end held for a human: a genuine
ambiguity, an operational policy call, a guard change nobody should wave
through silently. Those holds land as notification mail. Mail gets buried; a
measured backlog on the first corpus this tool was built against: 114 held PRs,
of which 54 had already merged or closed while their hold mail sat unread.

The insight from working that backlog by hand: **each hold reduces to one
operative question.** Rendered as a card with the prepared review beside it, a
maintainer can rule on most holds in seconds. The first paper prototype (a
self-saving web page with 20 cards) cleared 6 rulings in its first ten minutes
of use.

## Shape

One pure-Go binary, web UI embedded. `hold-court serve` prints a localhost URL
and opens the bench. No external services; state is a single SQLite file
(pure-Go driver, no cgo) next to the config. Builds and runs identically on
macOS and Linux.

Deployment modes:
- **Personal**: run it on your machine against your own feed.
- **Hosted** (later): one instance behind your own reverse proxy for a
  maintainer team; per-user identity via GitHub OAuth.

## UI: mutt, as a web page

Three panes, with the list and reading pane stacked so long titles have room:

```
+------------+---------------------------------------------------------+
| FOLDERS    | HOLD LIST: full titles, wrapping when needed            |
| Inbox  12  | > Push-tier relaxation for release branches             |
| Ruled   4  |   owner/repo #5795 · held date                           |
| Executed 6 |---------------------------------------------------------|
| ----       | READING PANE: question, prepared review, saved decision  |
| guard    2 | and consumer result. History and discussion have a separate tab.  |
| policy   3 |                                                         |
| scope    5 | [ruling bar + note + explicit execution mode]           |
+------------+---------------------------------------------------------+
```

- Unread semantics come from email: a hold you have not opened is bold; a hold
  updated since you last read it (new head, new discussion) re-bolds.
- Folders are virtual: state folders (inbox / ruled / executed / stood-down)
  and class folders (from the feed's `class` field).
- The reading pane renders the operative question first, then the full
  prepared review body (markdown), then the discussion thread, then the ruling
  bar. The PR link is always one keypress away.

### Keybindings (vim grammar, non-negotiable)

| Key | Action |
|-----|--------|
| `j` / `k` | next / previous hold in list |
| `gg` / `G` | first / last hold |
| `Ctrl-d` / `Ctrl-u` | half-page scroll in reading pane |
| `Enter` or `l` | open selected hold (focus reading pane) |
| `h` | back to list |
| `Tab` / `Shift-Tab` | cycle folders |
| `/` | filter/search holds; `n`/`N` next/prev match |
| `p` | rule: proceed |
| `c` | rule: request changes |
| `x` | rule: close |
| `d` | rule: discuss |
| `i` | annotate (note field; `Esc` returns to normal mode) |
| `u` | toggle read/unread |
| `o` | open PR on GitHub |
| `s` | save pending rulings |
| `?` | key cheatsheet overlay |

Mouse works everywhere; keys are the fast path. A visible pending-rulings bar
mirrors the count (`s` to commit), so partial work is never silently lost.

The browser polls `/api/holds` every five seconds using ETag revalidation.
Updates preserve selection, scroll, and the textarea DOM. Changed content for
the active hold is offered through Show update; other changes appear in Updates.
Read acknowledgements include the displayed content revision, so a new result
cannot be swallowed by a delayed acknowledgement of an earlier view. Pending
rulings and notes are backed up to local storage for the current server origin.

The MPR action meanings and discussion lifecycle need an explicit consumer
contract; see [the proposed decision flow](docs/mpr-decision-flow.md).

## Feed contract (v0)

Hold Court is fed by a directory (or later, an HTTPS endpoint) of JSON
documents, one per hold — the **feed**. Any pipeline can write it; the
maintainer-pr-review exporter is simply the first adapter.

```jsonc
// feed/<id>.json
{
  "id": "gastownhall-gascity-5795-a1b2c3",   // stable per hold+head
  "source": "maintainer-pr-review",           // adapter name
  "repo": "gastownhall/gascity",
  "pr": 5795,
  "url": "https://github.com/gastownhall/gascity/pull/5795",
  "class": "ambiguous-needs-discussion",      // folder key
  "title": "Push-tier relaxation",
  "question": "The one operative question, <= ~80 words.",
  "review_body_md": "... full prepared review, markdown ...",
  "verdict": "fix-merge",                     // pipeline's recorded verdict, if any
  "head_sha": "abc123...",
  "held_at": "2026-09-01T15:00:00Z",
  "resolved": false,                          // adapter sets true when OBE
  "resolved_reason": ""                       // "merged", "closed", ...
}
```

Rules:
- The adapter owns the feed dir; Hold Court treats it read-only and re-scans on
  fsnotify + interval.
- A changed `head_sha` on the same PR is a NEW hold document (new id); the old
  one should be marked resolved by the adapter. This mirrors how review
  pipelines re-derive holds per head.
- `resolved: true` holds auto-move out of the inbox (the "54 already-merged
  PRs buried the queue" lesson, mechanized).

## Rulings out

A ruling writes `rulings/<hold-id>.json`:

```jsonc
{
  "id": "stable request hash",
  "repo": "owner/repo",
  "pr": 42,
  "head_sha": "exact reviewed commit",
  "hold_id": "...",
  "action": "proceed" | "changes" | "close" | "discuss",
  "note": "free text",
  "ruled_by": "operator",        // local identity; OAuth login later
  "ruled_at": "2026-09-01T16:20:00Z"
}
```

plus an optional configured hook: `on_ruling = ["/path/to/cmd"]` receives the
JSON on stdin. The executing automation (a human's script, a CI job, or an AI
agent session) consumes rulings and reports back by writing
`rulings/<hold-id>.result.json` (`{"status": "executed", "summary": "..."}`),
which Hold Court renders on the card and moves the hold to Executed.

This is deliberately symmetrical: Hold Court never executes anything itself.
It is the bench, not the bailiff.

## Discussion + read state

SQLite retains read acknowledgements and an append-only `hold_history` log
of observed review, decision, result, and conversation revisions. Incoming
activity has a separate revision from the operator's own saved choice. The
History & discussion tab presents the conversation and expandable versions;
versions not observed by the server cannot be reconstructed.

Consumers correlate results with `ruling_id` and publish messages in
`rulings/<hold-id>.thread.json` as `{"messages": [{"id": "...", "author": "...",
"body": "...", "at": "RFC3339 time"}]}`. Messages from superseded decisions
remain visible; their results cannot overwrite the current decision's status.
See the [MPR decision contract](docs/mpr-decision-flow.md) for the optional agent
adapter and its execution boundaries.

## v1 scope (the playable build)

1. Feed scan + three-pane UI + vim keys + unread.
2. Rule + note + save -> rulings dir + hook.
3. Result files render as Executed.
4. Single user, no auth, localhost only.

**v1.1**: discussion threads + on_comment hook. **v1.2**: search, multi-feed
folders. **v2 (later)**: GitHub OAuth, hosted mode, public read-only sharing,
GitHub-labels adapter for pipelines that have no feed writer.

## Non-goals

- Not a review tool: it renders the pipeline's prepared review; it does not
  diff code.
- Not an executor: rulings are data out; execution is the consumer's job.
- Not a notification service: it is the thing notifications should have been.
