# Contracts

Contracts are versioned JSON Schema documents. Version `v1` uses JSON Schema
Draft 2020-12 and is intentionally strict: objects reject unknown fields unless
a schema explicitly says otherwise.

## Compatibility

- Additive changes require a new schema file or a new contract version when
  existing strict consumers would reject the payload.
- Breaking field, meaning, or enum changes require a new version directory.
- Producers must emit `schema_version`; consumers must reject unsupported
  versions rather than guessing.
- UTC instants use RFC 3339 timestamps. Local interpretation uses a separate
  IANA time-zone ID in private/local contracts.
- Intervals are half-open: `[start_at, end_at)`.

`trusted-view.schema.json` is a minimized projection contract. It is not a
serialization of the private model and deliberately has no extension point for
medication, diagnosis, raw activity, provenance, location, identifiers, or
calendar text.

`overview.schema.json`, `rhythm.schema.json`, and `accuracy.schema.json` cover the
authenticated server read projections. They are also projection contracts, not raw sync
or domain-model serialization: refusals are typed, observation/source IDs are omitted,
and the rhythm chart uses presentation row IDs plus civil-time labels.
