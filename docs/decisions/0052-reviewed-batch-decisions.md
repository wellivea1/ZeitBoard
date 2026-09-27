# ADR 0052: Suggested times are reviewed and decided together

- Status: accepted
- Date: 2026-09-27
- Implements C2's "batch review preserves each proposal's evidence and authority".
- Builds on [ADR-0046](0046-unified-review-queue-and-exact-request-times.md) (the review
  queue) and the planner's reservations (completion plan, C2, 2026-09-24).

## Context

The planner suggests a time for each open task and leaves room for the others: each suggestion
reserves its time for the next. The suggestions were planned together, but they could only be
decided one at a time.

That worked only because the queue refreshes after every decision. Each suggestion's identity
and evidence include the calendar it was planned against. Accepting one puts a block in that
calendar, so every other suggestion from the same plan goes stale and is re-planned. A list
reviewed in one sitting could not be accepted in one sitting, and "accept these" was never
possible.

The feature specification's batch is "approve all low-risk", meaning high confidence and
avoiding fixed events. That would trust confidence buckets that ADR-0022 found inverted on real
history, so it is not built.

## Decision

1. **One step for a reviewed set.** "Review N suggestions together" in Plan's decision queue lists
   every planner suggestion with its title, time and reasons. The owner can leave any out, then
   accepts or declines the rest.
2. **One transaction, each decision on its own evidence.** `DecideLocalProposals` builds each
   decision exactly as a single decision would:
   - its own window, reasons and confidence;
   - the task revision, calendar and sleep hashes it was planned on;
   - for an approval, its app-owned block.

   The store checks every decision against its evidence before recording any, then records all
   of them. If anything changed since the review, none is recorded, and the owner is told to
   review again.
3. **Only planner suggestions.** Other decisions keep their own paths:
   - a visitor's request needs its own block chosen and its disclosure read;
   - a task conflict needs a version chosen;
   - an assistant proposal is decided with its own one-use token;
   - a dose or a new task is the owner's record.
4. **Undo stays per decision.** The toast offers to undo the whole batch, which undoes each
   decision on its own record. The decision history keeps each one's Undo.

## Consequences

- Several tasks added in one sitting can be planned in one decision, and nothing is decided
  against a plan that has since changed.
- A suggestion left out of the batch is re-planned against the new calendar. It usually keeps
  its time, since the accepted ones had reserved around it.
- The single-decision path is now the batch of one. There is one construction of a decision and
  one transaction that records it.
