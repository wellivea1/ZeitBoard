# ZeitBoard product quality review — 2026-09-08

## Assessment

ZeitBoard has a substantial working desktop and self-hosted system, rather than
just a UI prototype. Its strongest features are the real observation/correction
model, explicit uncertainty, local scheduling with approval, useful rhythm
visuals, user-owned storage, and the separation of private records from sharing
and assistant projections. The five-destination navigation and ruled visual
language should be retained.

It is not yet proven as an unattended, automatically rhythm-aware daily planner.
The outstanding work is operational evidence and completion of existing flows,
not another platform or visual rewrite. This review repairs concrete failures
in the current product while preserving the measurement gates for inference.

Scope: local checkout at `bde24a7`, application screens and adapters, Go task and
time handling, relevant core/server/Android implementations, project specs,
ADRs and roadmap, local Claude history and the original Codex development task.
Browser inspection uses synthetic records only. This is a product/code review,
not an independent security audit or evidence of medical effectiveness.

## Direction recovered from conversation history

The primary Claude development conversation runs from June 15 to August 10,
2026. The original Codex development task adds implementation instructions and
corrections. Duplicate imported histories were treated as the same evidence;
neighboring Zeitlog/Zeitdex and NoobBoard work was not treated as ZeitBoard
scope. Raw transcripts, credentials, and personal health records are not copied
into this repository.

- **Identity:** ZeitBoard is intentional; it is separate from Zeitlog/Zeitdex
  despite the shared prefix (Codex, June 15–16).
- **Purpose:** local assistant, calendar/task planning, disease-management
  records, and a self-hostable backend with a passcode-protected, live
  availability portal and owner-reviewed visitor requests (Claude, July 19–23).
- **Connectivity:** both desktop and companion must be able to connect; sync is
  to the owner's server, assistant providers are BYOK. An imposed no-network
  direction was explicitly rejected (Claude, June 17).
- **Experience:** visual-first for sighted Non-24; reasonable keyboard and
  screen-reader access, plus an agent/voice path. Avoid oversized bubbles,
  undifferentiated tiles, excess whitespace and distracting monochrome accents.
  Themes must work with reduced stimulation and amber lenses (June 17, July 14,
  Codex July 20).
- **Automaticity:** reduce transcription and make fresh data arrive reliably.
  Unknown is acceptable; uncertainty must remain obvious. Operational utility
  takes priority over further portal breadth (Claude, August 5–6).
- **Specific unresolved complaints:** Home spacing/separation, intermittent
  blank Rhythm views, and assistant-button overlap (Claude, August 8).
- **Completion standard:** test actual workflows and close the requested loop;
  passing checks or a nearby improvement is not proof that the requested
  feature is complete (Codex, June 19 and July 26).

## Surface and completeness review

| Surface | Working today | Assessment / remaining limits |
|---|---|---|
| Home | Real state, one-tap sleep logging, evidence freshness, cycle strip, 72-hour operational outlook | Useful hierarchy; repaired service failure and initial sample leakage, time-based refresh, compact recovery and import handoff. Model confidence is disclosed as uncalibrated. |
| Plan / Calendar | Read-only ICS/CalDAV import, busy-event constraints, app-owned approved blocks, undo and ICS export | Strong ownership boundary. External-provider write-back is not delivered. Calendar source management remains denser than ordinary planning. |
| Plan / Tasks | Persisted task CRUD, revision-based sync, deterministic proposals | UI formerly lacked editing despite the backend supporting it. Now leads with task entry, optional timing constraints, editing, visible feedback and a separate deletion decision. |
| Plan / Approvals | Local decisions/history/undo, backend agent proposals, visitor request review | Read failures now clear stale local candidates and offer retry. Local and backend origins still use separate sections; combined counts, batch review and richer expiry handling remain follow-up design work. |
| Rhythm | Actogram, drift, chronological hover probe, source/correction evidence | Retain visual-first charts. Missing freshness could crash Home; service failures could substitute sample charts. Both boundaries are repaired, with route-level recovery for other render failures. |
| Log / Sleep | Quick log, manual entries, correction history, suppression and confirmed erasure | Working. DST-gap input now rejects instead of silently shifting the time. Real sleep-source coverage and daily input burden still need measurement. |
| Log / Medications and markers | Local definitions/events/schedules/reminders, contextual markers, clinical report/export | Substantial local implementation; no inferred treatment timing. Medication sync and broader agent coverage are incomplete. This pass does not change medication behavior. |
| Sharing / portal | Real passcode/expiry link creation, listing, revocation/erasure; isolated portal and requests | Earlier claims that it is only an unbuilt feature were stale. Actual external deployment, security review and a trusted-recipient pilot are separate from the synthetic web prototype. |
| Data Sources / Settings | Local import preview/commit, provenance, own-server enrollment, appearance, reaching hours, export/erase, storage protection | Recovery links now lead here appropriately. Source permission and first-run setup still need fatigue-state user testing; avoid a forced onboarding wizard. |
| Assistant / MCP | BYOK assistant and allowlisted propose-only interfaces, visible approval boundary | Existing capability should be maintained. Every new feature needs agent-readable state; full per-feature coverage and voice-client usability are not established by this review. |
| Android | Health Connect ingestion, durable local records, enrollment/outbox/sync, source revision and offset checks | No longer a non-syncing skeleton. Background collection reliability, travel holds, battery cost and real-device coverage remain operational concerns. |
| Installation / update | Reproducible toolchain and installer/update scripts, desktop/server/Android build paths | Build capability is different from an installed-user upgrade pilot. No personal installation or running data store was replaced during this review. |

