# ADR 0028: Desktop-local agent endpoint

- Status: accepted
- Date: 2026-07-26
- Closes the deferral in [ADR-0021](0021-display-actions.md) ("agent-driven
  switching deferred until a desktop-local agent endpoint exists") and
  delivers phase-goals P4-a/P4-b. Extends [ADR-0006](0006-agent-accessible-interface.md)
  (agent-accessible interface) and preserves the
  [ADR-0012](0012-mcp-agent-connector.md) invariant: no approve/apply tool.

## Context

The MCP connector shipped in ADR-0012 runs against the *backend*. That makes
it useless for two things the "local assistant" goal requires: it cannot work
when the self-hosted backend is off, and it cannot reach device-local state —
most concretely the appearance/night-mode controls ADR-0021 defined as direct
local actions. A voice client on the user's own machine had no way to read
ZeitBoard's state or drive it without the server.

## Decision

1. **The desktop app serves its own loopback MCP endpoint.** `localagent`
   binds `127.0.0.1:0` (tcp4, ephemeral port) and speaks MCP-shaped JSON-RPC
   (`ping`, `tools/list`, `tools/call`) at `/mcp`. It starts with the app and
   stops with it; there is no separate daemon and no backend dependency.
2. **Four independent gates guard it**, because a loopback port is not by
   itself a boundary on a multi-user or malware-bearing machine:
   - loopback-only bind *and* a per-request loopback check;
   - a bearer token (≥32 chars, constant-time compare);
   - any request carrying an `Origin` header is rejected outright, so a
     browser page cannot reach the endpoint (DNS-rebinding defense);
   - the endpoint URL + token live in a descriptor file under the desktop
     config dir that is restricted to the current user - an explicit
     owner-only, inheritance-disabled DACL on Windows, mode `0600` elsewhere
     - whose shape is strictly validated on read, whose removal is
     ownership-checked, and whose creation takes an exclusive startup claim so
     a second instance cannot hijack the descriptor.

     The Windows DACL is set deliberately because `os.Chmod(0600)` does *not*
     restrict access there: it only toggles the read-only attribute and leaves
     the inherited ACL in place. A test asserts the published descriptor has a
     protected DACL containing exactly one entry, for the current user.
3. **The tool surface is allowlisted and read-heavy.** Reads: `get_status`,
   `get_overview`, `get_rhythm_summary`, `list_tasks`,
   `get_medication_timing`, `list_rhythm_markers`, `get_appearance`, and
   `ask_zeitboard_facts`. Exactly one **direct action** — `set_appearance` —
   permitted because ADR-0021 classified display settings as local,
   reversible, non-health state. Mutations are **propose-only**
   (`propose_move_task`, `propose_place_task`, `propose_reminder_shift`) and
   require an enrolled backend; they fail with an honest message when it is
   absent. **There is no approve or apply tool**, on this surface or any
   other.
4. **The medical refusal is shared code, not duplicated prose.** The refusal
   text and the prompt classifier moved to `core/agentpolicy` so chat, the
   backend MCP, and the local endpoint emit a byte-identical refusal.
   `ask_zeitboard_facts` (P4-b) answers medication *timing facts* and
   context markers from local records; ambiguous medication questions fail
   closed to the refusal, and decision-shaped language in a model answer is
   caught on the way out.
5. **Local does not mean unredacted.** Tools return the same projection DTOs
   the UI shows — never raw records — so a compromised or careless client
   cannot vacuum the store through an allowlisted read.

## Consequences

- A voice client (e.g. Claude Desktop) on the same machine can read
  ZeitBoard's state and switch appearance **with the backend off**, which is
  what "local assistant" was always supposed to mean. Scheduling requests
  still end as pending proposals a human approves.
- The appearance path is revision-versioned: agent-set and UI-set changes
  reconcile through a revision number rather than last-write-wins, so a voice
  command and a Settings click cannot silently clobber each other.
- **Residual (recorded honestly):** the endpoint runs whenever the app runs.
  It is not behind a user opt-in toggle, so its security rests entirely on
  the four gates above. The descriptor's ACL keeps *other* users out, but any
  process already running as this user can read the token and drive the
  endpoint with the user's own authority — it is a user-level boundary, not a
  sandbox. A future opt-in switch and a per-client approval prompt are the
  obvious hardening step if the threat model changes.
- The installer publishes and rolls back `zeitboard-local-mcp.exe` alongside
  the desktop binary, under the same SHA-256 verification and
  publish-transaction marker as every other artifact.
- **Known limitation, inherited not introduced: `propose_reminder_shift` does
  not shift a reminder.** The server has no reminder entity, so its resolver
  ignores the reminder id and schedules the target task; the proposal is
  labelled "Shift reminder" but carries a task placement. This predates the
  local endpoint (the action is in the ADR-0010 registry and the backend MCP
  exposes it too), and nothing auto-applies, so today the defect is a
  mislabelled pending proposal rather than a wrong mutation. Real semantics
  need a server-side reminder model - medication reminders are local-only per
  ADR-0025 - and belong to that work, not to this ADR.
- Cloud skill packaging remains out of scope and separately gated.
