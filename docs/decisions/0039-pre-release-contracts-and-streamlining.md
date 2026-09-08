# ADR 0039: One current pre-release contract

- Status: accepted
- Date: 2026-09-08
- Owner direction: “Disregard backwards compatibility, nobody has been using this
  app yet. There are no packaged releases. See where code can be streamlined given this.”
- Supersedes the compatibility requirement introduced at the end of ADR-0038 and
  historical prototype-preservation rules in ADR-0025/0037.

## Decision

ZeitBoard is qualifying its first release. A schema change is implemented in the
current contract and all current producers/consumers together. Each capability
has one current contract. Existing v1/v2 names can remain useful identifiers;
they do not imply that multiple implementations need support. Set release-to-release
upgrade guarantees when an actual packaged baseline exists.

## Implemented simplifications

- Sleep correction provenance stays in the current sync/export schema. Removed
  the newly added alternate sync routes, per-request version dispatch, old-client
  upgrade refusals, conditional export versions and duplicated schemas/fixtures.
- Removed unused v1 medication schema/export fixtures. The actual runtime and
  generated examples use the current medication contract with explicit zones,
  reminder consent and clinician-rule text.
- Android creates the complete current database directly. Removed development
  schema 1–4 migration/backfill code and its historical fixtures. An obsolete
  development profile is explicitly refused; the app never silently recreates it.
- The server creates its current tables and correction-target index directly.
  Removed scans that reconstructed metadata for old prototypes and the old device
  column-upgrade helper. Current observation erasure still removes its correction
  payloads and rejects later resurrection.
- Removed inferred legacy provider authorship and reconciliation of obsolete
  device-dependent parent links. Omitted correction author means manual;
  automatically imported provider changes explicitly name Health Connect.
  Different payloads under the same immutable queued ID now fail visibly.
- Desktop sleep and task uploads use one batching/acknowledgment/commit loop,
  retaining their typed storage adapters. Invalid, missing or trailing response
  data cannot acknowledge pending records or advance a download cursor.

## Further useful consolidation

ADR-0040 completes the Android repository consolidation: full exchange is the only
execution path and replica storage is mandatory. It also connects reviewed manual
corrections, replaces the single-parent relation with reviewed manual heads, and
removes the prototype enrollment-scope fallback. Local-only correction production
and later enrollment still need consolidation into this current record contract.

The assistant action catalog is repeated across validation, provider presentation,
MCP tools and dispatch. The completion plan's C4 registry should become the single
source during capability completion. Likewise, unify the planning approval queue
while adding the missing C2 workflows, rather than adding more parallel queues.

Runtime durability and restore reconciliation address failures in current use,
so they remain. Immutable evidence, erasure, source ownership, explicit consent,
bounded reads and uncertainty are product requirements. External source/import
formats are capabilities, independently of ZeitBoard release history.

## Acceptance

Verify current-schema creation/reopen and live sync, malformed-response refusal,
revision handling, erasure and the existing task/sleep batch regressions. Contract
tests validate only maintained examples. No compatibility matrix or prior-release
migration run is required for a release that has not existed. Current backup,
restore, installer, real-device and operational gates remain in the completion plan.
