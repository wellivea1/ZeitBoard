# Contracts

Contracts are versioned JSON Schema documents. Version `v1` uses JSON Schema
Draft 2020-12 and is intentionally strict: objects reject unknown fields unless
a schema explicitly says otherwise.

## Compatibility

- ZeitBoard has no packaged releases or deployed consumers (owner direction, 2026-09-08).
  Maintain one current contract per capability and update it together with its producers,
  consumers and fixtures. Existing version names identify current shapes, not a support matrix.
- Pre-release changes can edit the current schema in place. Add an upgrade policy when the
  first release establishes a supported external baseline; do not retain obsolete prototype
  schemas, fallback routes or mixed-version export machinery.
- Producers must emit `schema_version`; consumers must reject unsupported versions rather than
  guessing.
- UTC instants use RFC 3339 timestamps. Local interpretation uses a separate IANA time-zone ID in
  private/local contracts.
- Intervals are half-open: `[start_at, end_at)`.

Generated contract examples are registered once in
`tools/internal/fixtures`: the manifest owns each checked-in testdata path,
contract version, and validating schema. Contract validation consumes that
manifest directly so adding a fixture cannot omit its schema check.

`trusted-view.schema.json` is a minimized projection contract. It is not a
serialization of the private model and deliberately has no extension point for
medication, diagnosis, raw activity, provenance, location, identifiers, or
calendar text.

`overview.schema.json`, `rhythm.schema.json`, and `accuracy.schema.json` cover the
authenticated server read projections. They are also projection contracts, not raw sync
or domain-model serialization: refusals are typed, observation/source IDs are omitted,
and the rhythm chart uses presentation row IDs plus civil-time labels.

`v2/companion.schema.json` is an authenticated private device projection with typed UTC forecast
windows, zones, confidence, refusal, source cursor and cache expiry. It is not an extension of the
trusted, MCP or provider allowlists. Its sleep row IDs are presentation hashes and cannot be used to
erase observations.

`direct-proposal-request.schema.json` and `proposal-response.schema.json` cover the M4
agent/direct proposal path. The request reuses the assistant action target shape plus a
request-scoped planning context; the response contains a pending proposal and one-use
decision token, but no approval/apply operation.

`calendar-event-set.schema.json` is a private, device-local contract. Imported
event text may be displayed locally but must never be copied into schedule requests,
trusted views, MCP payloads, telemetry, or server projections. Scheduling receives
only event identifiers and half-open UTC intervals. Imported events are read-only;
only explicitly approved `app_owned` blocks may carry task and proposal links.

The current `v2` medication schemas include explicit schedule zones, reminder opt-in and private
clinician-rule text. The unused v1 prototypes were removed. These are private, device-local
contracts. Medication labels, strength/form or
clinician-rule text, and event notes must not enter trusted views, LLM context, MCP output,
telemetry, or logs. Wake-relative and predicted-sleep timing are read-time projections and are never
stored as medication evidence.

### Private sleep review

`v1/sleep-review.schema.json` describes authenticated owner editing context from
`GET /v1/sleep/{id}/review`. It carries the source revision, source/effective windows
and bounded competing manual edits. It is never a portal or agent projection.
The current correction schema uses `based_on_source_revision` and
`supersedes_correction_ids` for explicit resolution; a later save time alone does
not imply source review. Producers and consumers use this contract together; no
prototype single-parent format or version negotiation is supported (ADR-0039/0040).
