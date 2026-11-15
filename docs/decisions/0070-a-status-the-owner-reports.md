# ADR 0070: A status the owner reports

- Status: accepted
- Date: 2026-11-15
- Builds on [ADR-0017](0017-server-erasure-tombstones.md),
  [ADR-0029](0029-availability-portal-foundation.md),
  [ADR-0058](0058-a-link-can-show-skipped-doses.md) and
  [ADR-0069](0069-one-reading-of-the-records.md).

## Context

A share link shows a forecast. The forecast cannot know that the owner has just gone to bed three
hours early, or is awake and does not want a call. Both reviews of 2026-11-13 asked for a way to
say so, and ADR-0069 left it to be decided.

What the owner says about now is not a sleep record. It is also more private than a forecast: "I
am trying to sleep" is a statement about a person at a moment, where a waking window is an
estimate about a pattern. So the feature had to answer three questions before it could be built:
what a report is, who may see it, and who decides.

## Decision

1. **A report is its own kind of record.** `status` joins the sync contract, with five fields:
   `status_id`, `status`, `reported_at`, `expires_at` and `shared`. The statuses are `awake`,
   `trying_to_sleep`, `do_not_disturb` and `cleared`. `core/presence` holds the type and its rules.
2. **A report always ends.** Every report but `cleared` carries the time it lapses, at most 24
   hours after it was made. Left to the defaults that is 4 hours for awake, 10 for trying to
   sleep and 2 for do not disturb. A forgotten status cannot stand for a week.
3. **The newest report stands.** `presence.Current` reads the reports and returns the newest that
   has neither lapsed nor been cleared. A device that makes a report dates it after every report
   it knows of, so two devices with different clocks cannot bury a new report under an old one.
4. **A report is never evidence of sleep.** The estimator does not read it, the freshness of the
   records does not move with it, and a page's "Updated" time stays the age of its forecast.
   Saying "trying to sleep" does not make old sleep records new.
5. **Privacy is set at three points, and each defaults to private.**
   - *For each link.* A link's `reportedStatus` grant is empty, `contact` or `full`. Empty shows
     nothing. `contact` shows only "Not a good time right now" while the owner is trying to sleep
     or has asked not to be disturbed, with no status and no time. `full` shows the status and how
     long ago it was said. New links start empty.
   - *For each report.* A report carries `shared`. A report that is not shared never reaches a
     page, whatever the link allows.
   - *For each device.* The computer keeps whether new reports are shared, and whether its
     sleep and wake taps also report a status, in `status.json`; both are off until turned on.
     The phone keeps the same share choice in its settings, off until turned on.
6. **What a link shows can be changed after it is made.** `POST /v1/portal/profiles/{id}/grants`
   replaces a link's waking-window, skipped-dose and reported-status grants and republishes. The
   passcode, expiry and the request and message grants stay as they were made. Before this a link
   had to be revoked and made again to change what it showed.
7. **The page says whose word it is.** A shown status sits under "Reported", above the forecast
   under "Forecast", and reads "They said so 12 minutes ago. This is what they reported, not a
   forecast." A report of being awake adds that being awake does not mean being free to answer.
8. **Reports are not kept.** Each device and the server erase a report seven days after it was
   made, through the same tombstones as any other erasure. Erasing all sleep data erases them.

## Consequences

- The owner can correct a page for the next few hours without touching a sleep record.
- A visitor on a `contact` link learns less than the forecast already tells them about the
  owner's state, and learns it only while the owner has said so.
- The projection that crosses to the portal gains one optional object, `reported`, built by
  `reportedFor` from the link's level. The private record never crosses.
- A phone must accept the `status` kind to keep syncing. It also now accepts `event`
  ([ADR-0068](0068-events-the-owner-adds.md)): refusing either stopped its downloads at the
  first one.

## Found on the phone the same day

The feature was run end to end on the Android emulator against a local server, with synthetic
data. That run found faults that were not about status, and they are fixed with it.

9. **One queued report stopped every upload.** Reading the phone's upload queue looked for
   `provenance` in every record. A source report (ADR-0054) has none, so a phone that had
   checked Health Connect once could upload nothing afterwards. Only sleep records and their
   corrections are read that way now; other kinds keep the time they were queued with.
10. **The companion is told how long the waking stretch under way is likely to last.** The
    forecasts pair each sleep with the waking window after it, so the hours before the next
    sleep had no window. The phone drew that sleep starting sharply at its earliest time and
    said "This sleep is likely to end…" of a sleep that had not begun. `/v2/companion` gains an
    optional `waking_until`, from the same `outlook.CurrentWakingEnd` the computer draws with.
    The phone does not work it out for itself. A cached forecast without it reads "Sleep is
    likely to begin after…".
11. **A report shows the moment it is made.** The phone's screen clock ticks every half minute,
    and a report made since the last tick was dated after it and waited for the next.

## Not built here

- **The assistant cannot yet read or propose a status.** `AGENTS.md` asks every feature for an
  agent-operable interface. The assistant's snapshot and its allowlisted actions do not include
  the reported status, so a person using ZeitBoard by voice cannot set one. It needs a snapshot
  field, a propose-only action through the approval gate and their contract tests.
- **The phone cannot choose how long a status lasts or share one report differently from the
  rest.** It uses the default lifetimes and its one share setting. The computer has both.
- **Settings do not sync.** The share default is kept separately on each device. See
  `docs/settings-architecture-review-2026-11-15.md`.
