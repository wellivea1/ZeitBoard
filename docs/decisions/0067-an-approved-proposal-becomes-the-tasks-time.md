# ADR 0067: An approved proposal becomes the task's time

- Status: accepted
- Date: 2026-10-25
- Builds on [ADR-0016](0016-approvals-unification.md),
  [ADR-0023](0023-local-calendar-ownership-and-adapters.md),
  [ADR-0046](0046-unified-review-queue-and-exact-request-times.md) and
  [ADR-0047](0047-accepted-times-sync-as-placements.md).

## Context

The assistant and agents propose schedule changes, and the server holds them until the owner
decides (ADR-0016). The owner approves one from the rail or from Plan, with a one-use token.

On the first run with a real model, the owner approved "Move task "Paperwork"", Monday 6:30 to
8:00 PM, and nothing changed:
- the server recorded the approval;
- no device turned it into the task's accepted time;
- the planner went on suggesting a time for the task.

ADR-0046 had kept the queue honest about this: "backend approvals do not claim to have written a
local calendar event". But the rail says the assistant proposes schedule changes and that every
change waits for approval. An approval that changes nothing breaks that promise.

Accepting the planner's own suggestion already does all an approval should:
- it writes an app-owned block in one transaction;
- it binds the block to the task's revision and to the calendar and sleep it was planned on;
- it records the decision in the history;
- it syncs the block as a placement (ADR-0047);
- it writes the block to a calendar when the owner chose one (ADR-0053, ADR-0065);
- it can be undone.

## Decision

1. **Approving a schedule proposal writes the task's accepted time on the owner's computer**, by
   the same path as accepting a planner suggestion. Only the computer approves, since the phone has
   no decisions to make. Declining is unchanged: the server records it and nothing is written.
2. **The computer checks the time before it approves.**
   - It reads the proposal through a new device-authenticated `GET /v1/proposals/{id}`. The read
     carries no decision token, and visitor requests stay on their own surface.
   - It plans the task again inside exactly the proposed time, with the scheduler's own rules: in
     predicted waking time, clear of busy time, as sure as the task asks. It uses the planner's
     current estimate, calendar and sleep.
   - The proposed time takes the place of the task's own bounds, since the owner chose it.
   - A time that has begun is refused, and so is one not the task's length. The block is always
     the task's whole duration.

   A time that no longer fits is not approved at all, and the owner reads why, for example "Not
   approved. That time lasts 30 minutes, but File paperwork takes 1 hour." The proposal stays to be
   declined or to lapse.
3. **Then the server records the approval, and the computer writes the time.** A race between
   the two can still leave the approval recorded and the time unwritten. The owner reads
   "Approved, but the time was not written here: …" and can ask for another time.
4. **A task keeps one accepted time.** An approval replaces the task's earlier accepted time in
   the same transaction (`ProposalDecisionItem.Replaces`), so "Move task" moves it. The check leaves
   the task's own block out of the busy time, so a move may overlap where the task sits now.
5. **Decisions carry a note, not an error.** A refused or partly done decision returns the queue
   as it is, with `decisionNote`. Only a queue that cannot be read is an error. Before, the one
   status did both jobs, and the first refusal locked every other decision until a refresh.
6. **One row in the history.** The approval is this computer's decision, kept with its origin in
   its id (`decision_assistant_…`), so no schema changes. The history lists it once, as the
   assistant's, with Undo; the server's record of the same proposal is not listed again. Undo
   removes the block here. The server keeps its record of the approval, which the history then
   shows as undone (`undoneHere`), not approved.
7. **The plan is read again.** An approval that wrote a time announces a calendar change, as
   accepting a suggestion does. The planner's list now also reloads on any calendar change, since
   new busy time changes what it suggests.

## Consequences

- An approval does what the rail says: the task gets its time, the placement syncs, a chosen
  calendar receives it, and the planner stops suggesting another.
- A proposal can be approved only on a computer with an estimate and the task open. Elsewhere it
  waits.
- `propose_reminder_shift` resolves to a task's time, as the server always resolved it, so
  approving one gives the task that time.
- The server gains one read route. It shows an enrolled device nothing the proposal list does not.
