# ADR 0048: Medication and context-marker sync

- Status: accepted
- Date: 2026-09-26
- Implements slice M-D of [`medication-feature-plan.md`](../medication-feature-plan.md) and the
  C3 milestone of [`completion-plan.md`](../completion-plan.md). Builds on
  [ADR-0020](0020-task-sync-revisions.md) (revision records),
  [ADR-0017](0017-server-erasure-tombstones.md) (erasure tombstones) and
  [ADR-0024](0024-local-medication-evidence.md)–[ADR-0027](0027-local-clinician-context-report.md).

## Context

Medication definitions, doses, their corrections and rhythm context markers
were device-local. The owner records a dose where they are — at the computer or
on the phone — and a clinician report is only correct if it sees every dose.
Sleep, tasks and accepted times already travel through the self-hosted log;
medication records did not, and their contracts said so.

## Decision

1. **Four new sync kinds.** `medication` carries one revision of a definition
   (contracts/v2 `medication-set#/$defs/medication`) with record id
   `<medication_id>_r<revision>`; `medication_event` carries a dose
   (`medication-event-set#/$defs/event`) and `medication_correction` a
   correction (`#/$defs/correction`), each with its own id as record id;
   `context_marker` carries a marker (v1 `rhythm-marker-set#/$defs/marker`).
   The sync batch schema refers to those definitions directly, and the
   contract validators load every version together so it can.
2. **One definition of a valid record.** The record types and their validation
   live in `core/medication` and `core/markers`, used by the desktop's store and
   by the server's push validation. Neither side can accept a record the other
   would refuse.
3. **Definitions follow tasks; evidence follows sleep.** A definition is a
   mutable intention: consumers keep the highest revision. A device whose
   unsent edit meets a revision it had not seen rebases the edit on top of it,
   field by field: a field the edit changed keeps the local value, every other
   field takes the remote one, so neither device's change is silently
   reverted. When both changed the same field, the device that syncs later
   wins; when the combination would not be a valid definition, the local edit
   wins whole. Doses and corrections are append-only; a correction never
   rewrites its dose. Two devices may each correct the same dose while apart,
   so corrections of one dose need not form a single chain: every correction
   applies, in creation order, and a new local correction follows the latest.
4. **Deleting is erasure.** Deleting a medication erases every revision, every
   dose recorded against it and every correction of those doses, and registers
   the medication id so a later revision or dose from an offline device is
   refused. Deleting a dose erases its corrections, and a correction arriving
   later for an erased dose is refused. Markers erase alone. The server indexes
   doses by medication and corrections by dose at push time, so it never has to
   decrypt records to find them.
5. **Nothing new leaves the owner's trust zone.** The records are encrypted at
   rest on the owner's own server like every other record. The server reads
   none of them for estimation, projection, the portal, the assistant or MCP;
   the sleep review snapshot is pinned to sleep kinds so a medication record
   cannot be mistaken for one. Labels, notes and clinician text stay out of
   provider and agent context as before.
6. **A record never waits in vain or vanishes.** A dose that arrives before
   its definition, or a correction before its dose, waits on the device and
   applies when that record arrives. Parents normally arrive first; after a
   server restore, devices upload again in any order.
7. **Reminders stay with the desktop.** A dose reminder is delivered by the
   desktop app that owns the schedule; the phone does not remind, so a synced
   schedule cannot raise two reminders. A reminder missed while no desktop was
   running is not delivered late.

## Consequences

- A clinician report on any device reflects doses logged on every device once
  synced, and a correction made elsewhere changes the report.
- Two devices editing the same definition while apart converge on one
  revision that keeps each device's changed fields. Rare for a single owner,
  and visible in the definition's revision history.
- Clients reject kinds they do not know, so the desktop and the companion must
  support these kinds before any device pushes them. They ship together.
- The v2 medication and v1 marker contracts are no longer device-local; they
  are private contracts that sync to the owner's own server.