## Findings and implemented changes

| Priority | Finding and consequence | Resolution |
|---|---|---|
| P1 | `backend.ts`, `rhythm.ts`, `outlook.ts` substituted synthetic forecasts for failed/invalid desktop reads. A partly initialized bridge could also create sample proposals. | Explicit unavailable projections for desktop failures; fixture fallback only without a desktop bridge. Missing proposal methods report failure. |
| P1 | Home/Rhythm initialized to fixtures before personal reads completed. A nested Overview DTO without freshness passed validation and could crash rendering. | Non-synthetic initial desktop state; normalize freshness on both DTO paths and withhold unsupported current-state claims. |
| P1 | Render/lazy-load errors could leave a blank view and remove the user's recovery path. | Route-scoped error boundary retains navigation, offers retry/reload/Home, and does not print private exception details. |
| P1 | Home, Rhythm and local proposals refreshed only on data changes; a displayed time-based claim could outlive its evidence. | Visible views refresh every minute and immediately on window focus/visibility return. Home/Rhythm refreshes coalesce; listeners and timers are disposed. Local proposal read failure clears stale actionable candidates. |
| P1 | Civil-time parser accepted a nonexistent spring-forward time and silently shifted it. | Reuse the core civil-time resolver, rejecting gaps and consistently resolving repeated times to the earlier occurrence. Unchanged task constraints round-trip as exact offset-bearing instants. |
| P2 | Task editing had no UI; deleting/recreating loses intent and is unnecessary work. | Complete editor for name, duration, earliest start, deadline and time after wake. Preserve hidden minimum-confidence policy and revision checks. Legacy label-only task DTOs cannot be edited destructively. |
| P2 | Task creation followed a large proposal surface; saves could announce success for an unavailable result. | Task entry/list precede proposals; optional constraints are disclosed progressively; failed saves retain input, successful saves provide the next-step cue. |
| P2 | Task deletion was immediate, and compact CSS hid the action column. | Separate named delete/keep decision, a done alternative, and visible actions at narrow widths. |
| P2 | Skip-to-content changed the hash to an unrecognized route; tab arrows changed selection without focus. | Skip focuses the active main region without changing the route; arrow/Home/End navigation moves tab focus. |
| P2 | Home recovery reserved 300px of blank space; anchor buttons could collapse below their intended hit area. | Content-sized recovery, consistently sized link buttons, separated task editor/list, wrapping actions and aligned refresh controls. |
| P2 | README/Android docs still described Android as unable to sync and the available product too narrowly. | Current capability statements corrected and linked to this review and ADRs. |

The task edit DTO additions and preview/error boundaries are described in
[ADR-0036](decisions/0036-desktop-recovery-and-task-editing.md). These changes do
not introduce new sharing fields, health recommendations, telemetry, or a new
network service.

## Follow-up priorities and acceptance evidence

1. **Measure the automatic loop on the owner's devices.** Use the existing
   30–60 day pilot framework: passive principal-sleep coverage, manual minutes
   per day, sync delay, travel holds, collector uptime, stale-claim incidents,
   and resource/battery cost. A synthetic run cannot establish these outcomes.
2. **Promote activity-derived episodes only after measured validation.** Shadow
   inference remains shadow-only; do not relax the real-history accuracy gate
   to make the UI appear more complete.
3. **Complete queue presentation as one coherent product flow.** Specify total
   pending counts across local/backend/visitor origins, expiry, and batch review
   without losing per-proposal evidence or approval semantics. Keep backend
   revalidation authoritative.
4. **Validate first-run and fatigue-state use with the owner.** Observe import,
   server enrollment, one-tap correction, creating/editing a task, approving it,
   exporting a report and managing a trusted link. Record completion time and
   errors; do not invent success rates from automated tests.
5. **Complete remaining interoperability deliberately.** Medication sync, full
   agent feature coverage and external calendar write-back need their own
   contracts and product decisions. Keep the existing system usable while
   those are developed; no automatic external writes are added by this pass.

These are follow-up milestones, not hidden prerequisites for the implemented
UX repairs. No new portal breadth or estimator algorithm was added.

## Verification

Baseline web formatting/lint/typecheck/tests/build and Go core/desktop/server
tests passed. The new regression suite exercises unavailable and malformed
projections, initial fixture isolation, retry, focus/navigation, time-based
refresh, task editing and failed saves, deletion decisions, exact timestamp
round-trips, DST gaps, and stale task revisions.

Final verification results are recorded in `verification.md`. Browser review
uses the normal sample preview plus an ignored synthetic bridge harness, with
desktop and narrow viewport checks. Android unit/lint checks and all 29
versioned contract fixtures were also checked. Neither browser mocks nor unit
tests establish real-device passive coverage, production portal security, or
long-term forecast benefit.
