# Availability portal design (phase 5)

> Security and product contract for the public-facing portal. Slices P5-a and
> P5-b are implemented as of 2026-07-31
> ([ADR-0029](decisions/0029-availability-portal-foundation.md),
> [ADR-0030](decisions/0030-visitor-time-requests.md)); P5-c and P5-d are not.
> Nothing here is exposed: `portal.enabled` defaults
> to false, and the exposure gate in section 12 must pass before any public
> bind. The portal is scheduling support, not medical advice, and never names a
> disorder, medication, sleep observation, or treatment.

## 1. Honest public claims

The real-history backtest in ADR-0022 fixes the accuracy budget: median onset
error 1.71 h, P90 5.41 h, and a 0.78 hit rate on roughly 14.7 h windows. Current
confidence buckets are not reliably calibrated. The public product therefore:

- shows broad likely-awake windows, never an exact predicted wake or sleep time;
- uses the qualifier "estimate from recent patterns; often off by about 2 hours,
  sometimes more" on every availability view;
- does not expose confidence labels until calibration is fixed and reverified;
- includes `generated_at` and renders its age in plain language;
- marks data older than 6 hours as stale and data older than 24 hours as
  unavailable rather than presenting an old "awake now" claim;
- answers dates beyond the seven-cycle forecast as "too far ahead to estimate";
  and
- still accepts a scheduling request when availability is unknown, while making
  that uncertainty visible before submission.

The owner chose a live dashboard. The no-JavaScript response computes the
current likely-awake state server-side from the latest allowlisted snapshot.
First-party JavaScript receives authenticated SSE updates, falls back to
60-second polling, and a no-script page refreshes every five minutes.

## 2. Trust boundaries

Public handlers must not receive a private health-store handle. Phase 5 adds a
separate portal SQLite database and narrow bridge interfaces:

```text
private store                         portal store
-------------                         ------------
sleep records                         hashed link tokens
estimator inputs       materialize -> allowlisted window snapshots
proposals/tasks       <- submit sink  visitor request envelopes + outbox
owner decisions       -> status sink  public status + decision outbox
medication facts                       passcode hashes + sessions
private profile labels                 rate buckets + redacted access audit
                                       encrypted message bodies
```

The portal store contains no sleep records, medication rows, task details,
private profile labels, provider credentials, device tokens, or general read
models. Public repositories are constructed with only the portal database.
Request creation receives a `VisitorProposalSink` exposing one idempotent
`Submit(validatedRequest)` operation; it cannot query the private store. Decision
status returns through a separate idempotent outbox consumer.

This boundary limits a public SQL injection to portal data, but it is defense in
depth, not permission to build SQL strings. Every query remains parameterized,
every JSON decoder rejects unknown and trailing fields, and every response is
serialized from an explicit DTO.

Cross-database operations cannot be atomic. Both directions therefore use a
transactional outbox and stable idempotency key:

1. Portal request transaction stores the request plus `proposal_submit` outbox.
2. The bridge creates one private pending proposal and acknowledges its ID.
3. The portal changes `queued` to `pending`; retrying either side is harmless.
4. Owner decision and any ZeitBoard calendar block commit atomically in the
   private store with a `portal_status` outbox row.
5. The portal consumer applies that status once and notifies SSE clients.

A bridge outage is shown as "request queued" rather than falsely claiming that
the owner has received it.

## 3. Public routes and middleware

Routes share the daemon's TLS listener but use a dedicated mux and middleware
chain. They never use device-auth middleware.

```text
GET  /p/{linkToken}                         passcode or dashboard HTML
POST /p/{linkToken}/session                 verify passcode, issue session
GET  /p/{linkToken}/availability            allowlisted projection JSON
GET  /p/{linkToken}/events                  authenticated SSE
GET  /p/{linkToken}/requests                request form
POST /p/{linkToken}/requests                create request
GET  /p/{linkToken}/requests/{publicID}      status DTO or recovery-code form
POST /p/{linkToken}/requests/{publicID}/session   exchange requester secret
POST /p/{linkToken}/requests/{publicID}/messages
```

The delivered requester exchange is nested under its request rather than the
flat `/request-session` in the original sketch, so the route itself names the
request the secret belongs to. `/events` and `/messages` are P5-d and P5-c.

