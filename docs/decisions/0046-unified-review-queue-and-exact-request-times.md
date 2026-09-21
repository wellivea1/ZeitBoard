# ADR 0046: Unified review queue and exact request times

- Status: accepted
- Date: 2026-09-21
- Implements part of completion-plan C2; corrects a C1 analysis regression.
- Supersedes separate counts and list contracts in ADR-0016 and ADR-0030.

## Findings

Home, navigation and Plan counted only local proposals. Visitor requests had
screen-local drafts, no usable history or pagination, and their desktop decoder
rejected the actual server response's `schema_version`. The server filtered
visitor requests after limiting a mixed proposal list, allowing newer assistant
work to hide older requests. Terminal desktop pagination omitted the cursor that
the frontend required. A failed local decision lost its error during refresh.
The visitor picker sent ambiguous wall-clock strings that Go resolved using the
host time zone without asking which repeated-hour occurrence the owner meant.

The live analysis cache also accepted the planner's stable half-hour time anchor.
A planner read could replace the current snapshot, move the next worker deadline
backwards and cause a cycle of native change events and foreground repairs.

## Decision

One root review provider combines local, backend and visitor state. Home, Plan,
navigation and the assistant's Approvals link consume the same pending count.
The queue offers working source filters, explicit incomplete/loading states,
source errors, pagination and a shared history view. Local undo remains local;
backend approvals do not claim to have written a local calendar event.

Backend and visitor routes select their scope before pagination. Scoped opaque
cursors prevent accidental cross-route reuse. Each page and authoritative pending
count are read in one transaction; counts include all actionable items, not just
the loaded page. Unused approval nonces and expiry determine actionability. The
next expiry drives a refresh timer, with bounded retries after unavailable reads.
Expiry comparisons use UTC seconds, matching the signed token, rather than lexical
RFC3339Nano ordering. Tokens expire at the boundary itself.

Calendar and Approvals render the same visitor card and share its draft and
serialized decision state. The picker enumerates actual instants in the local IANA
time zone. A skipped time has no valid choice; a repeated time requires an explicit
UTC-offset occurrence when edited. The desktop receives exact RFC3339 instants,
preserves precision and submits the reviewed block for server validation. The
owner-facing disclosure remains beside the controls; visitor text stays outside
assistant/provider context. History labels the requested window as requested and
does not invent an accepted calendar placement.

A decision acknowledgment is distinct from the following list read. If the server
commits but refresh fails, the UI removes that item's authority to decide again,
retains the confirmed decision and visibly requires a refresh. A transport failure
without confirmation remains uncertain. Refreshing a list does not erase a failed
decision message. Generation checks prevent older page reads from replacing later
state, and initial loading tolerates React StrictMode remounts.

Contextual planning analysis uses the same estimator on a source snapshot without
publishing to the live cache or changing its worker journal/deadline. Current-time
views continue to consume the durable shared analysis snapshot.

## Pre-release simplification

There is one current response/decision contract. Remove the permissive first-page
pagination adapter, redundant local-string visitor bounds and wall-time decision
input, and the six historical route aliases plus their lint/tests. Use the current
five destinations and Plan/Log tab routes. No schema migrations or old-client
negotiation are added. External calendar formats and current-data durability remain
required.

## Remaining acceptance

C2 remains open for reviewed batch decisions, task conflict resolution, complete
constraint UX, and opt-in CalDAV write-back with revision checks, retry and undo.
This queue does not supply those features. C1 native lifecycle qualification and
C3–C8 software and operational gates also remain active. Browser mocks and TLS
contract tests are synthetic evidence, not production portal or device qualification.
