# ADR 0018: User-owned flexible tasks

- Status: accepted
- Date: 2026-07-02
- Builds on [ADR-0013](0013-desktop-local-data.md) (local contract-shaped
  records) and the v1 `schedule-proposals` / `schedule-request` contracts.

## Context

Every planning surface — the proposal queue, the calendar phase, the assistant,
the agent tools — assumes user-owned flexible tasks exist to place and move.
Until now they didn't: the desktop task list was hardcoded display rows, and
the scheduler planned a hardcoded fixture list (`localPlannerTasks`). Real
proposals were being generated for tasks the user never created.

## Decision

1. **Tasks are a first-class local entity** with a v1 contract
   (`task-set.schema.json`): id, title, duration, status (open/done),
   created-at, and optional scheduling constraints (earliest start, latest
   finish, preferred-minutes-after-wake, minimum confidence).
2. **Tasks are mutable in place — not append-only.** Unlike sleep records,
   tasks are planning *intentions*, not health *evidence*: editing a task is
   changing your mind, not rewriting observations. No correction chain; plain
   CRUD (add/list/update/status/delete) in `local_tasks`.
3. **The scheduler plans only stored open tasks.** `GetProposals` reads the
   user's open tasks; done tasks are never planned; with no tasks there are no
   fabricated proposals or unplaced rows; without an estimate, real tasks are
   honestly marked `estimate_unavailable`. Every mapped task sets
   `RequiresApproval` — proposals only, always.
4. **Titles are private user text.** They stay on the device (and the user's
   own instance once synced), never enter trusted views, and the assistant/
   agent redaction already sends task ids and bounds only — never titles.
5. **The Tasks screen is real**: add form (title, duration, optional deadline
   and after-wake preference), open/done toggle, delete, honest empty state.
   In a browser preview (no desktop service) the surface is read-only and says
   so — no dead form.

## Consequences

- Proposals now target things the user actually wants to do; approving one is
  meaningful. The assistant/agent propose tools can reference real task ids.
- **Task sync is deferred, deliberately** — the same staging as sleep data
  (ADR-0013 local first → ADR-0015 sync later). Records are contract-shaped,
  so syncing is a transport addition: new sync record kinds, server
  validation, and tombstone interplay get their own slice. Until then tasks
  are per-device.
- Applying an approved proposal (writing the placement anywhere) remains
  future write-back work (Phase 3c); approval still only records a decision.
- The sample task board is gone from the UI; the calendar phase and assistant
  UI inherit a real task inventory to build on.