Middleware order is request-size cap, security headers, source throttling, link
resolution, passcode-session gate, CSRF/origin gate for mutation, then handler.
The passcode-session endpoint sits after link resolution and throttling but
before the session gate.

All public responses set:

```text
Cache-Control: no-store, max-age=0
Content-Security-Policy: default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'
Referrer-Policy: no-referrer
X-Content-Type-Options: nosniff
Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()
Cross-Origin-Resource-Policy: same-origin
```

Public deployments also set HSTS at the TLS edge. HTML has no inline script or
style, third-party asset, analytics call, font origin, or LLM call. SSE sends a
heartbeat at least every 20 seconds and uses a route-specific write deadline;
the daemon's normal 30-second response write timeout must not silently kill the
stream. Each event contains only projection version and freshness, prompting a
normal authenticated DTO refresh.

Mutation requests must be attested by the browser as same-origin, and once a
session exists they additionally carry a synchronizer CSRF token.

The attestation is primarily `Sec-Fetch-Site: same-origin`, not `Origin`.
Because these responses set `Referrer-Policy: no-referrer`, the Fetch standard
requires a browser to send `Origin: null` on a same-origin form submission, so
an Origin-only gate refuses every real passcode POST. This was found by driving
the page in a browser; unit tests that set the header themselves cannot see it.
Relaxing the referrer policy to recover `Origin` is rejected, because it would
put link tokens into `Referer` headers that section 9 specifically avoids.
`Sec-Fetch-Site` is unaffected by referrer policy and cannot be set by page
script. An exact `Origin` match remains the fallback for clients predating
Fetch Metadata; a request carrying neither header is refused, and a present but
mismatched `Origin` is refused regardless of what `Sec-Fetch-Site` claims.

The authenticated cookie is named `__Host-zb_portal`, is opaque and random, and
has `Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`, and no Domain attribute.
It expires at the earlier of 24 hours or link expiry and rotates after passcode
authentication.

## 4. Share profiles and links

Private owner configuration stores:

- profile ID and private display label;
- grants: `waking_windows`, `allow_requests`, and `allow_messages`;
- required expiry, at most 90 days;
- required passcode policy; and
- created, updated, revoked, and audit metadata.

Portal data stores only the profile ID, grants needed for enforcement, expiry,
Argon2id passcode hash and parameters, token hash, projection snapshot, and
public operational state.

A link token is 256 random bits encoded base64url, displayed once, and stored
only as SHA-256. Unknown, expired, and revoked tokens receive the same generic
response shape and a bounded timing floor. The design does not promise
physically identical timing. Reverse-proxy and daemon logs must redact path
tokens; the operator runbook must disable raw request-URI logging on `/p/`.

Every link requires a passcode. Passcode failures use progressive delay and a
bucket keyed by profile plus a keyed-HMAC source identifier. There is no global
"lock this link after five failures" switch because an attacker could use it to
deny access to every legitimate visitor. Aggregate abuse can trigger owner
alerts and a temporary operator circuit breaker, but not an attacker-controlled
permanent lockout.

Revocation immediately rejects link and session tokens, closes pending portal
requests through the outbox, and hard-deletes portal messages and requester
sessions after bridge acknowledgement. Minimal private proposal decision/audit
rows remain under the normal audit policy. Calendar blocks already accepted by
the owner are owner data and are not deleted merely because a share link was
revoked.

## 5. Projection materialization

The owner-side materializer writes this only:

```json
{
  "version": 42,
  "windows": [
    {"startAt": "...", "endAt": "...", "zoneId": "America/New_York"}
  ],
  "generatedAt": "...",
  "horizonEnd": "...",
  "status": "available"
}
```

`status` is `available`, `refused`, or `insufficient_data`. No internal estimate
parameters, observation IDs, confidence buckets, medication facts, markers, or
free text cross the materialization boundary. Materialization runs after an
accepted sync push that changes sleep inputs, an estimate-affecting edit or hard
erasure, and estimator version changes. It publishes by transaction and then
notifies open streams.

A response-shape test recursively rejects every key outside this allowlist. A
fixture seeded with distinctive canary values in every private table verifies
that no canary appears in HTML, JSON, SSE, errors, logs, or access audit.

