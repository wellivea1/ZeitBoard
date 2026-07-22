# ADR-0025: User-authored medication schedules and local reminders

- Status: accepted
- Date: 2026-07-22

## Context

ADR-0024 established medication definitions as mutable local intent and
medication events as append-only evidence. The next slice must represent
explicit civil-time schedules, show how those times relate to the current
sleep forecast, and optionally deliver desktop reminders. Medication timing is
sensitive: an inferred, shifted, or recommended time could be mistaken for
clinical guidance, while duplicate reminders could plausibly cause harm.

The [AASM intrinsic circadian-rhythm guideline][aasm-guideline] does not support
app-generated individual medication timing. Reminder evidence is also limited:
the [MedISAFE-BP trial][medisafe-bp] found a small improvement in self-reported
adherence without a blood-pressure difference, while the 53,480-person
[REMIND trial][remind] found no adherence improvement from its low-cost
reminder devices. The feature can preserve user intent and present local
prompts, but it cannot claim an adherence or clinical outcome.

## Decision

1. A schedule is optional and entirely user-authored. Its only kinds are
   `as_needed`, `fixed_clock`, and `cycling`. Clock schedules require an
   explicit IANA zone and one to eight unique `HH:MM` values. Cycling also
   requires a real civil start date and 1-365 on/off days. The app never
   derives a schedule from a medication label, event history, sleep estimate,
   or clinician-rule text.
2. Expansion follows civil dates in the schedule zone. A nonexistent
   daylight-saving time is skipped and reported. A repeated time resolves to
   its first occurrence. Cycle position advances by civil date rather than
   elapsed UTC hours.
3. The desktop displays 14 civil days of occurrences, but classifies only
   instants covered by the current estimator horizon. Each occurrence says
   either that it is inside predicted sleep, outside predicted sleep, or
   outside the current forecast horizon. This is feasibility information, not
   dosing guidance. ZeitBoard never moves, suppresses, or recommends a time.
4. The optional clinician rule is private verbatim text entered by the owner.
   The UI labels it as clinician guidance and never parses or paraphrases it.
5. Desktop reminders are off by default and can be enabled only on an explicit
   clock schedule. Opting in asks ZeitBoard to show the reminder at those civil
   times even when the forecast predicts sleep; the collision surface makes
   that overlap visible. The notification copy is exactly
   "Reminder you set: {label}." after control-character normalization.
6. Reminder delivery polls locally and accepts occurrences no more than two
   minutes late. Before showing a notification, it inserts a durable claim
   keyed by an opaque digest of medication ID and scheduled UTC instant. The
   claim is immutable and unique. A delivery failure is not retried
   automatically: the system prefers a possible missed notification to a
   duplicate medication prompt.
7. Archiving a medication pauses its reminders without deleting its schedule.
   Hard medication erasure cascades through reminder claims. Claims contain no
   label or note, are excluded from export and sync, and do not leave the local
   SQLite trust boundary.
8. M-A's strict v1 medication contracts remain unchanged. Schedule zones,
   reminder opt-in, clinician-rule text, and the schedule-capable export use
   strict medication v2 contracts, which unsupported consumers must reject
   rather than guess.
9. The current unpackaged Windows desktop uses the existing tray
   `Shell_NotifyIcon` transport. A packaged-app migration may adopt the
   supported Windows App SDK notification API without changing the scheduling,
   claim, privacy, or wording rules.

## Consequences

- Owners can encode a schedule they already follow without ZeitBoard becoming
  a medication recommender.
- DST gaps and estimator-horizon limits remain visible instead of being
  silently guessed.
- Durable claim-first delivery provides at-most-once behavior across polling
  and restarts, with an explicit missed-versus-duplicate tradeoff.
- Notification text is visible to the local operating-system notification
  surface after explicit opt-in; the editor must disclose that boundary.
- Adherence summaries, actogram markers, clinician reporting, sync, agent
  projections, and missed-dose sharing remain separate gated work.

[aasm-guideline]: https://pmc.ncbi.nlm.nih.gov/articles/PMC4582061/
[medisafe-bp]: https://pmc.ncbi.nlm.nih.gov/articles/PMC6145760/
[remind]: https://pmc.ncbi.nlm.nih.gov/articles/PMC5470369/
