# Proposed MPR decision contract

This is the contract proposed from the first real-data trial. It describes
consumer and discussion work that is not connected yet. Hold Court currently
records action codes and notes; it runs an optional operator-configured hook.
None of the behavior below is implied by an action code alone.

| Intent | Proposed behavior | Maintainer input |
| --- | --- | --- |
| Accept MPR recommendation | Publish the prepared review for the held commit and resume MPR's recorded disposition. `fix-merge` means perform and verify the fixes first; `auto-merge` means proceed through the existing merge gates. | Show the actual verdict and review being accepted before saving. |
| Revise our review or fix plan | Ask an agent to revise the preparation and return it for another maintainer decision. This does not request changes from the contributor. | Required instructions describing what to revisit. |
| Request changes from the author | Publish a maintainer-approved request-changes review. This is distinct from revising our own preparation. | Show the exact proposed review text, allow edits, then confirm sending. |
| Close the PR | Close with a maintainer-approved explanation. An agent may draft the explanation from the review and note, but cannot silently invent and send it. | Show the exact closing message, allow edits, then confirm closing. |
| Discuss | Append a question to the hold's local conversation and route it to an agent. A reply supplies information and returns the decision to the maintainer; it does not authorize execution. | Required question or annotation. |

Accepting a recommendation must name what is being accepted. A generic
“Proceed” label obscures whether the prepared verdict asks for fixes, merging,
or another disposition. The consumer must recheck the current PR head and
standing hold before any action. A stale review cannot authorize a new commit.
The existing `changes` code means requesting PR changes; repurposing it for
revising our review would reinterpret saved decisions, so revision needs a
separate action or an explicitly versioned contract.

## Discussion and activity

The conversation needs a durable, append-only record of maintainer notes and
agent replies, with author, time, message identity, and the hold/review revision
each message addresses. The maintainer should see the dispatch lifecycle:

1. **Queued**: the message is saved and awaits an agent.
2. **In progress**: an agent has acknowledged the message.
3. **Reply ready**: the agent has posted a response in the same thread.
4. **Needs your decision**: the hold is back with the maintainer.

Failure to dispatch is a visible failure, not “In progress.” Retrying a message
must not create a second job. Reading a reply does not acknowledge replies that
arrive afterward. New replies belong in the Updates view and should make the
hold unread, with a specific Reply ready label rather than bold text alone.
Arrival must preserve the hold currently being read and the note being typed.

The live UI already signals review/result changes and preserves drafts. The
thread model, delivery acknowledgements, action previews, and agent consumer
remain separate implementation work tracked in Beads.
