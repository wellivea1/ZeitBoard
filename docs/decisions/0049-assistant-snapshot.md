# ADR 0049: One assistant snapshot

- Status: accepted
- Date: 2026-09-27
- Extends [ADR-0028](0028-desktop-local-agent-endpoint.md) (the desktop's local
  agent endpoint) and [ADR-0010](0010-assistant-backend-byok.md) (the chat
  assistant). Contract: `contracts/v1/assistant-snapshot.schema.json`.

## Context

What an assistant could learn about the owner's day was scattered, and each
surface assembled its own partial view:

- The chat assistant's model saw open tasks with their bounds, predicted sleep
  and waking windows, and busy calendar intervals. It did not know whether the
  owner was likely awake now, whether the estimate could be trusted, when the
  night ahead was likely to begin and end, which times the owner had already
  accepted, which suggestions were waiting for a decision, which tasks could not
  be placed and why, or which tasks needed review after edits on two devices.
- The local agent endpoint split the same world across six read tools with six
  shapes. An agent had to call them all and reconcile them, and nothing
  guaranteed they described the same moment.
- Home's lead sentence ("Sleep is likely to begin between 11:30 PM and 1:35 AM")
  was computed in the web code from the timeline's bands. An assistant could not
  get the same answer except by re-deriving that rule.

## Decision

1. **One snapshot.** The desktop assembles one versioned document of what an
   assistant may know. It covers:
   - the rhythm estimate and whether it may describe now: as on Home, what the
     owner is doing now is stated only on current evidence, and otherwise the
     explanation Home shows takes its place;
   - what the owner is doing now, when the next sleep is likely to begin, and
     when waking is likely to follow;
   - the fitted cycle, drift and typical sleep, with the fit rating and its
     caveat;
   - recent sleep: nights on record, last night's date and length, and whether
     a "going to sleep" tap is waiting;
   - the next three days: the presence timeline, reachable hours, commitments
     with their conflicts and accepted task times, suggestions awaiting a
     decision, and tasks the planner could not place;
   - what needs the owner;
   - tasks, medication timing, context markers and sync state.

   It is built from the computations the screens use — the local estimate and
   freshness verdict, the operational outlook, the local planner, and the
   medication and marker projections — so an assistant and the screen cannot
   disagree.
2. **One rule, one place.** Reading the onset and waking ranges off the
   timeline moves into `core/outlook` (`Outlook.SleepAhead`). Both the Outlook
   DTO and the snapshot carry its result, and Home's lead reads it. The
   presence-now rule (`presenceNow`) is shared by the overview and the snapshot.
3. **Structured and versioned.** The snapshot has a JSON Schema contract, uses
   typed values throughout, and marks every truncated list:
   - instants are given in the owner's zone, to the minute, with their offset;
   - durations are in minutes;
   - levels, statuses, conflicts and reasons are enums;
   - task, proposal and medication ids are the app's opaque ids.

   The desktop's tests validate its real output against the contract.
4. **Redacted by construction.** The snapshot carries no title, label, note,
   location, clinician text or raw record. Like the other agent projections, it
   carries no exact time of recorded evidence:
   - sleep is stated as Home states it: awake for about so long, to five
     minutes, and last night's date and length;
   - doses and markers are stated as their projections already state them.

   Calendar entries are numbered within the snapshot, because an imported
   entry's own id can carry its source's text.
5. **Each surface takes what its policy allows.**
   - The local endpoint serves the whole snapshot as `get_snapshot`, listed
     first as the place to start. The narrower tools remain.
   - The chat assistant sends its server the planning view: rhythm without the
     time since waking, plans, and needs-you. The model context has never
     carried a sleep record, and medication and marker questions stay on the
     server's template path. The server decodes requests strictly, so a chat
     snapshot carrying a sleep, task, medication, marker or sync section is
     refused.
   - Of the planning view, the model reads only enums, counts, opaque ids and
     civil-time ranges near now. Sentences in the snapshot, and values outside
     their allowlists, never reach it.

## Consequences

- A local agent can answer "what does my day look like" from one call, in the
  same terms as Home. The chat model can explain that a meeting falls in
  predicted sleep, that a suggestion is waiting for the owner, or that a task
  has no window, and it can decline to state a current state when the estimate
  is withheld.
- The snapshot is the contract for future surfaces (a companion assistant, a
  voice client). A new fact goes into the snapshot, its schema and its tests
  once, not into each surface.
- The desktop and the server ship together: an older server's strict decoding
  refuses the new `snapshot` field. Both live in the owner's installation, and
  the release notes pair them.
- The agent confidence vocabulary is lower-case everywhere (`low`, `medium`,
  `high`, `unknown`). The medication projection used to pass the screen's
  capitalized labels.
