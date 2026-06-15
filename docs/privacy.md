# Privacy

## Commitments

- Local-first operation with no analytics, telemetry, tracking SDKs, or health
  data upload by default.
- User-controlled acquisition, correction, export, sharing, and deletion.
- Data minimization at collection, storage, logging, projection, and testing.
- Explicit consent and least privilege for every platform permission.

## Collection defaults

Fixture mode is enabled in the sample configuration. Windows activity and
Health Connect collection are disabled until the user enables them through a
visible permission flow. The application does not collect keystrokes, typed
content, screenshots, browser history, active-window titles, clipboard data,
precise location, or unrelated files.

Activity collection, when enabled, records only the minimum coarse state needed
by the product boundary. It must not retain application names or content.

## Local storage

Private data stays in the configured local SQLite database. Database files,
write-ahead logs, exports, and backups must be treated as sensitive. File
permissions should be restricted to the current user. At-rest encryption is a
future enhancement and is not assumed by phase one.

Deletion removes local derived data and source records according to the user's
explicit request, subject to a clear confirmation flow. Export must be an
intentional action and should identify whether it contains private data or only
a minimized projection.

## Logging

Logs may contain component names, error categories, counts, durations, and
opaque correlation IDs. They must not contain observation payloads, notes,
health details, calendar content, tokens, exact behavioral timestamps, or raw
trusted-view URLs. Debug logging does not relax this rule.

## Sharing

Sharing is default-deny. Every field is separately allowlisted, profiles expire,
and revoked or expired profiles produce no view. Projection code constructs the
trusted DTO field by field and never serializes the private model. The phase-one
trusted website is static, synthetic, and makes no network request.

## Development data

Fixtures, tests, screenshots, demos, and static assets contain synthetic data
only. `scripts/generate-testdata.py` is deterministic and checks trusted-view
fixtures for forbidden private keys. Real exports must never be committed,
attached to issues, or used in CI.

## Product claims

The product presents estimated sleep-wake phase, predicted sleep and waking
windows, and confidence/uncertainty. It does not claim exact circadian phase or
DLMO and does not provide diagnosis, treatment, or behavioral recommendations.
