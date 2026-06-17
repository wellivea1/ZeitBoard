# Threat model

> **⚠ Under revision per [ADR-0007](decisions/0007-connected-cloud-architecture.md).**
> This model assumes a local-first app where "no health payload leaves the device." That
> invariant no longer holds: the product is a connected cloud app that syncs the private
> model to the user's account and uses a cloud LLM. This document needs a rewrite to add
> accounts/auth, server-side storage of identifiable health data, cloud-provider access,
> transport, and the LLM provider's data handling as in-scope assets and surfaces.

## Scope and assets

Phase one protects local observations, corrections, estimates, schedules,
user-entered health-related data, share permissions, and trusted-view output.
Availability matters, but confidentiality and permission correctness are the
highest priorities.

## Trust boundaries

1. Platform collectors to the Go core.
2. Core to local SQLite storage.
3. Wails service to the desktop frontend.
4. Android permission APIs to Android repositories.
5. Private state to trusted-view projection.
6. Development tools and CI to synthetic fixtures.

The static trusted website is outside the private-data boundary and receives
only pre-projected synthetic data in phase one.

## Assumptions

- The operating-system account and device are not fully compromised.
- Official toolchains and pinned dependencies are obtained over authenticated
  channels.
- Users can understand visible permission and revocation controls.
- Phase one has no public relay or remote account service.

## Threats and mitigations

| Threat | Impact | Phase-one mitigation |
| --- | --- | --- |
| Over-broad platform permission | Unrelated private data becomes accessible | Request only required permissions; keep collectors disabled by default; isolate adapters |
| Sensitive logging | Payloads leak into support bundles or CI | Structured redaction policy; log categories/counts, not values or exact timestamps |
| Source mutation | Audit history and estimator support become misleading | Append-only observations and corrections; effective read model; persistence tests |
| Time-zone confusion | Incorrect drift or schedule proposals | UTC instants plus IANA zones; half-open intervals; DST-focused tests |
| Estimator overclaim | Uncertain data appears authoritative | Typed refusal, ordinal confidence, widening windows, constrained product language |
| Fixed-event mutation | User calendar intent is changed | Immutable input DTOs; proposals are separate outputs |
| Projection regression | Private fields enter a trusted view | Closed allowlisted DTO, explicit permissions, forbidden-key fixture checks, projection tests |
| Stale or revoked share | Recipient retains unintended access | Expiry and revocation checked before projection; no view on inactive profile |
| Malicious import | Resource exhaustion or malformed records | Size limits, strict schemas, bounded strings/arrays, transactional validation |
| Local database theft | Private history is disclosed | User-only file permissions, no automatic upload, documented backup sensitivity |
| Dependency or CI compromise | Build or release artifacts are altered | Pinned tool/action versions, lockfiles, minimal workflow permissions, dependency review |
| Real data in fixtures | Private data enters source control | Deterministic synthetic generator, review policy, CI fixture regeneration check |

## Residual risks

Phase one does not provide database encryption, secure deletion guarantees on
all filesystems, remote revocation after a recipient has copied a projection,
or protection from a compromised user account. The desktop webview and Android
runtime remain part of the trusted computing base.

## Security verification

- Test projection output against a forbidden-field list and all permission
  combinations.
- Test revoked and expired profiles return no view.
- Fuzz or property-test import and correction-chain parsing.
- Scan logs in integration tests for representative sensitive values.
- Review platform permissions and network calls before each release.
- Revisit this model before adding a relay, remote crash reporting, or account
  synchronization.
