# ADR 0074: Choices that travel between devices

- Status: accepted
- Date: 2026-11-17
- Builds on [ADR-0057](0057-times-on-the-readers-clock.md) and
  [ADR-0070](0070-a-status-the-owner-reports.md).
- Answers finding 4 of the [settings architecture review](../settings-architecture-review-2026-11-15.md).
- Chosen by the owner on 2026-11-17, from three options.

## Context

Every choice the owner made was kept on the device it was made on. For most of them that is
right: a theme or a window's notes belong to one screen. Two kinds mislead when devices
disagree. Turning off "let links show what I report" on the computer left it on on the phone,
and a person who turned sharing off once could believe it was off. And the clock is how every
time is written, so a 12-hour computer beside a 24-hour phone reads two ways.

These are the first choices to leave a device, so the decision had to say which ones, where
they go, and which wins.

## Decision

1. **Three choices travel.** Whether a status the owner reports may show on share links by
   default (`share_status_by_default`), whether the sleep buttons also report one
   (`follow_sleep_taps`), and the clock (`clock`: the device's own format, 12-hour or 24-hour).
   Everything else stays on its device.
2. **A choice is kept as records.** `setting` joins the sync contract. Each change, on any
   device, is one immutable record of `setting_id`, `name`, `value` and `changed_at`. It goes to
   the owner's own server, as their records do, and nowhere else. `core/sharedsetting` holds the
   type and its rules.
3. **The newest choice stands.** For each choice, the record made latest stands on every device;
   two made in the same instant are told apart by their ids, so every device settles on the same
   one. A device dates a new choice after the one standing, so a device whose clock is behind
   cannot bury a new choice under an old one.
4. **Each device keeps its own copy, and follows.** The computer keeps the values in use in
   `status.json` and `regional.json`, records each change before storing it, and after each sync
   puts in use whatever now stands, then tells the window. The phone keeps each choice with the
   time it was made; at each sync it takes a choice made later elsewhere and sends one of its
   own made later than any standing, including one made before it was connected.
5. **A choice never made is not sent.** A device's defaults are not a choice: until the owner
   picks something, the other devices' choices are followed and nothing is sent.
6. **Each device says it follows.** The sharing switch and the clock, on both devices, say that
   the other follows the choice through the owner's server and that a change made there is
   followed here.
7. **The phone gains the computer's three clocks.** "As this phone is set", 12-hour and 24-hour,
   where it had a 24-hour switch. "As this phone is set" follows Android's own format, as the
   computer's follows Windows'. The phone's old switch is not read; it starts as this phone is
   set, before the first release.

## Consequences

- Sharing turned off on one device is off on the other after both have synced, and the owner is
  told so beside the switch.
- The phone has no sleep buttons, so it keeps `follow_sleep_taps` without showing it; the choice
  matters between two computers.
- Setting records are few and tiny, and are not pruned. Erasing all data on the computer removes
  its copy.

## Not decided here

- Theme, reduced stimulation, the night rule, closed notes and reaching hours, which only matter
  between two computers and would put the names the owner gave people on their server.
- Settings an assistant may propose changes to.
