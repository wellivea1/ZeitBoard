# ADR 0056: Reaching several people, each on their own hours

- Status: accepted
- Date: 2026-10-11
- Completes slice 16 of the roadmap ("Configurable reaching hours"), which shipped one schedule
  with the same hours on every chosen day and named both limits as not done.

## Context

The 48–72 hour outlook says when the person is likely awake while someone they need is
available. It allowed one such someone, open the same hours on each chosen day. A drifting rhythm
rarely meets only one schedule. A clinic opens Tuesday mornings and Thursday afternoons. A
pharmacy closes later on Fridays. Family keep their own hours in their own zone. Fitting all of
that into one uniform schedule either drops people or misstates their hours.

## Decision

1. **The outlook takes several schedules.** Each one has its own zone and its own hours for each
   weekday: open or closed, with an opening and a closing time. A close at or before the opening
   still runs into the next day, and equal times still mean all day. Each schedule carries an
   opaque ID, and every reaching window says which schedule it came from. `core/outlook` knows no
   names.
2. **The desktop keeps a list of people.** Each person has a private label, a zone, hours for each
   weekday, and an "off" switch that keeps the hours without showing them. The list holds at most
   five people, because Home draws a row for each. An empty list means nobody to reach, and the
   outlook then says nothing about reaching anyone.
3. **The old schedule carries over as one person.** The previous file becomes one person with the
   same hours on each of its days. A schedule that was off becomes a person that is off, so no
   hours are lost. The new file replaces the old one only after it is written.
4. **Labels stay on this computer.** Home and Settings show them. The assistant snapshot names a
   person only as `contact_01`, `contact_02` and so on, in list order, as it already does for
   commitments.

## Consequences

- Home's figure has one "reachable" row per person shown, labelled with their name, and the text
  alternative lists each window under the person's name. Someone with no hours in the days shown
  keeps an empty row, so the setting is visibly in force.
- The summary of each schedule groups days with the same hours, in week order from Monday
  ("Monday to Friday, 9:00 AM to 5:00 PM; Saturday, 10:00 AM to 1:00 PM").
- Five people is a display limit, not a data one. More would need a different figure.
- The snapshot holds up to 24 reaching windows, up from 16, because five people can have about
  twenty in three days. The server's assistant accepts the `contact` ref and passes it to the
  model. It still accepts a window without one, from a desktop built before the list.
