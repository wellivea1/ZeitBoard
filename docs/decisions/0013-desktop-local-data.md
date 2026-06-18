# ADR 0013: Desktop-local sleep data

- Status: accepted
- Date: 2026-06-18
- Builds on [ADR-0002](0002-provenance-corrections-and-estimation.md),
  [ADR-0005](0005-desktop-theming-reduced-stimulation.md), and
  [ADR-0011](0011-server-side-estimation.md).

## Context

The desktop shell previously demonstrated Overview, Rhythm, and Approvals with synthetic
sleep sessions. That was useful for validating UI and engine integration, but it was not a
trustworthy local product slice: the user could not enter sleep, corrections were not
stored locally, and empty or insufficient data could be masked by sample fallbacks.

The next Phase 2 slice needs one real desktop-local path before calendar sync or backend
sync enters the loop.

## Decision

Add a desktop-local, single-user manual sleep log backed by SQLite. The desktop app stores:

- sleep observations as immutable records matching `observation-set.schema.json` fields
  for `sleep_episode`;
- sleep corrections as append-only records matching `correction-set.schema.json` fields;
- correction supersession when an edit replaces a prior correction for the same entry.

The desktop read path converts those contract-shaped records into core domain sessions,
applies `domain.ApplySleepCorrections`, then resolves overlapping reports with
`ingest.ResolveOverlappingSleepReports`. Overview, Rhythm, and Proposals use the effective
stored sessions and the existing core estimator/scheduler. They return typed empty/refusal
states when the local store has no data or fewer than the estimator minimum usable
principal sleep episodes.

## Scope Boundary

This slice is desktop-local only:

- no backend sync;
- no calendar import or write-back;
- no hidden background collection;
- no DLMO, exact circadian phase claim, diagnosis, dosing, or treatment timing advice.

The frontend may still use sample data when Wails bindings are unavailable in a browser
preview, but the desktop service itself does not substitute synthetic sleep sessions for a
local store.

## Consequences

Users can now manually enter sleep/wake civil times, edit them by appending corrections,
suppress a source entry from estimates, and inspect correction history. The raw observation
remains immutable. Rhythm charts and proposal availability now come from local user-entered
sleep data when present, and remain honest refusal/empty states otherwise.

Phase 2 follow-up should harden import/export/deletion controls, add onboarding, and run
the accuracy harness on participant-controlled real histories. Phase 3 can later sync
these same contract-shaped records to the self-hosted backend after explicit consent.
