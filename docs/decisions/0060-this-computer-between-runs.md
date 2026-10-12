# ADR 0060: This computer between runs, from Windows' own log

- Status: accepted
- Date: 2026-10-12
- Delivers roadmap slice 11's open part ("collects only while ZeitBoard is running"). Builds on
  [ADR-0042](0042-desktop-consent-and-background-sync.md)'s consent and
  [ADR-0059](0059-account-activity-in-the-shadow.md)'s shadow figure.

## Context

The activity collector runs inside ZeitBoard. It sees nothing after Quit, before ZeitBoard starts at
sign-in, or while the computer sleeps or is off. A sleep inside a run is inferred from a missed poll
and recorded with unknown confidence. Nothing shows what the collected activity implies, either: it
is counted, exported and erased, and read by nothing.

Windows itself writes to its System log when the computer goes to sleep, wakes, shuts down and
starts, and it keeps months of those entries. A program run by the signed-in person may read them
without administrator rights.

## Decision

1. **A choice of its own.** With desktop activity on, Settings offers to read the computer's sleep
   and wake times from Windows. It is off by default and says what it reads before it is chosen.
   Turning it off stops reading. What was read stays with the other activity records until they
   are erased.
2. **Six entries, by time alone.** ZeitBoard asks the System log for six entries by provider and
   number, and nothing else:
   - Kernel-Power 42 and 506: going to sleep, or into Modern Standby.
   - Kernel-Power 107 and 507: waking, or leaving it.
   - Kernel-General 13 and 12: shutting down and starting.

   Of each entry it keeps only the time and the log's record number. It reads as far back as
   Windows keeps them, then again each time ZeitBoard starts and every half hour while it runs.
3. **Activity records like the others.** The entries become observed `suspended`, `resumed`,
   `shutdown` and `startup` transitions. These states are already in the closed set, so they need
   no new field. They carry their own algorithm tag and are kept once each. They are counted,
   exported and erased with the collector's records, and they stay on this computer.
4. **A computer that is off is not in use.** A shutdown read from the log starts a quiet stretch
   that ends when the computer starts, as a sleep does. The collector's own shutdown, ZeitBoard
   quitting, still ends what is known. Where the collector inferred a sleep that the log observed,
   the two stretches are one.
5. **What it suggests, in the shadow.** Quiet stretches of three to fourteen hours become suggested
   nights by ADR-0031's rules. Rhythm › Sources shows them beside the recorded nights, with the
   agreement ADR-0059 measures, and a choice between Google activity and this computer when both
   exist. Nothing else reads them.

## Consequences

- Settings' description of desktop activity changes: Quit still stops the collector, but with this
  choice on, the times Windows noted in between are read when ZeitBoard runs again.
- The quiet stretches use the collector's idle and lock records too, so a computer that never
  sleeps still suggests nights while ZeitBoard runs.
- Not done: lock and unlock history from Windows' logs, whose event data is undocumented, and
  other operating systems' logs.
