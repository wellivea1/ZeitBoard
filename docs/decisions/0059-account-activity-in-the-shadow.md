# ADR 0059: Activity from a Google export, by time alone, in the shadow

- Status: accepted
- Date: 2026-10-11
- Delivers roadmap slice 7's import ("Takeout / My Activity import"). Keeps
  [ADR-0031](0031-evidence-freshness-and-shadow-inference.md)'s inference out of the forecast,
  since its gate has not passed.

## Context

Recorded nights begin when someone starts recording. Many people arrive with years of a drifting
rhythm they never wrote down, and much of it is implied by when they used their phone or searched.
Google keeps that as "My Activity", and Takeout exports it. There is no API for it, only the
export. The export is also among the most private files a person owns, holding every search, video
and place.

ADR-0031 built candidate sleep from long quiet stretches and measured it against a gate it did not
pass, so inferred sleep stays out of the forecast. Until now, nothing in the app produced
candidates.

## Decision

1. **The person brings the export.** ZeitBoard reads a Takeout `.zip`, or a My Activity file from
   one, that the person chose. It never signs in to Google. My Activity exported as HTML is refused
   with how to ask for JSON instead.
2. **Time alone.** Each activity entry gives up its instant, to the minute, and nothing else. What
   was searched, watched or visited, the product, links and locations are passed over while the file
   is read. They are never stored, logged or shown. Beside the times, ZeitBoard keeps how many
   entries were read and when.
3. **Local and removable.** The times stay in the local database. They are never synced, shared or
   given to an assistant, and one action removes all of them.
4. **In the shadow.** A gap of three to fourteen hours between moments of activity becomes a
   suggested night, by ADR-0031's rules with a source kind of its own. Suggested nights are shown
   beside recorded ones, with how many recorded nights they match and how far their boundaries are.
   They reach no forecast, plan, snapshot, link or projection. ADR-0031's gate still decides when
   that may change.

## Consequences

- Rhythm › Sources gains a figure of suggested nights against recorded ones, and an agreement
  measured only on the nights the person recorded. An unrecorded night is not evidence of wakefulness,
  so a suggestion there counts neither for nor against. The figure opens on the last twelve weeks,
  where its rows are tall enough to compare; the whole export is one press away.
- A large export is read as a stream. The file is never held whole, one entry may use at most a
  megabyte, and only the minutes are kept between preview and import.
- Not done: taking a suggested night into the record. That needs a correction prompt and is still
  to come. Other services' exports and HTML exports are not read either.
