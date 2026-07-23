# Availability portal design (phase 5)

> Implementation-ready design for the public-facing portal: expiring shared
> links that show when the user is likely awake, answer availability
> questions, and accept time requests that land in the approval queue.
> This is the largest threat-model change in the project's history; the
> threat-model v2 section below must be folded into `threat-model.md` and
> `privacy.md` **before** any public exposure. Not medical advice; the
> portal never mentions disorders, medications, or sleep records.

## 1. What the measured accuracy allows the portal to claim

The real-history backtest (ADR-0022, `verification.md`) fixes the honesty
budget: median onset error **1.71 h**, P90 **5.41 h**, hit rate **0.78**
on ~14.7 h windows — and confidence buckets are imperfectly calibrated
(High 0.61 < Medium 0.81). Therefore:

- The portal shows **windows, never times**: "likely awake roughly
  15:00–23:00" with a visible qualifier ("estimate from recent sleep
  patterns; typically off by ~2 h, occasionally more").
- No per-window confidence *labels* on the public page until the
  calibration follow-up lands (the inversion would make "high" a lie);
  the qualifier sentence carries uncertainty instead.
- Beyond the forecast horizon (7 cycles), the portal says "too far ahead
  to estimate" — it never extrapolates for a requested date it cannot see.
- When the estimator refuses, the portal says availability is currently
  unknown and still accepts requests (they queue like any proposal).

## 2. Architecture

One new server module (`apps/server/internal/portal`) plus public routes on
the existing daemon, sharing TLS and storage but **never** the device-auth
middleware:

```
/p/{linkToken}                 GET   public page (server-rendered HTML)
/p/{linkToken}/availability    GET   projection JSON (windows + qualifier)
/p/{linkToken}/requests        POST  create time request (+ message)
/p/{linkToken}/requests/{id}   GET   status + thread (requester secret req.)
/p/{linkToken}/requests/{id}/messages POST append visitor message
```

- **Server-rendered, zero-JS-required HTML** with a strict CSP
  (`default-src 'none'; style-src 'self'`): no third-party origins, no
  analytics, no LLM anywhere on the public path. Progressive enhancement
  only for auto-refresh.
- Public handlers run behind a dedicated mux with its own middleware
  stack: rate limiter → link resolver → optional passcode gate → handler.
  They can only reach the **portal projection store**, a separate read
  model — by construction they have no code path to sync records,
  proposals, tasks, or medication tables.
- The projection is materialized by the owner's side of the house
  (recomputed when the estimate changes): `{windows: [{start_at, end_at,
  zone_id}], generated_at, horizon_end, status}`. Materialization is the
  privacy firewall: even an SQL injection in a public handler would reach
  only already-public fields.

## 3. Link and profile model

Extends §9.7 sharing profiles (ADR to confirm numbering at build time):

- `share_profile`: id, display label (private), **granted fields**
  (`waking_windows` bool — the only projection field v1 offers),
  `allow_requests` bool, `allow_messages` bool, `expires_at` (required,
  max 90 days), optional passcode (argon2id hash), `revoked_at`,
  created/updated audit.
- `share_link`: profile id + `link_token` = 256-bit random, base64url,
  stored **hashed** (SHA-256) like device tokens; shown once at creation.
  Constant-time lookup by hash; unknown/expired/revoked → uniform `410
  Gone` with identical body and timing (no enumeration oracle).
- Revocation is immediate (single delete → 410) and erases the profile's
  request threads server-side via the ADR-0017 machinery (tombstones for
  any synced copies).
- The user-facing name for a visitor ("Mom", "boss") lives only in the
  private profile label; the portal never displays or accepts real names —
  visitors optionally sign messages with a free-text handle capped at 40
  chars, rendered escaped, never echoed into notifications.

## 4. Time requests = proposals with origin `visitor`

A request is `{window_start, window_end, duration_minutes?, message?}`
(message ≤ 500 chars, control characters stripped, stored encrypted like
proposal payloads). The server validates bounds (must be ≥ now, ≤ 60 days
out, ≤ 8 h span) and creates a **pending proposal with origin `visitor`**
in the existing store: same one-use decision tokens, same queue, same
audit, decidable from any enrolled device (ADR-0016). Nothing about the
queue grows new authority — the portal cannot approve, list others'
proposals, or see the calendar.

