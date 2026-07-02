# ADR 0016: Approvals unification and sync pull robustness

- Status: accepted
- Date: 2026-07-02
- Builds on [ADR-0010](0010-assistant-backend-byok.md) (one-use approval
  tokens), [ADR-0012](0012-mcp-agent-connector.md) (propose-only agent tools),
  [ADR-0014](0014-local-sleep-export-erasure.md) (hard erasure), and
  [ADR-0015](0015-desktop-backend-sync.md) (opt-in desktop sync).

## Context

After ADR-0015 the *data* loop was closed, but approvals lived in two
disconnected worlds: the desktop's in-session queue over its local scheduler,
and the backend's persisted proposals (assistant/agent origins) with one-use
signed decision tokens. The M2 decision endpoint additionally required the
deciding device to be the *creating* device, and the token was only returned at
creation — so the desktop could not decide an agent's proposal at all.

Separately, the ADR-0015 pull loop failed the whole batch when a synced
correction targeted an observation missing locally (typically hard-erased under
ADR-0014). Because the cursor only advances after a fully successful batch, one
permanent orphan could wedge sync forever.

## Decision

1. **Any enrolled device may decide a pending proposal.** All devices belong to
   the single user; requiring decider == creator added no security and blocked
   the human-approval UI. The decision endpoint still requires device
   authentication, a valid signed token, and records the *deciding* device in
   the audit trail. The token's claims remain bound to the proposal, action,
   creating device, exact payload hash, single-use nonce, and expiry.
2. **The proposal list carries the decision token for pending, unexpired
   proposals.** The token is deterministic over the stored claims and the
   proposal's single unused nonce, so it is re-minted at list time rather than
   stored. Re-minting does not widen the one-use guarantee: every copy shares
   the same nonce, and the first decision consumes it for all of them. Decided
   or expired proposals never carry a token.
3. **Desktop synced-approvals surface.** New Wails bindings `GetBackendProposals`
   and `DecideBackendProposal` list the backend's proposals (humanized:
   civil-time window, confidence, explanation-code reason labels) and decide
   them through the backend endpoint. The Approvals screen shows a "Synced
   backend" panel with the same accept/reject interaction as the local queue,
   a polite live-region announcement, and — per "omit, don't disable" — the
   panel is entirely absent when sync is off (zero network calls). The
   in-session local queue remains the offline path.
4. **Pull skips permanent orphans.** `ErrSleepObservationMissing` is a typed
   storage error; the pull loop skips a synced correction whose target is
   absent (counting it in sync status as skipped) instead of failing the
   batch, so the cursor always advances. Orphans are erasure artifacts: the
   user deleted the target, so dropping its corrections on this device matches
   ADR-0014's intent.

## Consequences

- The control loop is closed: an agent or assistant proposal created on the
  backend can be reviewed and decided from the desktop, audited with the
  deciding device, and can never be applied by any agent path (ADR-0012's
  no-approve invariant is unchanged).
- Listing proposals is now a capability-granting read *for enrolled devices*:
  any device with a valid device token can decide. This is the intended
  single-user trust model; device revocation (M2) remains the containment for
  a lost device.
- A lost/stolen *decision token* alone is still bounded: single-use, short TTL,
  bound to one proposal and payload, and only usable over an authenticated
  device session.
- Sync can no longer be wedged by one bad record; skipped counts are surfaced
  in sync status rather than hidden.

## Next steps

Server-side erasure/tombstone propagation (roadmap slice 2) so local hard
deletes remove synced copies too; batch review and proposal expiry surfacing in
the unified queue; making tasks real (slice 4) so proposals target user-owned
tasks.
