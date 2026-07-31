# ADR 0030: Visitor time requests through a transactional outbox

- Status: accepted
- Date: 2026-07-31
- Implements slice P5-b of [`portal-design.md`](../portal-design.md).
- Builds on [ADR-0029](0029-availability-portal-foundation.md) (split store and
  projection firewall) and [ADR-0016](0016-proposal-queue.md) (the one-use
  decision token every proposal is decided with).
- Messaging threads (P5-c) and the live layer (P5-d) remain unimplemented.

## Context

ADR-0029 made the portal read-only: a visitor could see broad likely-awake
windows and nothing else. The portal's actual purpose is two-way — someone
with the link should be able to *ask* for a time — and that is the first thing
in the project that lets an outsider write into the user's world.

Two problems make this harder than an ordinary form.

The stores are deliberately separate, so "store the request and file it in the
owner's queue" spans two databases and cannot be one transaction. Any naive
implementation either loses requests when the second write fails or tells the
visitor their request was delivered when it was not.

And the visitor and the owner want different things from a decision. The
visitor asks for a *range*; the owner needs to answer with a *time*. Approving
"some point in this four-hour window" is not a usable answer to either party.

## Decision

1. **A transactional outbox, and `queued` as an honest visible state.** The
   portal stores the request and an outbox row in one transaction. A bridge
   turns outbox rows into pending private proposals and acknowledges them,
   which is the only thing that moves a request from `queued` to `pending`.
   Until then the visitor is told "saved and on its way", not "sent". A status
   the visitor cannot verify must not be claimed on their behalf.

2. **Submission is idempotent on the portal request id.** A unique
   `portal_request_proposals` row means a retry after a lost acknowledgement
   finds the existing proposal instead of creating a second one. This is the
   failure that actually happens — the private commit succeeds and the ack is
   lost — and it must not produce two asks in the owner's queue.

3. **Decision and delivery commit together.** The owner's decision, the
   consumption of the one-use token, the audit row, and the row that will tell
   the visitor are written in a single private transaction. A decision the
   visitor never learns about cannot happen without also losing the decision.

4. **Approval names an exact block inside the requested window.** The chosen
   block must lie within the window, and when the visitor supplied a duration
   it must be exactly that long. A shorter block silently changes the ask; a
   block outside the window is a counteroffer, which v1 does not have — that
   needs a new request, not a moved time.

5. **The generic decision route refuses visitor proposals.** `POST
   /v1/proposals/{id}/decision` cannot discharge the slot obligation or the
   delivery obligation, so it returns 409 and points at the sharing endpoint.
   Leaving it able to decide a visitor request would have been a silent way to
   skip both rules.

6. **Visitor proposals are not an agent action.** `place_visitor_request` is
   absent from the assistant's action registry, and only the bridge creates
   one. An agent that could mint a visitor-origin proposal could manufacture
   social pressure from a person who does not exist, and the user would have no
   way to tell the difference.

7. **Requester authorship is a separate secret from the link.** Creating a
   request returns a 256-bit requester secret delivered in a URL *fragment*,
   which browsers never send to a server and proxies never log, plus a
   one-time recovery code for the no-script path. Exchanging it yields a
   cookie scoped to one request under one profile. Holding the shared link and
   passcode is therefore not enough to read another visitor's request; a wrong
   secret and an unknown request return identical responses.

8. **A declined request carries no reason, ever.** The visitor learns the time
   did not work. Any reason would disclose sleep state, calendar contents, or
   health, none of which a link holder is owed. An approval does reveal the
   exact chosen block — that is necessary coordination information, and the
   owner is told so before they choose rather than after.

9. **Visitor text is private, in both directions.** `handle` and `message` are
   encrypted at rest in the portal store, encrypted again inside the private
   proposal payload, shown to the owner because judging a request requires
   them, and never placed in an availability projection, an access-audit row,
   or a log line. The product does not claim it "never accepts names" — it
   accepts them and protects them.

10. **No product cap on how far ahead someone may ask.** Owner decision 3. A
    window past the forecast horizon is accepted, flagged, and shown with an
    explicit statement that the estimate cannot reach that far. What *is*
    bounded is the window's length: an eight-hour ask is a scheduling request
    and a three-week ask is not.

11. **Local input that does not exist is rejected, not guessed.** A
    `datetime-local` value is parsed in the visitor's stated zone, and a time
    that falls in a spring-forward gap is refused. Silently resolving an
    ambiguous local time is how scheduling bugs are born.

12. **Revocation closes open requests rather than deleting them.** The owner
    may still hold a pending proposal referencing one, and a request that
    silently vanished would leave the visitor watching a status that never
    moves.

## Consequences

- The bridge is pumped when a request is created, when a decision is made, and
  every minute as a recovery path. The timer is not the happy path; it exists
  so a request that arrived while the bridge was failing still lands without
  anyone intervening.
- Because the portal package must not reach the private store, request
  creation signals the owner side through a bare `func()` rather than calling
  it. The signal carries no data, which is what keeps the import boundary from
  ADR-0029 intact.
- A visitor proposal is filed against an enrolled device, because proposals are
  keyed to a creating device and a visitor has none. With no enrolled device
  the request stays `queued` — correct, and visibly so.
- CSRF tokens are now derived from the session under a server key rather than
  stored as a hash. ADR-0029 stored only a hash, which a server-rendered form
  can never embed; that mechanism could not have worked.
- The owner's decision surface exists as an API. The desktop dialog that picks
  a block on the calendar is not built, so today an owner decides through the
  API rather than the UI.

## Residual risk

- A visitor can still send unwanted text within the caps. The owner sees it,
  which is the point; per-link disabling and revocation are the remedies.
- Approval discloses the chosen time to whoever holds the requester cookie or
  the recovery code, including anyone the visitor shared them with.
- Request rate limits are per portal session and per profile. A determined
  visitor with several sessions is bounded by the per-profile pending cap
  rather than the per-session daily cap.
- The exposure gate in `portal-design.md` section 12 is still not satisfied;
  `portal.enabled` remains false by default and no independent review has run.
