# Architecture

## Scope

ZeitBoard is a connected, entirely self-hostable system (ADR-0007/0008): a
visual-first desktop app running on the user's real local sleep data, a
self-hosted Go backend (`apps/server`) that syncs that data across the user's
own devices and hosts the BYOK assistant, and a local MCP connector that makes
the same capability layer agent-operable. It demonstrates ingestion,
append-only correction, estimated sleep-wake phase, uncertain forecasts,
deterministic schedule proposals, propose-only assistant/agent actions behind
a human approval gate, and minimized trusted-view projection. It is not a
medical device and does not provide diagnosis or health recommendations.

The detailed analysis, optional-agent, validation, and UI specifications in
this directory are tracked against the implementation in
[`specification-alignment.md`](specification-alignment.md). Versioned contracts
take precedence when terminology or field shapes differ.

## Components

```mermaid
flowchart LR
  Sources["User-controlled sources"] --> Interfaces["Platform interfaces"]
  CalendarSources["Selected ICS / read-only CalDAV"] --> Desktop
  Interfaces --> Core["Go core"]
  Core --> SQLite["Local SQLite"]
  Core --> Projection["Allowlisted projection"]
  Desktop["Wails desktop service"] --> Core
  DesktopUI["React desktop UI"] --> Desktop
  Desktop <-->|"opt-in TLS sync + server projections"| Server["Self-hosted Go backend (zeitboardd)"]
  Server --> ServerStore["Encrypted append-only store"]
  Server -->|"redacted context, user's key"| LLM["BYOK LLM provider"]
  MCP["Local MCP connector (zeitboard-mcp)"] -->|"read + propose-only tools"| Server
  Agent["MCP client / agent"] --> MCP
  Android["Android repositories"] --> Contracts["Versioned contracts"]
  Fixtures["Synthetic fixtures"] --> Android
  Projection --> TrustedFixture["Pre-projected synthetic fixture"]
  TrustedFixture --> TrustedWeb["Static trusted-view prototype"]
  Core --> Contracts
  Server --> Contracts
```

### Go core

The core owns domain types, collectors' interfaces, persistence, effective
observation reads, estimation, scheduling, and sharing projection. It must not
import Wails or platform UI packages. Platform behavior is supplied through
interfaces and OS build tags.

### Desktop

The Wails service is an adapter over core use cases. It maps private domain
objects into explicit UI DTOs and does not expose database handles. The React
frontend communicates through typed adapters under `frontend/src/data`; those
adapters alone locate Wails bindings and validate unknown DTOs before screens
can use them. Backend sync is opt-in and off by default; with sync off the
desktop is purely local. Screen, component, styling, and lint boundaries are
defined in [`frontend-architecture.md`](frontend-architecture.md).

Calendar adapters are deliberately device-side (ADR-0023). Imported text is
stored and rendered only in the local trust zone. Core scheduling receives a
text-free fixed-event projection. The local SQLite ownership boundary keeps
imported snapshots immutable and app-owned approved placements separate; only
the latter are eligible for ICS export.

### Self-hosted backend and agent connector

`apps/server` ships two binaries the operator runs: `zeitboardd` (device
enrollment, TLS sync of contract-shaped records into an encrypted append-only
store, server-side estimation projections, and the BYOK propose-only assistant
with one-use approval tokens) and `zeitboard-mcp` (a stateless local adapter
exposing allowlisted read and propose-only tools to an MCP client; no tool can
approve or apply). The server may import `non24.app/core` but not Wails. See
ADR-0009 through ADR-0012 and `self-hosting.md`.

### Android

Android uses repositories whose return values conform to the shared contracts.
Fixture repositories support API 26. Real Health Connect access is isolated
behind an availability and permission boundary and requires API 28 or newer.
Android does not reimplement estimation in phase one.

### Trusted web prototype

The prototype is static and network-free. It reads only a pre-projected,
synthetic trusted-view fixture. It cannot access the private database or accept
arbitrary private-domain JSON.

## Data flow and invariants

1. Collectors append source observations with acquisition method and evidence
   status recorded independently.
2. Manual edits append correction records. Source observations are immutable.
3. An effective-read layer applies the latest valid correction chain.
4. Estimation selects recent principal sleep episodes and either emits an
   estimate with provenance, algorithm version, confidence, and uncertainty or
   returns a typed refusal.
5. Scheduling treats fixed events as immutable inputs and emits proposals with
   explanation codes. A local approval transaction revalidates the task,
   sleep-data fingerprint, and calendar fingerprint before creating an
   app-owned placement; rejection creates no event and undo cannot alter an
   imported event.
6. Sharing projects private state through an explicit permission allowlist.
   Revoked or expired profiles produce no view.

## Time semantics

- Persist instants in UTC and store the originating IANA time-zone ID where
  local interpretation is required.
- Treat intervals as half-open `[start_at, end_at)`.
- Never infer cycles by grouping records at civil midnight.
- Time-zone database updates may change future local rendering; stored instants
  remain authoritative.

## Failure behavior

Missing permissions, unsupported contract versions, invalid correction chains,
ambiguous cycle indexing, and insufficient evidence fail closed. Estimation
refusal is expected domain behavior, not an exceptional success substitute.
Logs contain event categories and opaque IDs only, never sensitive payloads or
exact behavioral timestamps.
