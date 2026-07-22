# ADR-0024: Local medication definitions and append-only evidence

- Status: accepted
- Date: 2026-07-22

## Context

The Medications route is the final full sample-only desktop screen. ZeitBoard
already has a core helper for attaching wake-relative and predicted-sleep
context, but it has no contract, persistence, correction history, export, or
real logging workflow. Medication text is especially sensitive and timing
context can easily be mistaken for dosing guidance.

The [AASM intrinsic circadian-rhythm guideline][aasm-guideline] supports only
limited, population-specific treatment recommendations and rates the evidence
behind strategically timed melatonin for blind adults with N24SWD as weak.
Medication adherence app trials, including
[MedISAFE-BP](https://pmc.ncbi.nlm.nih.gov/articles/PMC6145760/), also do not
justify claiming improved clinical outcomes.
ZeitBoard therefore records user-entered facts and may later preserve explicitly
clinician-authored rules; it does not infer or recommend a medication, dose,
schedule, or interaction.

## Decision

1. A medication definition is mutable local intent with a monotonically
   increasing revision. Its label, form, strength label, and future clinician
   rule are private text.
2. A medication event is immutable evidence: a user records `taken` or
   `skipped` at a civil timestamp. Changes append a correction linked to the
   original event; ordinary edits never update the evidence row in place.
3. Definitions, raw events, and corrections export through strict v1 local
   contracts. Empty arrays remain arrays. Derived timing never appears in the
   export.
4. Wake-relative and before-sleep values are recomputed for each read from the
   current effective sleep evidence and estimate. The UI labels observed and
   predicted relationships separately and shows an honest unavailable state.
5. Hard deletion is distinct from correction or exclusion. Event erasure
   removes the selected event and its correction chain while retaining the
   medication definition. Medication erasure removes the selected definition
   and all dependent events/corrections. Both paths checkpoint the WAL and
   vacuum the local database under `secure_delete`.
6. Medication labels and notes remain inside the desktop trust zone. They are
   excluded from trusted views, server projections, assistant/LLM context, MCP
   output, telemetry, and logs. Later agent work may expose only opaque IDs and
   timing/status projections.
7. M-A ships no reminder scheduling, collision forecast, interaction checking,
   or treatment recommendation. Those remain separately gated work; the UI
   states the boundary directly.

## Consequences

- The Medications screen can retire fixture data without overstating the M-B
  schedule/reminder feature.
- Corrections retain provenance while privacy erasure remains real deletion.
- Sleep corrections can change derived rhythm context without rewriting the
  medication record.
- Any future sync requires revision records for definitions, append-only event
  and correction records, and erasure tombstones before medication text may
  leave the device.

[aasm-guideline]: https://pmc.ncbi.nlm.nih.gov/articles/PMC4582061/
