# Privacy

> **Architecture: connected, self-hosted, BYOK**
> ([ADR-0007](decisions/0007-connected-cloud-architecture.md) +
> [ADR-0008](decisions/0008-self-hostable-backend-byok-llm.md)). The backend is **entirely
> self-hostable** (the project operates no service and collects no telemetry); the user's
> data syncs to *their own* instance, and the assistant LLM is **bring-your-own-key**. The
> Milestone 1 sync path and Milestone 2 BYOK assistant backend are implemented; the MCP
> connector/skill layer remains future work.
> Not legal advice.

## Commitments

- **Self-hostable, user-controlled.** The backend is entirely self-hostable; the project
  runs no service and collects no telemetry. The operator of an instance controls the
  data and is its data controller.
- **Bring-your-own LLM.** The assistant uses the user's own provider key (OpenCode Zen /
  OpenRouter / OpenAI / Anthropic, modeled on OpenCode). The project ships no keys; only
  minimized, redacted context is sent, to the provider the user chose and discloses —
  that provider relationship and its terms are the user's.
- No advertising, data brokerage, or third-party tracking SDKs.
- User-controlled acquisition, correction, export, sharing, and deletion.
- Data minimization at collection, storage, logging, projection, and testing.
- Encryption in transit (TLS) and at rest on the instance; BYOK credentials in a secret
  store, never logged.
- Explicit consent and least privilege for every platform permission and data source.
- **Legal scope: US / North Carolina** — honest representations (NC UDAP) and
  breach-notification awareness (NC Identity Theft Protection Act) for instance
  operators. Compliance outside the US/NC is the user's responsibility.

## Collection defaults

Fixture mode is enabled in the sample configuration. Windows activity and
Health Connect collection are disabled until the user enables them through a
visible permission flow. The application does not collect keystrokes, typed
content, screenshots, browser history, active-window titles, clipboard data,
precise location, or unrelated files.

Activity collection, when enabled, records only the minimum coarse state needed
by the product boundary. It must not retain application names or content.

## Local storage

Private data lives in the local SQLite database and syncs to the user's
self-hosted instance. Database files, write-ahead logs, exports, and backups must
be treated as sensitive and have file permissions restricted to the owner.
**At-rest encryption (local and on the instance) and TLS in transit are required**,
not optional — the operator holds the keys.

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
only. The `tools/` fixture generator is deterministic and checks trusted-view
fixtures for forbidden private keys. Real exports must never be committed,
attached to issues, or used in CI.

## Product claims

The product presents estimated sleep-wake phase, predicted sleep and waking
windows, and confidence/uncertainty. It does not claim exact circadian phase or
DLMO and does not provide diagnosis, treatment, or behavioral recommendations.
