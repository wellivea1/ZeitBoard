# ADR 0004: Companion app repository strategy

- Status: accepted
- Date: 2026-06-15

## Context

The product spans three client surfaces (Wails/React desktop, native Kotlin/Compose
Android companion, static trusted-view web) over one Go core and one set of versioned
JSON Schema contracts. A question was raised whether the Android companion should live in
its own repository or remain in the combined monorepo established by
[ADR 0001](0001-monorepo-and-runtime-stack.md).

## Decision

Keep the Android companion **in the monorepo** for now.

The binding coupling between surfaces is the `contracts/v1` schemas (and shared design
tokens). Android must consume the same contract version the core emits; in a monorepo a
contract change and its Android consumer land in one atomic change, which is the dominant
need during phase one while contracts and all three surfaces co-evolve. The separation the
architecture cares about — the Go core not importing Wails, platform code behind interfaces
and build tags, Android not reimplementing estimation — is enforced by module boundaries,
not by repository boundaries.

## Why not split now

The usual reasons to split a client into its own repo do not currently apply:

- **No independent team or cadence.** A split optimizes for ownership and release
  independence that does not exist yet.
- **Toolchain coexistence is already solved.** Go modules + `go.work`, Gradle, and npm
  workspaces live together with per-stack CI jobs; Gradle does not benefit from isolation
  here.
- **Version skew is the larger risk.** Two repos would require publishing/pinning the
  contracts and cross-repo PR coordination for every schema change — premature overhead
  that also makes contract drift easier, not harder.

## Revisit triggers

Split the Android companion into its own repository when **any** of these hold:

1. A dedicated Android team forms with a release cadence independent of the desktop/core.
2. The `contracts/` directory is promoted to a **published, versioned artifact** (released
   schema package or generated client). That artifact is the natural extraction seam:
   Android then depends on a pinned contract version rather than the working tree.
3. Play Store release/signing constraints diverge materially from desktop releases.
4. Repository size or CI duration makes the combined build painful for routine work.

## Consequences

- ADR 0001 stands; no structural change is required.
- `contracts/` is treated as the future extraction seam and should remain free of
  surface-specific code so it can be published independently later.
- Android continues to consume observations and estimates through repositories that
  conform to the shared contracts.
