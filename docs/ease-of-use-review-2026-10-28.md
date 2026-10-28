# Ease-of-use review: three everyday tasks

Date: 2026-10-28

The owner asked how easy three everyday tasks are:

- checking the shared portal;
- adding a new event;
- correcting sleep and wake times already recorded.

## How it was checked

Each task was done in the running app (`wails dev`, driven from a browser), starting from Home.
- **Server:** a throwaway server on 127.0.0.1, with the portal turned on.
- **Data:** synthetic only:
  - 21 nights with a rhythm drifting 30 minutes a day, the newest ending 2:00 PM on Oct 25, so
    about 64 hours old;
  - one 90-minute task, "Paperwork";
  - one share link, "Sam (synthetic)", which may see when the owner is likely awake.
- **Phone:** the companion was read from its code, not run.

## At a glance

| Task | Desktop path from Home | Effort | Verdict |
|---|---|---|---|
| Check the shared portal | Sharing → Preview | 2 clicks | Fast, but it can mislead: it blames the link when the records are old |
| Add a new event | none in ZeitBoard | Not possible in the app | Events come only from a connected calendar |
| Correct a sleep or wake time | Log → Edit → retype → Save correction | 3 clicks, and a 25-character timestamp typed exactly | Easy to find, hard to edit |

## 1. Checking the shared portal

**Path.** Sharing is in the top bar. It opens on a line such as "One link is working right now",
with the policy, the number of working links and the transport. Each link has **Preview**, which
shows inline what that person sees after the passcode, and records no visit. The address itself
is shown only once, at creation, because the server keeps only a scrambled copy. Preview is
therefore the owner's only way to check later, and it is two clicks away.

**What works.** The headline and counts are clear, and Preview is quick and honest about not
counting as a visit.

**Findings.**

| # | Severity | Finding |
|---:|---|---|
| 1.1 | High | **The preview blamed the link.** With the newest record 64 hours old, Sam's preview read: "Availability is not being shared right now. This link does not currently show waking windows." Yet the link may show them. The server withholds windows when the evidence is too old to claim a current state (`portalbridge/materialize.go`), and the portal words that as a fact about the link (`portal/view.go`). The owner is left checking permissions that are fine. Nothing says that recording a night brings the windows back. |
| 1.2 | Medium | **Home's sharing line never changes.** The foot of Home reads "Server projection only; trusted sharing remains default-deny." whether no link or several are working. It is a fixed string (`backend_sync.go`, `app.go`) in developer words, so Home cannot answer "is my portal working, and what does it show?". |
| 1.3 | Low | **No route from the phone.** The companion has no sharing view. That is consistent with its role, but checking needs the computer. |

## 2. Adding a new event

**Path.** There is none inside ZeitBoard.
- Home's "In the diary" links to the week.
- The week board offers only **Add a task**, and clicking an empty slot does nothing.
- The desktop has bindings to add sleep, tasks, medications and context markers, but no event.

Fixed events come only from calendars that Data Sources reads: a file, a private address, CalDAV,
or a Google or Microsoft account.

**What it takes today.**
- *With a calendar connected:* add the event in that calendar's own app. ZeitBoard reads it at the
  next hourly refresh, or after Data Sources › the calendar › refresh.
- *With none connected:* Data Sources says "Calendars: None added". Adding one means a file
  picker, a subscription address, or a sign-in, before the first event.
- *As a task:* give a task an earliest start and a latest finish of exactly the event's time, wait
  for a suggestion, and accept it. That takes about eight steps. It also refuses exactly the case
  that matters: an appointment that falls in predicted sleep is never placed. And it records a
  task, not a commitment.

**Findings.**

| # | Severity | Finding |
|---:|---|---|
| 2.1 | High | **A one-off commitment cannot be entered.** A dentist on Thursday at 3 PM is the most common thing a person with Non-24 needs to see against predicted sleep. The app can show it, warn when it falls in predicted sleep, and plan around it, but only if it arrives from another calendar. |
| 2.2 | Low | **Adding a calendar is buried.** "Add a calendar" sits in Data Sources, not Plan, though the week board is where the owner meets the gap. |

## 3. Correcting sleep and wake times

**Path.** Log opens on Sleep, with nights newest first. Each has **Edit · Exclude · Delete**, and
the page says "Edits keep the original." **Edit** opens the night in place. After saving, the row
reads "6:00 AM – 1:00 PM · CORRECTED · first recorded … 2:00 PM", with **Undo** and a correction
history. Finding the night and reading the result are both good.

