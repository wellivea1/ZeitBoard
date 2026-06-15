# ADR 0003: Default-deny trusted sharing

- Status: accepted
- Date: 2026-06-15

## Decision

Sharing is a pure projection from private state plus an explicit permission set into a small trusted-view DTO. Unknown fields are excluded by construction. Revoked or expired profiles produce no view.

The phase-one trusted website is static, synthetic, and network-free. It receives only pre-projected fixture data. Medication, diagnosis, location, raw activity, provenance, private identifiers, and calendar text are absent from the share contract.

## Consequences

- Adding a private domain field cannot expose it automatically.
- Per-person permissions remain granular and testable.
- A future hosted relay can transport encrypted or minimized projections without becoming authoritative for private local data.

