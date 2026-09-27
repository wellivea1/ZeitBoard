# ADR 0051: Proposals that wait for the owner: doses and new tasks

- Status: accepted
- Date: 2026-09-27; extended the same day from doses to new tasks.
- Implements the dose-logging proposal of `medication-feature-plan.md` §4 (slice M-E) and the
  task operations of completion-plan C4.
- Builds on [ADR-0050](0050-agent-action-registry.md) (the action registry),
  [ADR-0028](0028-desktop-local-agent-endpoint.md) (the local agent endpoint),
  [ADR-0027](0027-local-clinician-context-report.md) (adherence counts only explicit marks)
  and [ADR-0048](0048-medication-and-marker-sync.md) (medication sync and erasure).

## Context

An owner using a voice assistant or another agent could ask it about their day. They could not
tell it either of two things:

- "I just took my evening tablet";
- "Remind me to call the pharmacy, fifteen minutes, before Friday".

The medication plan requires agent-assisted logging to land as a pending confirmation, because a
dose is a health record and the owner confirms it. A new task is the owner's plan, and it should
be added the same way.

The existing proposals could not carry either:

- the server's scheduler resolves them, and the server has no reason to receive a dose or a
  task title from an agent;
- they change when an existing task happens, and name that task;
- they are approved with a one-use token on the server, while doses and tasks are made on the
  owner's computer.

## Decision

1. **Proposals have a subject.** The registry gives each proposal one:
   - schedule proposals (`propose_move_task`, `propose_place_task`, `propose_reminder_shift`)
     name an existing task, and the server's scheduler resolves them wherever they arrive;
   - `propose_log_dose` names a dose, and `propose_add_task` a new task. Both wait on the owner's
     computer.

   The server refuses a proposal that waits on the computer, even one dressed with a task target.
   The chat assistant and the server's connector never offer one: the chat model sees no
   medication or titles, and the server resolves only schedule changes.
2. **What an agent can say.**
   - A dose proposal names a medication by the opaque id that `get_snapshot` and
     `get_medication_timing` list; taken or skipped; and optionally when, at most a week ago and
     not in the future. It carries no label, note, amount or "scheduled" mark.
   - A task proposal names a one-line title in the owner's words (1 to 120 characters), the
     minutes it takes (5 to 720), and optionally the earliest start and latest finish. The latest
     finish must still be ahead. It carries no task id and no notes: it adds a new task and
     reshapes none.

   The strict decoder refuses any other field, and the result names only the proposal's id.
   Titles an agent writes are never read back to an agent.
3. **One queue.** The desktop keeps both kinds in `local_agent_proposals`:
   - at most 20 wait at once, of either kind, and each lapses after a day;
   - they are never synced or exported;
   - erasing a medication erases its dose proposals;
   - decided and lapsed proposals leave the history after 30 days.
4. **The owner decides.** A proposal appears:
   - under "Waiting on you" on Home;
   - in Plan's decision queue, with the other proposals, under the same Accept and Decline;
   - in the shared pending count.

   Accepting makes, in one transaction, the record the owner's own entry would make:
   - a dose becomes the medication event a hand-logged dose would be: manual, user-reported,
     never marked scheduled (ADR-0027). It syncs, and appears in the history and the clinician
     report like any dose.
   - a task becomes an open task at its first revision, planned like any task: a suggested time
     follows for the owner to accept.

   Declining makes nothing.
5. **Agents see the outcome, not the record.** The snapshot counts the proposals still waiting:
   doses in its medication section, tasks in its tasks section. An agent learns about a recorded
   dose or an added task the way it learns about any other, through the same projections.

## Consequences

- An agent can relay "I took it" or "remind me to…" without being able to write health data or
  the owner's plan.
- The desktop's zone at the time of a dose proposal is the recorded dose's zone. The desktop names
  that zone from the system (`core/platform/localzone`). Until 2026-09-27 it named New York on
  every Windows computer, which also affected quick logging.
- Completing, editing or deleting a task stays the owner's: an agent cannot name a task by its
  title, and editing and deleting are not voice-first acts.
- The server and its connector are unchanged in what they accept, apart from the explicit
  refusal of proposals that wait on the owner's computer.