## 6. Visitor scheduling requests

A request DTO is:

```text
window_start       RFC3339 instant
window_end         RFC3339 instant
zone_id            IANA zone used for the visitor's display/input
duration_minutes   optional; 15..480 and no longer than the window
handle             optional private visitor text; at most 40 characters
message            optional private visitor text; at most 500 characters
```

The server normalizes instants, rejects overflow and ambiguous malformed local
input, requires `window_start >= now`, and limits the requested window to eight
hours. There is no product-level upper date horizon, per the owner's decision;
normal timestamp representation limits still apply. Dates beyond the forecast
horizon are accepted with a required warning and a `beyond_horizon` flag.

`handle` and `message` are visitor-supplied private request data. The product
must not claim it "never accepts names." They are escaped on render, stripped of
control characters, encrypted at rest in the portal store, omitted from public
projection DTOs, and never copied into notification titles or access logs.

The private proposal has origin `visitor` and the same one-use owner decision
authority as every other proposal. Pending requests appear as a neutral band
covering the requested window. Approval requires the owner to choose an exact
calendar block within that window. A supplied duration must be preserved; when
no duration was supplied the owner chooses one in the dialog. The private
transaction atomically consumes the decision token and creates the
ZeitBoard-owned block.

An approved visitor learns the exact block the owner explicitly selected. This
is necessary coordination information and must be called out in the approval
UI. Rejection reveals only that the time was declined, never a reason, sleep
state, calendar conflict, or medication fact. V1 has no counteroffer authority;
a time outside the requested window requires a new request flow.

## 7. Requester authentication and messaging

Request creation returns a public request ID plus a second 256-bit requester
secret. The secret is placed in a URL fragment, never a path or query. First-
party JavaScript exchanges it once by POST for a request-scoped HttpOnly cookie,
then calls `history.replaceState` to remove the fragment. A no-script fallback
displays a one-time recovery code that the visitor can enter on the status page.
Neither form is written to server or proxy logs.

The request cookie is scoped to one link and one request. Knowing the shared
link and passcode is not enough to enumerate another visitor's status or thread.
Public request IDs are random, not sequential.

Messages are plain text only, no HTML, links, attachments, or embeds. New
messages close when the request is decided. Encrypted bodies remain available
for 14 days so both parties can read the completed exchange, then a tested hard-
delete job removes them. The owner can erase a thread sooner. Minimal decision
audit does not retain message bodies.

## 8. Abuse resistance and privacy-preserving audit

Persisted limits survive restarts. P5-a implements the read limit, the passcode
backoff, and the body cap; the SSE, request-creation, and message limits arrive
with the slices that introduce those operations. The delivered read limit is
keyed on source alone — a per-session limit adds little while the only
unauthenticated surface is enumeration, and it is added in P5-b when sessions
begin carrying write authority.

- page/availability reads: 120 per hour per authenticated session and source;
- SSE: 2 streams per session, 20 per profile, with excess clients polling;
- request creation: 5 per day per session and 20 pending per profile;
- messages: 20 per day per thread and 100 per day per profile;
- passcode attempts: progressive per profile-and-source delay; and
- request bodies: 4 KiB before JSON decode.

Source identifiers are a keyed HMAC of normalized IP using a rotating audit key;
raw IP is discarded immediately. Rotation and retention are documented. This is
pseudonymous abuse data, not anonymity: NAT can group people and changing
networks can split one person. The Sharing screen shows coarse last-access and
abuse events without exposing raw identifiers.

Request work never invokes the estimator or an LLM. Availability reads serve a
materialized row. Queues and SSE connections have hard global bounds so creating
many share profiles cannot bypass per-profile limits.

## 9. Threat-model delta

Phase 5 adds these adversaries and residual risks:

- A recipient forwards a link and passcode. Expiry, revocation, audit, and a
  required passcode reduce exposure; screenshots and remembered information
  cannot be revoked.
- A scraper enumerates tokens. High-entropy hashed tokens, generic failures,
  timing floors, rate limits, and no indexing reduce this; traffic volume still
  reveals that a host exists.
- A visitor harasses through requests/messages. Caps, per-link disable, thread
  close, and one-action revoke limit this; the owner may still see abusive text.