Decision mapping, deliberately coarse: `approved` → "suggested time
accepted"; `rejected` → "couldn't make that time". The visitor never
learns *why*, and approved requests reveal only the requested window they
themselves supplied. A per-request **requester secret** (second random
token, returned once in the creation response URL) gates status reads so
one visitor cannot read another's thread on a shared link.

Messaging: a thread per request (visitor ↔ user), user side rendered in
the Approvals detail; length caps, no attachments, no HTML; thread dies
with the request decision + 14 days, then is erased.

## 5. Abuse resistance

- **Rate limits** (per link token and per IP): availability reads 60/h;
  request creation 5/day/link with 20 open-thread cap per profile;
  messages 20/day/thread. 429 with Retry-After; limits enforced in the
  portal store so they survive restarts.
- Passcode gate optional per profile (argon2id, constant-time, 5
  attempts/h then temporary lock of that link only).
- Request bodies hard-capped (4 KB); JSON strictly decoded; every string
  length-bounded exactly like sync validation.
- The public page never triggers estimator work: it serves the
  materialized projection only, so a scraper cannot induce CPU load
  beyond static reads (the "ask" feature is a projection lookup, not
  computation, not an LLM).
- Access log per profile (timestamp, route class, hashed IP) surfaced in
  the Sharing screen ("last accessed") and erasable.

## 6. Threat-model v2 delta (to fold into threat-model.md before exposure)

New adversaries: link forwarder (link is a bearer capability — mitigated
by expiry, passcode option, revocation, per-profile audit); scraping/
enumeration (hashed constant-time tokens, uniform 410s, rate limits);
harassment via requests/messages (caps, per-link disable, one-tap "block
this link" = revoke); traffic analysis of availability changes (documented
residual: anyone with the link learns the rhythm's public projection —
that is the feature; the mitigation is the owner's choice of who gets
links and the expiry ceiling); DoS on the public mux (rate limits +
optional operator allowlist/Cloudflare-free fail2ban guidance in the
runbook). Explicit non-mitigations stated honestly: a link holder can
always screenshot; revocation cannot recall knowledge already seen.

## 7. Owner-side UI

Sharing screen becomes real (retiring its remaining preview affordances):
profile list with state/expiry/last-access, create/edit with the §9.7
permission toggles (v1: waking windows + requests + messages), passcode
set, link copy (shown once), **exact recipient preview** (renders the true
public page in an iframe-free preview), revoke, access log. Approvals
gains the `visitor` origin chip (a fourth origin stripe color: neutral
ink — visitors are not agents) and the message thread panel.

## 8. Slices

| Slice | Scope | Acceptance spine |
|---|---|---|
| **P5-a Projection + links** | portal store, materializer, share profiles/links CRUD + Sharing UI, public availability page (read-only) | every public response body contains only allowlisted fields (asserted by a response-shape test); revoked → uniform 410; expiry required |
| **P5-b Requests** | request validation → origin-`visitor` proposals, requester secrets, status page, Approvals integration | request → in-app decision → visitor-visible status round-trip; queue authority unchanged (no-mutation test extended) |
| **P5-c Messaging** | threads, caps, erasure-on-close, Approvals thread panel | thread caps enforced; erasure verified; no name/health leakage in any body |
| **P5-d Hardening + exposure gate** | rate limits, passcode, access log, threat-model v2 + privacy.md + runbook (reverse-proxy guidance), red-team pass from the validation plan §12 | all limits tested; docs merged; only then may an operator expose the mux publicly (config flag `portal.enabled`, default **off**) |

P6 interface note: P5-b/c emit internal events (`request_created`,
`request_decided`, `message_added`) onto a small dispatcher; phase 6
subscribes web-push/companion transports to those events without touching
portal code.

## 9. Open decisions for the owner (before P5-a starts)

1. Availability granularity on the public page: windows only (recommended)
   vs windows + "now awake/asleep" live dot (more useful, leaks more).
2. Passcode default: off (link-only) vs required for request-enabled
   profiles (recommended: required when `allow_messages`).
3. Request horizon: 60 days proposed — shorten?
4. Whether approved requests should auto-create a calendar block via the
   ADR-0023 placement path (recommended: yes, as a *second* explicit
   approval step in-app, never automatic).
