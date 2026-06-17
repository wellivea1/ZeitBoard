# ADR 0009: Backend sync design

- Status: accepted
- Date: 2026-06-17
- Builds on [ADR-0007](0007-connected-cloud-architecture.md) and
  [ADR-0008](0008-self-hostable-backend-byok-llm.md).

## Context

ADR-0008 makes ZeitBoard connected without making it vendor-hosted: the user runs the
backend instance, holds the transport and storage keys, and supplies any future LLM
provider credentials. Milestone 1 needs only the authenticated, encrypted device-sync
path. It must not build the BYOK LLM layer or the agent/MCP surface.

## Decision

Add a new Go module, `non24.app/server`, at `apps/server/`. The daemon binary is
`cmd/zeitboardd`. Internal packages are split by responsibility:

- `internal/config` loads operator-supplied config, secret material, and TLS settings.
- `internal/auth` generates device IDs/tokens and hashes bearer tokens.
- `internal/sync` defines the versioned sync wire model and validates payloads.
- `internal/store` owns the encrypted append-only SQLite log.
- `internal/api` exposes the HTTP API over `net/http`.

The server may import `non24.app/core` for domain validation helpers. It must not
import Wails or `non24.app/desktop`.

## Device Authentication

The server is single-user and multi-device. `POST /v1/devices` accepts an
operator-created enrollment secret and a device label, then returns a generated device
ID and bearer token. Only the token hash is stored. All sync endpoints require
`Authorization: Bearer <token>`.

Enrollment is deliberately simple for Milestone 1: the operator keeps the enrollment
secret out of source control and shares it only while adding their own devices. Token
revocation and richer device administration are later hardening work.

## Sync Protocol

All sync traffic is TLS-protected. Operators may provide certificate/key paths; a
runtime-generated localhost self-signed certificate is allowed for development.

- `POST /v1/sync/push` accepts a versioned batch of records. The client supplies
  `recordId`, `kind`, `createdAt`, and `payload`; the server assigns `seq` and
  `deviceId`. Re-pushing the same `recordId` with the same payload is a no-op.
  Reusing a `recordId` for different plaintext is rejected.
- `GET /v1/sync/pull?since=<cursor>` returns records with `seq` greater than the
  cursor, ordered by `seq`, plus the new cursor.
- Pull records use the envelope `{seq, recordId, kind, deviceId, createdAt, payload}`.
  Payloads are existing v1 observation or correction contract objects.

The log is append-only. Sync carries the user's own full observations and corrections.
Sharing to other people remains the separate default-deny trusted-view projection.

## Validation And Limits

The API enforces bounded request bodies, bounded batch length, bounded payload bytes,
strict JSON decoding, record-ID idempotency, and contract-shaped observation/correction
payload validation. Observation intervals are validated with UTC instants, IANA zones,
and half-open interval semantics matching `core/domain`.

Malformed or oversized batches are rejected before storage mutation.

## At-Rest Encryption

Each record payload is encrypted independently with AES-256-GCM using a 32-byte
operator-supplied data key. The SQLite log stores metadata needed for sync
(`seq`, `record_id`, `kind`, `device_id`, timestamps, token hashes, payload hashes,
nonces, ciphertext) but never plaintext payload bytes. The project never receives the
data key.

Backups of the data directory remain sensitive. Operators should back up the database
and key material separately and encrypt backups.

## Config Format

Milestone 1 supports environment variables and a small JSON config file:

- `ZEITBOARD_CONFIG`
- `ZEITBOARD_LISTEN_ADDR`
- `ZEITBOARD_TLS_CERT`
- `ZEITBOARD_TLS_KEY`
- `ZEITBOARD_DATA_DIR`
- `ZEITBOARD_DATA_KEY`
- `ZEITBOARD_DATA_KEY_FILE`
- `ZEITBOARD_ENROLLMENT_SECRET`
- `ZEITBOARD_ENROLLMENT_SECRET_FILE`

Environment variables override file values. Secrets are loaded only from env or files;
no committed config contains a real token, data key, or enrollment secret.

## Scope

In scope for Milestone 1: self-hosted daemon, config, TLS serving, device enrollment,
authenticated sync endpoints, encrypted append-only SQLite storage, sync contract,
tests, scripts, CI, and self-hosting docs.

Out of scope for Milestone 1: BYOK LLM providers, assistant actions, MCP, account
multi-tenancy, hosted project infrastructure, telemetry, token revocation UI, and
calendar write-back.

## Next Steps

M2 is recorded in ADR-0010 and implements the BYOK provider layer plus the propose-only
assistant backend. M3 should add the agent/MCP connector from ADR-0006 on top of the same
capability layer, with the same redaction, proposal, approval-token, and audit rules.
