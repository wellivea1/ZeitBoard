# Usability review, and an outside review's findings — 2026-11-13

Two reviews are recorded here, with what was done about each finding:

- a walk through everyday tasks in the running app, following
  [`ease-of-use-review-2026-10-28.md`](ease-of-use-review-2026-10-28.md);
- an outside reviewer's reading of the code at `890b30d`, which the owner shared. Each of its
  claims was checked against the code before it was accepted.

The decisions that came out of both are in
[ADR-0069](decisions/0069-one-reading-of-the-records.md).

## How the app was checked

- **App:** `wails dev`, driven from a browser, on a throwaway profile (`ZEITBOARD_DATA_DIR`,
  `APPDATA` and `LOCALAPPDATA` under the ignored `.cache/`).
- **Data:** synthetic only: 21 nights drifting 30 minutes a day, one 90-minute task, and two
  events.
- **Not run:** a server, a phone, a screen reader, or the packaged app. Sharing was checked
  through the server's tests and the page's own script, not a live link.

## Everyday tasks

| Task | Path from Home | Verdict |
|---|---|---|
| Log going to sleep and waking | The two buttons beside the lead | Fast. A wake tap minutes after the sleep tap asks, with pickers, and records nothing until answered |
| Read today | The lead sentence and Fig. 1 | Clear. The sentence does not yet mention a "going to sleep" already marked |
| Add an appointment | Was not possible | Fixed: **Add an event** on the week and under Home's diary |
| Add a task and accept its time | Plan → Add task → Accept | Easy; Undo is offered at once |
| Correct a night | Log → Edit | Easy since the pickers of 2026-10-28 |
| Record a dose | Log › Medications | Works, but the first step, adding a medication, is at the foot of the page |
| Check what a link shows | Sharing → Preview | Now says why no times show |

## Findings from the walk

| # | Severity | Finding | State |
|---:|---|---|---|
| 1 | High | **One event added by the owner made the whole calendar unreadable.** The desktop answered with ownership `owner` (ADR-0068), which the window's adapter refused. Home's diary, the week and Data Sources all said the calendar "could not be read". | Fixed. The adapter reads it, and a test holds the case |
| 2 | High | **No event could be added in the window.** ADR-0068's storage and sync had landed without it. | Fixed: add, change and delete, with the same pickers as a night. The form says at once when the event falls in a predicted sleep window, and keeps it as entered. A click on an empty part of a day starts an event there |
| 3 | Medium | **A test failed about one run in four.** A night's history was ordered by time alone, so an edit and its undo made in the same clock tick could swap. | Fixed: the edit that replaces the other is the later one |
| 4 | Medium | **Home's lead sentence dropped down the page** when the Record column grew to ask a question, leaving the top of Home blank. | Fixed |
| 5 | Medium | **An event's details opened below the fold**, so a click on the board seemed to do nothing. Its time read "Nov 13, 2026, 3:00 PM EST to Nov 13, 2026, 4:00 PM EST". | Fixed: the details come into view, and read "Today 3:00 – 4:00 PM" |
| 6 | Low | The browser preview contradicted itself: Log's heading said the log "could not be read", Home counted a trusted view that Sharing did not, and Home showed a "Record" title over nothing. | Fixed |
| 7 | Low | The lead does not acknowledge a "going to sleep" already marked. | Open |
| 8 | Low | Log › Medications puts "Add a medication" last, below an empty history. | Open |
| 9 | Low | Small type on Home: 31 of about 60 text elements are 12px or smaller. The top navigation is clipped at phone width. | Open; a visual choice for the owner |
| 10 | Low | The preview's Rhythm sample is dated June under today's date. | Open |

## The earlier review's recommendations

| # | Recommendation (2026-10-28) | State |
|---:|---|---|
| 1 | Correct with pickers | Done on the computer. The phone still asks for a typed timestamp |
| 2 | Let the owner add an event | Done on the computer (finding 2). Writing events to the owner's calendar and showing them on the phone are ADR-0068's later parts |
| 3 | Say why the portal shows nothing | Done. The reader's page says "There are no current waking windows to show" and that the link is working. The owner's preview says why |
| 4 | Make Home's sharing line true | Done: "One share link is working", with a note when it shows no times |
| 5 | Adjust right after a tap | Open |
| 6 | Fix the setup guide's calendar check | Was already done |
| 7 | Hold suggestions to the freshness Home keeps | Done (ADR-0069) |

## The outside review

It read the estimator, the desktop, the phone, the planner and the portal, and reproduced several
faults. Its conclusion matches this project's own: the software is ahead of the evidence that it
helps, and the README's "every planned function is delivered" was too strong.

| Finding | Checked | Disposition |
|---|---|---|
| Cycles are counted against 24 hours, so missing nights stop or bend a 27-hour fit | True | **Fixed**, and measured on the owner's history (ADR-0069): 809 forecasts became 921, with errors unchanged |
| The planner ignores freshness and plans in uncertain hours | True | **Fixed**: the planner plans only in the outlook's awake time, and not on old records |
| The shared page calls uncertain hours "Likely awake" | True | **Fixed**: shared windows leave out what a sleep window also covers |
| An excluded record makes old evidence current | True; both new tests fail without the fix | **Fixed** on the computer and the server |
| A shared page left open keeps its claim through an outage | True: a failed refresh was never tried again, and a reconnecting stream started no polling | **Fixed**: it retries, says the connection is lost, and withdraws the claim once the claim has expired |
| A refresh empties a message part way written | True | **Fixed**: drafts, focus and cursor survive |
| "Probably a reachable time" says more than the records can | Agreed | **Fixed**: "Being awake does not mean being free to answer." |
| Overlapping reports are averaged two at a time, and can read as confirmed | True | **Fixed**: merged as a group; the owner's confirmed times decide |
| Every Health Connect session is imported as main sleep | True | **Fixed in the estimator**, not the phone: one sleep start opens each cycle. The phone still labels every session main sleep |
| An excluded dose can come back on the phone | True by reading | **Fixed**. Not run on a device |
| "Going to sleep" does not reach the people sharing a link | True | **Not built.** A reported status that syncs and expires is a new feature with a new record kind and a privacy review. It needs the owner's decision |
| A healthy 27-hour source reads as stale near the end of each waking day | True of the policy | **Not changed.** The thresholds are ADR-0031's. Making them follow the cycle length is a decision |
| A session the provider deleted stays in the estimate | True | **Not built.** Needs Health Connect's change log and a device |
| Sleep recorded in another time zone is held | True, and deliberate (ADR-0032) | **Not changed.** Keeping the instants with their own offsets is a contract change |
| Sleep in several pieces counts only its first piece towards duration | True | **Not changed.** Low risk today: the phone sends one interval for each session |
| The portal's rate limit counts everyone behind a proxy as one visitor | True | **Not changed.** Trusting a forwarded address is a security decision for the exposure review |
| The backtest's "hit rate" is easy to misread | Agreed | Clarified in `verification.md` |
| The local-agent restart test and three Settings tests fail under load | Seen here too, once | Open |

## What this does not establish

- Nothing here was run on the owner's phone, watch or server, or by anyone but the agent that
  wrote the code.
- The estimator's change was measured on one person's history from 2021 to 2023.
- The private pilot, the independent portal review and the clean-machine install are still open,
  and remain the acceptance gate.