- A public handler is compromised. Separate storage, explicit DTOs, narrow
  outbox sinks, process-level request limits, and no private-store handle limit
  reach; portal requests and already-public windows remain exposed.
- Availability changes enable traffic analysis. That is an inherent consequence
  of sharing the live projection. The UI must explain it before link creation.
- Link tokens leak through browser/proxy history. Fragment-only requester
  secrets, no-referrer, no-store, HSTS, and log-redaction guidance reduce but do
  not eliminate endpoint metadata.

This delta must be integrated into `docs/threat-model.md` and `docs/privacy.md`,
not merely linked, before exposure.

## 10. Owner UI

The Sharing screen provides profile state, expiry, coarse last access, create or
edit grants, required passcode reset, one-time link display, exact recipient
preview, revoke, and audit/erasure controls. The preview renders the same DTO and
template as the public page without an iframe or a live public token.

Approvals adds a neutral `visitor` origin, private handle/message details, and
an explicit disclosure of what approval reveals. Calendar and Approvals call the
same decision service; simultaneous decisions race on the same one-use token and
only one can commit.

## 11. Delivery slices

| Slice | Scope | Acceptance spine |
|---|---|---|
| P5-a **(delivered 2026-07-31, ADR-0029)** | Separate portal store, security middleware, profile/link CRUD, allowlisted materializer, read-only public page | Canary leak test; exact DTO schema; stale/unavailable behavior; uniform generic failures; portal disabled by default |
| P5-b **(delivered 2026-07-31, ADR-0030)** | Request validation, requester-secret exchange, outbox bridge, visitor proposals, exact-slot approval | Queued/pending behavior under bridge failure; idempotent retry; one-use decision race; approved slot is inside request; no private-store reads from public package |
| P5-c | Encrypted threads, owner replies, caps, close and hard-delete jobs | Cross-request authorization tests; content never reaches projection/logs; 14-day deletion with clock-controlled tests |
| P5-d | SSE/polling, rate persistence, audit UI, threat/privacy/runbook integration, reverse-proxy profile | Connection bounds; CSRF/header suite; log-token redaction; restart persistence; external red-team pass |

Phase 6 consumes only internal `request_created`, `request_decided`, and
`message_added` events. Notification transports do not receive portal-store or
health-store access.

**P5-c and P5-d are paused as of 2026-08-04**, by the priority correction in
[`automaticity-review-2026-08-04.md`](automaticity-review-2026-08-04.md):
messaging and a live layer are downstream of source freshness, and the portal
cannot honestly publish a live status while the owner's own current-state claim
has no freshness policy. Paused means maintained and tested, not cancelled —
this design stands, and the delivered P5-a/P5-b surfaces stay green. When the
phase resumes, the sequence is: wire the desktop Sharing screen, centralize
freshness, complete the independent review, ship read-only availability, then
requests, and only then evaluate whether messaging is needed at all.

The transactional outbox in section 2 is implemented in both directions as of
P5-b. Section 7's messaging threads and the second requester exchange remain
P5-c; SSE and the audit UI remain P5-d.

## 12. Exposure gate

Public exposure is prohibited until all of the following are true:

1. `portal.enabled` defaults false and a test proves public routes are absent.
2. P5-a through P5-d acceptance suites pass on Linux and Windows server builds.
3. Threat model, privacy policy, operator TLS/reverse-proxy runbook, retention,
   and incident/revocation procedures are merged.
4. Response/log canary tests show no health or secret leakage.
5. CSRF, session-cookie, CSP/header, passcode-throttle, request-enumeration,
   queue-bound, SSE-bound, and hard-erasure tests pass.
6. An independent review resolves every high- or medium-severity finding.
7. The owner UI states the measured uncertainty and live-sharing residual risk.

## 13. Resolved owner decisions

1. Live dashboard: yes, with SSE/polling/no-script layers and visible freshness.
2. Passcode: required for every link; no attacker-triggerable global lockout.
3. Request horizon: no product cap; beyond-forecast requests carry an honesty
   warning and `beyond_horizon` marker.
4. Calendar integration: yes; approval chooses one exact block inside the
   requested window and atomically records the decision plus calendar block.