**Findings.**

| # | Severity | Finding |
|---:|---|---|
| 3.1 | High | **Editing needs machine timestamps.** Sleep start and wake time open as `2026-10-25T06:00:00-04:00` and `2026-10-25T14:00:00-04:00` in plain text boxes. The hint says to keep the offset valid for the zone. Moving a wake time from 2:00 to 1:00 PM means retyping `…T13:00:00-04:00` in 24-hour time. Clocks go back on Nov 1, after which the offset is `-05:00` and a wrong guess is easy. The text boxes are deliberate (`SleepEntryForm`: `type={editing ? "text" : "datetime-local"}`), to tell apart the hour that happens twice that night. Every other night of the year pays for that hour. |
| 3.2 | High | **"Add a past night" already does it well.** The same kind of record gets date-and-time pickers ("10/27/2026 10:42 PM") with sensible defaults. Edit is the odd one out. |
| 3.3 | High | **The wrong error, in the wrong place.** Typing "Oct 25, 1:00 PM" and saving gave "Wake time must be after sleep start." But the time could not be read, and 1:00 PM is after 6:00 AM. The message appears above the log, under the "Add a past night" form, out of view of the form being edited. One error line serves both forms (`SleepLogPanel`). |
| 3.4 | Medium | **No quick fix after a mis-tap.** After **I woke up**, the app records the night and says so. If the tap was late, the fix is Log → find the night → Edit → retype a timestamp. Nothing offers "adjust" at the moment the mistake is fresh. The one exception is a long-pending "going to sleep": then the app does ask for real times, with pickers. |
| 3.5 | High (phone) | **The phone is harder still.** The companion's correction fields are text boxes labelled "yyyy-MM-dd HH:mm:ss with UTC offset", typed on a phone keyboard. Its nights are listed as raw timestamps under "Choose a source", and it speaks of "review", "snapshot" and "resolution". |

## Seen along the way

| # | Finding |
|---:|---|
| A | **Home ticks "Your calendar" when none is connected.** The setup guide counts every calendar source, including ZeitBoard's own "placements" source, created the first time a time is accepted (`app_setup_guide.go`: `len(calendars) > 0`). Data Sources rightly says "Calendars: None added". |
| B | **Three views disagree on old evidence.** With the newest record 64 hours old, Home withholds the forecast, rightly. Plan still suggested "Paperwork, today 7:00 – 8:30 AM", labelled "In a predicted waking window", with High confidence and "about 66 hours after you woke". The week board draws the same time inside today's "Sleep may begin" band. The planner should not be surer than Home, and the views should agree on where that morning stands. |

## Recommendations, in order

1. **Correct with pickers.** Use the date-and-time pickers of "Add a past night" in Edit, on both
   computer and phone. Ask which one is meant only when a time falls in the hour that repeats at
   a clock change. Show errors beside the form being edited, and say "This time cannot be read"
   when it cannot. (3.1–3.3, 3.5)
2. **Let the owner add an event.** Offer **Add an event** beside **Add a task** on the week board
   and under Home's "In the diary": a title, a day, a start and an end. Keep these as ZeitBoard's
   own busy events. The planner then avoids them, and Home marks one that falls in predicted
   sleep. Write them to the owner's calendar when writing is on (ADR-0065). This needs a
   decision record, and the owner's choice of whether events stay on the computer or sync. (2.1)
3. **Say why the portal shows nothing.**
   - The owner's preview should give the reason, e.g. "Sam sees no times because your newest
     record is 2 days old. Record a night to share again."
   - The visitor's page should say "There are no current waking windows to show", without blaming
     the link.
   - The visitor learns nothing new about the owner's records. (1.1)
4. **Make Home's sharing line true.** Say "One link working · Sam sees no times right now", with
   a link to Preview, or remove the line. (1.2)
5. **Adjust right after a tap.** For a minute after **I woke up**, offer "Adjust the times" with
   the pickers. (3.4)
6. **Fix the setup guide's calendar check**, so ZeitBoard's own placements do not count. (A)
7. **Hold suggestions to the freshness Home keeps.** On evidence too old for Home, the planner
   should not offer times at High confidence, and the planner and the week board should place
   that morning the same way. (B)
