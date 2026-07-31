# ADR 0029: Availability portal foundation (split store and projection firewall)

- Status: accepted
- Date: 2026-07-31
- Implements slice P5-a of [`portal-design.md`](../portal-design.md) and the
  phase-5 goal in [`phase-goals.md`](../phase-goals.md).
- Extends [ADR-0016](0016-proposal-queue.md) (the queue a visitor request will
  later join), [ADR-0017](0017-server-erasure-tombstones.md) (erasure reaches
  portal state too), and [ADR-0022](0022-local-sleep-import-and-real-history-validation.md)
  (the measured accuracy that bounds what may be claimed publicly).
- Supersedes nothing. Requests, messaging, and the live layer remain P5-b..P5-d.

## Context

Every surface built so far is reached by someone the user has already
authenticated: a device with an enrollment token, or a process on the user's
own machine. The portal is different in kind. It accepts traffic from people
the user has only shared a link with, against the same daemon that stores
sleep history and medication evidence. This is the largest threat-model change
in the project.

The temptation is to add a handler to the existing API and filter the output.
That fails the wrong way: a filtering bug is a health-data disclosure, and the
filter is the only thing standing between an anonymous request and the private
store. The private store handle would be one field dereference away.

## Decision

1. **A separate portal database, and a package that cannot reach the private
   one.** `internal/portal` owns `zeitboard-portal.db` and holds link hashes,
   passcode hashes, materialized window snapshots, sessions, rate buckets, and
   a coarse access audit. It contains no sleep record, medication row, task
   detail, or private label. The package does not import `store`,
   `readmodel`, `estimation`, `domain`, or `api`, and a test parses the
   package's own imports to keep that true. A public handler therefore cannot
   reach a sleep session by following an import — not merely "does not today".

2. **The materializer is the only inbound path, and it lives outside the
   portal package.** `internal/portalbridge` is the one place holding both a
   private read model and the portal store. It narrows a `PhaseEstimate` to
   `{version, windows[startAt,endAt,zoneId], generatedAt, horizonEnd, status}`
   and nothing else. Window IDs, estimate IDs, explanations, input session
   IDs, and confidence are dropped at that boundary.

3. **Confidence labels are withheld deliberately.** ADR-0022 measured the
   buckets inverted on real history: the High bucket hit 0.61 against Medium's
   0.81. Publishing a label that is anti-correlated with accuracy would
   misinform a visitor making a scheduling decision, so the public surface
   shows a window and a plain-language uncertainty statement instead. This is
   a data-driven omission, not a UI simplification, and it stays until
   calibration is fixed and reverified.

4. **Freshness is part of the claim.** Every rendered state carries the
   measured qualifier ("often off by about 2 hours, and sometimes more",
   derived from the 1.71 h median / 5.41 h P90 backtest) and a plain-language
   age. Past six hours the page says the estimate is stale; past twenty-four
   it stops making a current-state claim at all, because an out-of-date "awake
   now" is worse than no answer. `BuildView` implements those rules once, and
   the JSON path applies the same age cut, so a future live layer cannot drift
   from the no-script page.

5. **Unknown, expired, and revoked links are indistinguishable by
   construction.** `ResolveLink` returns a single `ErrLinkNotUsable` for all
   three, so the handler is never told which occurred and cannot leak it. A
   bounded timing floor is applied to resolution whether it succeeds or fails.
   That is a floor, not a constant-time guarantee, and the design says so.

6. **Every link requires a passcode, throttled rather than locked.** Argon2id
   (t=3, 64 MiB, p=4) with per-row parameters, verified with a constant-time
   compare, and a decoy KDF run when no profile matches so absence is not
   measurably cheaper than a wrong passcode. Failures arm a per
   profile-and-source exponential backoff capped at fifteen minutes. There is
   deliberately no global lockout: an attacker could otherwise use it to deny
   access to every legitimate visitor.

7. **Source-based throttling runs before link resolution.** That ordering is
   load-bearing rather than cosmetic — it is what stops an unauthenticated
   caller from using a 64 MiB KDF as a memory-exhaustion lever.

