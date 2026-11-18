# ADR 0073: A night recorded away from home

- Status: accepted
- Date: 2026-11-17
- Revises the holding rule of [ADR-0032](0032-android-sleep-synchronisation.md).
- Builds on [ADR-0057](0057-times-on-the-readers-clock.md).
- Chosen by the owner on 2026-11-17, from three options.

## Context

Health Connect stores a UTC offset for each end of a sleep session, not a time zone. A v1
observation names an IANA zone, and ADR-0032 refused to invent one: a session whose offsets
were not the home zone's at those instants was held on the phone and counted as held. That kept
a record from contradicting its own evidence, and it also meant every night recorded while
travelling was missing from the estimate, the forecast and every other device, for good.

The estimate does not need a zone. It fits the instants a sleep began and ended, and every
device draws them on the home clock (ADR-0057). What a night away needs is a way to say which
clock it was recorded on, without pretending it was the home one.

## Decision

1. **A night away carries its own clock.** An observation gains an optional `source_offset`:
   the offset, in seconds east of UTC, at each end of the sleep. It is sent only when it is not
   the zone's own at one end or the other, so a record never says the same thing two ways, and
   it is sent with both ends or not at all. `zone_id` stays the zone the owner reads their
   records in.
2. **It counts like any other night.** A night away syncs, joins the estimate and is drawn on the
   home clock. The owner can leave any night out with a correction, as before.
3. **Every surface says where the clock stood.** The desktop's sleep log and the phone's review
   say "Recorded 6 hours ahead of home" or "behind home", from the start of the night, or from
   its end when the clock changed during it. The server's review context carries
   `source_offset`.
4. **Only a night with no clock is held.** A session that recorded no offset at all says
   nothing about the clock it was on, so the phone still holds it, counts it as held, and says
   why. A correction made on the phone waits only for that reason now; one held under the old
   rule is looked at again.

## Consequences

- Travel stops leaving holes in the record. The first night after a long flight may sit off the
  fit, and the owner can leave it out.
- A night recorded away and then corrected keeps its clock: a correction changes times, never
  where the clock stood.
- Records synced before this change are unaffected. Nights the phone held within its 30-day
  read start to sync on its next read; older ones are past what a read reaches.

## Not decided here

- Moving the home zone itself while travelling. The home zone is still set by the owner.
