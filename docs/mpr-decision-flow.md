# MPR decision contract

The default configuration records decisions locally. The optional
[MPR adapter](../adapters/mpr/README.md) turns newly confirmed decisions into
Gas City tasks and brings their acknowledgement and replies back here.

| Choice | Authorized behavior | Required input |
| --- | --- | --- |
| Accept recommendation (`proceed`) | Inspect the prepared review for the held commit and resume the recorded MPR disposition through existing checks. `fix-merge` requires fixes and verification first. Report ambiguity instead of guessing a continuation. | Confirmation naming the PR, held commit, and verdict. |
| Request author changes (`changes`) | Post a request-changes review using the exact approved note and repository maintainer workflow. Report self-review or policy blockers. | Exact review text and confirmation. |
| Close PR (`close`) | Close with the exact approved explanation through the repository maintainer workflow. | Exact closing message and confirmation. |
| Discuss (`discuss`) | Investigate the question and reply in the local conversation. No GitHub comment, hold clearance, or merge is authorized. Use this to ask for revisions to our preparation too. | Question/instructions and confirmation. |
| Clear choice | Remove an unsaved selection, retaining the note. No task is sent. | None. |

Save validates a content revision and generates a stable request ID. Retrying
the same request reuses its queue entry and task identity. Existing trial
rulings are not scanned for dispatch. Before routing, the worker checks that the
PR remains open at the held head and that the decision has not been superseded.
The agent is also instructed to repeat those checks before any external
mutation. Clearing a draft does not revoke a submitted task. A newer submitted
decision supersedes the previous one, but cannot undo work already performed.

The consumer creates a scoped agent task, rather than directly implementing
GitHub operations. The agent must follow repository policy and provide evidence
of the requested outcome. A successful MPR `clear-hold` exit alone is not proof
of publication or merge: notice-only holds can produce a no-op. Execution
status is agent-reported; the adapter does not independently verify each effect.

## Discussion and activity

The worker observes the task and publishes these states:

- **Queued**: saved, awaiting dispatch or agent acknowledgement.
- **In progress**: the agent claimed the task.
- **Reply ready**: the agent explicitly reports a completed discussion reply.
- **Needs decision**: the agent needs input, the PR head changed, or a task was
  closed without an explicit outcome.
- **Executed**: the agent reports the authorized action completed.
- **Failed**: dispatch/synchronization failed, or the agent reports failure.
  Transport failures are retried and remain visible.

Replies come from task comments, notes, and closing explanations. They retain
message identity, author, and time. Late replies to an older decision remain in
the conversation without overwriting a newer decision's status. Reading a
reply does not acknowledge a later reply. Saving a question is not itself an
incoming update.

The browser polls every five seconds. Review changes, acknowledgement, and
replies appear in Updates without replacing the review or note being read.
Choose Show update, then History & discussion to read the conversation or
expand previously observed review versions. History is an observation log in
SQLite; it starts when this server sees each revision and cannot recover
intermediate versions written while the server was offline. It displays full
historical text, not computed diffs.
