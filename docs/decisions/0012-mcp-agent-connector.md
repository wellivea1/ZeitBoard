# ADR 0012: MCP agent connector

- Status: accepted
- Date: 2026-06-17
- Builds on [ADR-0006](0006-agent-accessible-interface.md),
  [ADR-0010](0010-assistant-backend-byok.md), and
  [ADR-0011](0011-server-side-estimation.md).

## Context

M1 gives the self-hosted backend authenticated encrypted sync. M2 gives it a
propose-only assistant action registry, scheduler resolution, pending proposals, and
one-use human approval tokens. M3 gives it authenticated read projections for overview,
rhythm, and accuracy. The missing non-visual surface is an external-agent adapter that
lets a local MCP client read those projections and create pending proposals without
creating any new mutation authority.

## Decision

Add two pieces:

- `POST /v1/proposals`, behind `requireDevice`, accepts a structured direct proposal
  action using the existing assistant action target and planning context. It calls the
  same server-side scheduler/proposal creation path as the assistant, but never calls an
  LLM provider. It returns the same pending proposal summary and one-use decision token
  shape the assistant returns.
- `cmd/zeitboard-mcp` is a stateless stdio MCP adapter over the backend HTTPS API. It
  uses a configured backend URL and device token, exposes no local database or files, and
  stores no health data.

The MCP implementation is a minimal JSON-RPC stdio adapter rather than a Go MCP SDK. The
protocol surface needed for this milestone is small (`initialize`, `tools/list`,
`tools/call`), and avoiding a new dependency keeps the self-hosted binary easier to
audit. The implementation follows MCP 2025-11-25 stdio framing: newline-delimited
JSON-RPC messages on stdin/stdout, with any diagnostics on stderr only.

## Tool List

Read tools return existing backend DTOs verbatim as MCP structured content plus JSON text:

- `get_status` -> `GET /v1/status`
- `get_overview` -> `GET /v1/overview`
- `get_rhythm` -> `GET /v1/rhythm`
- `get_accuracy` -> `GET /v1/accuracy`
- `list_proposals` -> `GET /v1/proposals`

Propose tools call `POST /v1/proposals`:

- `propose_move_task`
- `propose_place_task`
- `propose_reminder_shift`

Each propose tool requires the task target plus the request-scoped planning context
needed by the scheduler. Task and fixed-event sync remain a future data-model extension,
so M4 does not infer those server-side.

## Hard Invariant

The MCP layer is read + propose only. It exposes no `approve`, `apply`, or `decision`
tool, and it has no route to consume an approval token. Approval remains the existing
human decision endpoint, with the same signed one-use token and audit path used by the
visual UI and assistant.

## Call Budget

Each MCP process is one session with in-memory hard caps:

- total tool calls, default 20
- propose tool calls, default 5

Budgets decrement before backend calls. When exhausted, the adapter returns an MCP tool
execution error and fails closed for the rest of that budget. The budget is deliberately
local and stateless; restarting the adapter starts a new session.

## Configuration

The adapter reads:

- `ZEITBOARD_MCP_BACKEND_URL`
- `ZEITBOARD_MCP_DEVICE_TOKEN` or `ZEITBOARD_MCP_DEVICE_TOKEN_FILE`
- optional call-budget and timeout env vars

The backend URL must be HTTPS. A local self-signed TLS escape hatch exists only through
the explicit `ZEITBOARD_MCP_INSECURE_SKIP_VERIFY` setting. Missing or unreachable backend
configuration exposes no tools. Tokens are never logged or returned.

## Consequences

M4 makes the backend usable by Claude Desktop, OpenCode, or another MCP-capable local
client as the primary non-visual path. The agent can ask about current state and queue
proposal cards for a human, but cannot approve or apply anything.

## M5 Hooks

M5 should package the same read/propose capability layer as a Claude/ChatGPT cloud skill
or connector wrapper, still opt-in and still read + propose only. The reviewer-model
auto-apply gate remains future work and must not weaken the default human approval path.
Task/calendar sync should land before agents can propose from fully server-derived
planning context.
