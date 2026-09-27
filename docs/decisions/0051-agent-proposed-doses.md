# ADR 0051: Agents propose doses; only the owner records them

- Status: accepted
- Date: 2026-09-27
- Implements the dose-logging proposal of `medication-feature-plan.md` §4 (slice M-E).
- Builds on [ADR-0050](0050-agent-action-registry.md) (the action registry),
  [ADR-0028](0028-desktop-local-agent-endpoint.md) (the local agent endpoint),
  [ADR-0027](0027-local-clinician-context-report.md) (adherence counts only explicit marks)
  and [ADR-0048](0048-medication-and-marker-sync.md) (medication sync and erasure).

## Context

An owner using a voice assistant or another agent could ask it what their medication timing
looks like, but could not tell it "I just took my evening tablet". The medication plan requires
agent-assisted logging to land as a pending confirmation. A dose is a health record, so the
owner confirms it.

Task proposals already exist, but they could not carry a dose:

- the server's scheduler resolves them, and the server has no reason to receive a dose from an
  agent;
- they name a task, not a medication;
- they are approved with a one-use token on the server, while doses are recorded on the owner's
  computer.

## Decision

1. **A second kind of proposal.** The registry gives each proposal a subject:
   - task proposals name a task, and the server's scheduler resolves them wherever they arrive;
   - `propose_log_dose` names a dose, and it waits on the owner's computer.

   The server refuses a dose proposal, even one dressed with a task target. The chat assistant
   and the server's connector never offer one: the chat model sees no medication, and the
   server resolves only tasks.
2. **What an agent can say.** A dose proposal names:
   - a medication, by the opaque id that `get_snapshot` and `get_medication_timing` list;
   - taken or skipped;
   - optionally when, at most a week ago and not in the future.

   It carries no label, note, amount or "scheduled" mark. The strict decoder refuses any other
   field, and the result names only the proposal's id.
3. **The queue.** The desktop keeps proposed doses in `local_dose_proposals`:
   - at most 20 wait at once, and each lapses after a day;
   - they are never synced or exported;
   - erasing a medication erases its proposals;
   - decided and lapsed proposals leave the history after 30 days.
4. **The owner decides.** A proposed dose appears:
   - under "Waiting on you" on Home;
   - in Plan's decision queue, with the other proposals, under the same Accept and Decline;
   - in the shared pending count.

   Accepting records, in one transaction, the medication event that a hand-logged dose would be:
   manual, user-reported, never marked scheduled (ADR-0027). It then syncs, and appears in the
   history and the clinician report like any dose. Declining records nothing.
5. **Agents see the outcome, not the record.** The snapshot's medication section counts the
   proposed doses still waiting. An agent learns about a recorded dose the way it learns about
   any dose, through the aggregate timing facts.

## Consequences

- An agent can relay "I took it" without being able to write health data.
- The desktop's zone at the time of the proposal is the recorded dose's zone. The desktop names
  that zone from the system (`core/platform/localzone`). Until 2026-09-27 it named New York on
  every Windows computer, which also affected quick logging.
- The server and its connector are unchanged in what they accept, apart from the explicit
  refusal of dose proposals.
