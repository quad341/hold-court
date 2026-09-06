# MPR decision contract

The default configuration records decisions locally. The optional
[MPR adapter](../adapters/mpr/README.md) turns newly confirmed decisions into
Gas City tasks and brings their acknowledgement and replies back here.

| Choice | Authorized behavior | Required input |
| --- | --- | --- |
| Proceed with guidance (`proceed`) | Resolve the hold using the operator response, the proposed disposition, and the evidence. Explain the selected path before carrying it out through existing checks. `fix-merge` requires fixes and verification first. Report ambiguity instead of guessing a continuation. | Response stating direction, reason, and conditions; confirmation naming the PR and held commit. |
| Request author changes (`changes`) | Compose and post a request-changes review from the operator intent, annotations, and review context using the repository maintainer workflow. Report self-review or policy blockers. | Response to the hold and confirmation. |
| Close PR (`close`) | Establish the rationale from context and compose an appropriate closing explanation. If unclear, ask before closing. | Response to the hold and confirmation. |
| Discuss (`discuss`) | Investigate the question and reply in the local conversation. No GitHub comment, hold clearance, or merge is authorized. Use this to ask for revisions to our preparation too. | Question or requested investigation and confirmation. |
| Clear choice | Remove an unsaved selection, retaining the note. No task is sent. | None. |

Rulings express intent. Annotations are instructions to an agent, not final
correspondence. The agent improves grammar, tone, and clarity without requiring
another approval round for ordinary wording. Verbatim delivery requires an
explicit operator instruction. The agent must preserve meaning, avoid invented
rationales, and return to discussion rather than silently choose a different
consequential action.

Every newly saved ruling requires a response to the hold, including Proceed
and record-only saves. A category button is not a resolution of an ambiguity.
State which path the agent should take, why, and any conditions or required
fixes. The browser, server, and queue hook reject blank/whitespace responses.
Existing queued requests remain observable; this change does not erase earlier
records or prevent replies from arriving.

The MPR verdict and contributor message are proposals. Proceed with guidance
instructs the agent to reconcile your response with the evidence and adapt the
draft; it does not mean silently accepting every conclusion or publishing the
draft unchanged. For example: “Proceed after fixing the silent skip and testing
the warning; preserve the fail-closed behavior.”

If that direction is still materially unclear, the agent posts its interpretation
and a focused question, reports `needs_clarification`, and takes no external
action until answered. The original ruling and response stay in history.
Answer through Discuss or submit a clarified ruling when ready to authorize
execution. The agent records its interpretation, outgoing wording, and outcome.

## Evidence for the human decision

The Review tab must explain the ambiguity, not merely name its category.
The MPR exporter reads the run matching the held commit and presents:

- Synthesis disagreement notes and findings: what conflicts, and how synthesis
  reconciled it or why a human decision remains.
- Each reviewer's verdict, reasoning, proposed fixes, and correctness risks,
  attributed to that review's source file.
- Contract/behavior-change evidence and existing/proposed contract sections
  when provided. Absent before/after descriptions are explicitly marked missing.
- Actual reviewer/stage error output and failed precheck diagnostics when
  available; an error label without its diagnostic is insufficient evidence.
- The proposed disposition, fix plan, and contributor-facing message draft.

These are source reports, not independently verified conclusions or a newly
invented reconciliation. Missing reviewer output or explanation is visible and
should be requested through Discuss when necessary to decide. Historical review
versions retain this context alongside the draft message.

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
- **Needs clarification**: intent is unclear; execution is paused and a focused
  question awaits an operator reply in History & discussion.
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
