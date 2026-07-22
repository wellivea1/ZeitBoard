# ADR-0027: Local clinician context report

- Status: accepted
- Date: 2026-07-22

## Context

Medication slices M-A and M-B record local taken/skipped evidence and explicit owner-authored
schedules. ADR-0026 adds self-reported travel, illness, disruption, and forced-schedule context
without making those records estimator inputs. M-C must combine those existing facts with effective
sleep history in a clinician-readable artifact without turning temporal alignment into treatment
advice, causality, or an inferred missed dose.

The [AASM actigraphy guideline] supports actigraphy only conditionally as an assessment aid for
circadian rhythm sleep-wake disorders; it does not make an actogram diagnostic or causal. The [AASM
intrinsic circadian-rhythm guideline] also uses population- and treatment-specific recommendations
with qualified evidence, which cannot justify individualized timing advice from this app. [STROBE
explanation] shows why simultaneous changes and other confounders prevent a simple before/after
association from establishing cause. Medication-reminder trials have also produced mixed or null
outcome results ([MedISAFE-BP trial], [REMIND trial]), so the product must not imply that logging or
reminders improve health outcomes.

## Decision

1. The desktop builds the report locally from effective sleep observations, effective medication
   events, medication definitions, and rhythm-context markers already in SQLite. It introduces no
   estimator, inference source, upload, sync record, trusted-view field, agent context, or telemetry
   path.
2. The longitudinal chart is one row per selected civil date on a 24-hour axis. The default row
   anchor is 18:00 and noon is optional. Row boundaries use the domain civil-time resolver, so DST
   transition rows may span 23 or 25 real hours while retaining their civil clock positions.
   Recorded gaps remain explicit no-data rows. Forecast is off by default and appears only after an
   explicit opt-in.
3. A medication definition may carry an optional owner-recorded start instant and explicit IANA
   zone. The start marker is not a dose, schedule, treatment instruction, or claim that the
   medication changed the rhythm.
4. Adherence denominators contain only events explicitly marked scheduled. Explicit `taken` and
   `skipped` records are counted separately; as-needed records remain separate; excluded effective
   events are counted but omitted. Absence of a log is never converted into a missed dose.
5. A before/after association reuses the estimator's principal-episode selection, cycle indexing,
   Theil-Sen slope, and ordinal-confidence logic, constrained to sleep and context inside the
   selected report range. Each side requires at least five usable episodes and uses at most
   fourteen. Sparse, ambiguous, or unsupported data produces a typed unavailable result.
   Overlapping ADR-0026 markers are listed as possible confounding, never as an explanation. No
   effect size, recommendation, or causal conclusion exists.
6. Redaction happens in Go before either the React preview or HTML renderer receives a report DTO.
   Diagnosis and calendar/location information are always omitted, as is verbatim clinician-entered
   medication guidance. Medication labels/forms/strengths use neutral aliases by default; medication
   notes and rhythm-context notes are separately opt-in. The returned redaction list states exactly
   what was removed.
7. Export requires the exact typed confirmation `EXPORT` and regenerates from the submitted settings
   rather than trusting cached browser state. The result is a standalone, auto-escaped HTML document
   with an offline content-security policy, no scripts or external assets, month print breaks, chart
   legends, accessible tables, provenance, and the interpretation boundary. A browser's print
   command is the supported PDF path. No network request is made.
8. `clinical-chart-request` v1 names the stable range, orientation, day anchor, zone, layers, and
   mandatory redactions. Its 48-hour orientation is reserved for the already-specified chart
   refinement; this M-C desktop workflow emits the 24-hour clinical orientation only.

## Consequences

- M-C is complete without creating a new health-data store or external egress path. M-D sync and M-E
  agent work remain separately gated.
- The report is useful with sparse data because calendar gaps remain visible, while drift and
  association components can independently refuse.
- Private labels and notes can enter an owner-created file only after explicit opt-in and
  confirmation. That file is outside ZeitBoard's control once saved.
- Direct PDF generation, PNG export, the 48-hour clinical view, a blank clinical log template, and a
  generic light-therapy event type remain refinements. A private context note is not parsed to
  manufacture any of those records.

[AASM actigraphy guideline]: https://aasm.org/actigraphy-guideline-release/
[AASM intrinsic circadian-rhythm guideline]:
  https://aasm.org/resources/clinicalguidelines/crswd-intrinsic.pdf
[STROBE explanation]: https://pmc.ncbi.nlm.nih.gov/articles/PMC2020496/
[MedISAFE-BP trial]: https://jamanetwork.com/journals/jamainternalmedicine/fullarticle/2678454
[REMIND trial]: https://jamanetwork.com/journals/jamainternalmedicine/fullarticle/2605527