8. **Mutations are gated on browser attestation, primarily
   `Sec-Fetch-Site`.** A passcode POST predates any session, so there is no
   synchronizer token to check yet. Origin alone does not work: these
   responses set `Referrer-Policy: no-referrer`, and per the Fetch standard
   that makes a browser send `Origin: null` on a same-origin form submission.
   Gating on Origin alone refused every real login — a defect the unit tests
   could not see, because they set the header themselves, and which only
   surfaced when the page was driven in an actual browser. Relaxing the
   referrer policy to recover Origin would have put link tokens into `Referer`
   headers, which is exactly what the design avoids. `Sec-Fetch-Site` is
   unaffected by referrer policy and cannot be set by page script, so it is
   the primary signal, with an exact Origin match as the fallback for clients
   predating Fetch Metadata. A request carrying neither is refused.

9. **Pseudonymous abuse data, and rotation that is real.** Source identifiers
   are a keyed HMAC of a normalized address; the raw address is never stored.
   IPv6 is collapsed to its /64 because a single host is routinely given a
   whole /64 and could otherwise defeat per-source limits for free. Rotation
   mints a new key, deletes audit rows past the retention window, and drops
   the old key — rotation that kept the old key would be rotation in name
   only. This is pseudonymity, not anonymity: NAT groups distinct people and a
   changing network splits one person. `privacy.md` states that.

10. **The audit table cannot hold free text.** Events come from a closed enum
    and `RecordAccess` rejects anything else. There is no column for a URL, a
    user agent, or visitor-supplied text, so link tokens and visitor content
    cannot accumulate there by oversight.

11. **Retention is enforced, not documented.** The daemon runs an hourly sweep
    that expires sessions, drops stale rate buckets, and deletes access-audit
    rows past a 30-day window. A retention policy that nothing executes is not
    a retention policy.

12. **Private labels stay private.** The owner's name for a link ("Mum", a
    clinician) is encrypted in the *private* database. The portal receives an
    opaque profile ID. A full compromise of the public surface reveals that a
    link exists, not who it was shared with.

13. **Off by default, and absent rather than refusing.** `portal.enabled`
    defaults false. When it is false the daemon never constructs a portal
    handler, never opens the portal database, and never registers the owner's
    sharing routes — there is no `/p/` path to probe. An enabled portal
    without an exact `publicOrigin` fails to start rather than serving with
    its CSRF check disabled.

14. **Revocation deletes, it does not merely flag.** Revoking drops every
    session for the profile immediately and deletes the materialized snapshot,
    so an already-authenticated visitor loses access at once and no
    availability data survives in the public database.

## Consequences

- The estimator runs on the owner's side only. Public reads serve a
  materialized row, so visitor traffic cannot drive estimator cost.
- Cross-database writes cannot be atomic. P5-b's request path needs the
  transactional outbox described in design section 2; this slice avoids the
  problem by making the flow one-directional.
- Materialization is currently synchronous inside sync push and erase. That is
  acceptable at the present snapshot cost and keeps "the page reflects what
  the user just changed" true, but it is the first thing to move to a
  background worker if push latency becomes visible.
- The portal database is encrypted with a key derived one-way from the daemon
  data key. Reading the portal database never yields the private key, but the
  two are not independently rotatable. Separating them is future work.
- Sharing a live projection is inherently observable: a recipient who watches
  the link sees the user's rhythm shift over time. That cannot be engineered
  away, so it is disclosed in the owner's create-link response and must be
  shown before a link is created.

## Residual risk

- A recipient can screenshot or remember what they saw. Revocation cannot
  reach that, and the disclosure says so.
- Availability windows are health-adjacent by nature. The portal never names a
  disorder, medication, observation, or treatment, but a person's waking
  pattern is still personal information, which is why sharing it is opt-in,
  passcode-gated, expiring, and revocable.
- The timing floor bounds enumeration signal rather than eliminating it.
- No independent review has run against this surface yet. Exposure-gate item 6
  in `portal-design.md` remains open, and public exposure stays prohibited
  until it and the rest of the gate pass.

## Verification

`docs/verification.md` records the measured results: the canary suite (private
values planted in device labels, observation IDs, source record IDs, task
titles, correction IDs, and the share label; asserted absent from every public
response, every response header, and the portal database file bytes including
the write-ahead log), the recursive DTO allowlist, the indistinguishable-failure
check, the freshness ladder, the Fetch-Metadata matrix, the throttle and
backoff behaviour, and the routes-absent-when-disabled checks at both the
daemon mux and the owner API.
