# ADR 0057: Times are written on the reader's clock

- Status: accepted
- Date: 2026-10-11
- Delivers "localization readiness" (roadmap phase 2; limitation 11) as far as it goes without a
  translation.

## Context

ZeitBoard is English only, and that is accepted for the private release. What is not acceptable is
that the desktop writes every time on a twelve-hour clock. Much of the world reads 15:40, not
3:40 PM. For a rhythm that drifts round the clock, a twenty-four-hour time is also easier to reason
about: a sleep from 03:40 to 11:55 needs no a.m. or p.m. to be read correctly.

The phone already follows its own 24-hour setting. The desktop wrote its times with about seventy
fixed layouts in Go and several hand-built "AM" and "PM" strings in the window. Following the
reader's clock was therefore not one change but seventy, and nothing stopped the seventy-first.

## Decision

1. **One formatter per runtime.** Go code writes times through `core/civilfmt`, which takes the
   clock as a value and leaves the rest of a layout alone. The window writes them through
   `utils/clockFormat.ts`. The phone keeps using its own setting, in the places it did not yet.
2. **The desktop follows Windows unless told otherwise.** Its clock is the one in Windows' regional
   format, read at start. Settings › Display can choose 12-hour or 24-hour instead, kept on this
   computer. The window and every label the desktop writes use the same clock, and changing it
   redraws the window.
3. **Core takes the clock from its caller.** The rhythm projection writes its labels on the clock
   it is given. The server keeps the twelve-hour clock for the labels it writes, because it does
   not know who will read them.
4. **A guard keeps it that way.** A test fails if desktop or core Go code formats a twelve-hour
   time directly, and the UI lint fails if the window builds an "AM" or "PM" itself.

## Consequences

- Day and month names, sentences and plural rules stay English. They are the rest of
  localization, and they need a message catalog. Code that writes a sentence keeps it whole in one
  place, so that a catalog can replace it later.
- The visitor's availability page on the owner's server still writes a twelve-hour clock.
- Nothing stored changes with the clock. Records keep instants and "HH:MM" civil times; only what
  is shown changes.
