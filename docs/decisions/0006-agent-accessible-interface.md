# ADR 0006: Agent-accessible interface (the non-visual modality)

- Status: accepted
- Date: 2026-06-17

## Context

ZeitBoard is **visual-first** for its primary audience, people with *sighted* Non-24,
and [ADR-0005] / the accessibility docs commit to never sacrificing the visuals for
accessibility. But blindness is a leading *cause* of Non-24, so a strong non-visual
experience matters. Transcribing inherently visual charts (actogram, drift) into
screen-reader tables is a serviceable fallback, not a great experience.

A better non-visual experience is a **conversational agent with live voice**: "when am
I likely awake tomorrow? move my tax block to after I wake," spoken back and queued.
ZeitBoard already specifies a local-first in-app assistant ([`ui-ux-feature-specs.md`
§4](../ui-ux-feature-specs.md)) whose model **mutates nothing** — it emits an allowlisted
action that the *server* resolves into a `change-proposal` that a human approves. That
same action registry is exactly what an external agent needs.

## Decision

Treat **an agent + live voice as the primary non-visual modality**, served by a
**local, contract-typed capability layer** that exposes every feature to an agent
non-visually — structured, speakable readable state plus allowlisted *propose-only*
actions — reusing the §4.6 assistant action registry, approval gate, and redaction.

Adopt the design principle: **every feature must be operable by an agent non-visually**
(read + act) through that gated, allowlisted, redacted interface. A feature is not
"done" non-visually because it has a screen-reader table; it is done when an agent can
perceive its state and perform its actions.

Deliver the agent surface via either or both of:

- **(a) A local MCP server** *(leading option)* — ZeitBoard exposes allowlisted MCP
  tools (read projections; propose move/place/reminder; log sleep; request a backtest
  summary; etc.). An MCP-capable client (e.g. Claude Desktop) on the same machine
  connects; the client provides voice. The connector is local; whether health data
  leaves the device depends on the *agent/model* the client uses.
- **(b) A skill / integration for Claude or ChatGPT** — a packaged wrapper over the
  same capability layer for a cloud assistant.

Build the **capability layer + local MCP server first**; cloud skills are the additive,
opt-in tier.

## Invariants (carried from ADR-0003 and §4.4/§4.6)

- **Propose-only.** The agent has no more authority than the visual UI: every mutation
  becomes a pending `change-proposal` in the approval queue; nothing auto-applies.
  Approval consumes the same one-use signed token, audited identically.
- **Default-deny projection.** The agent reads only allowlisted projection DTOs (the
  same speakable, civil-time-primary, uncertainty-visible summaries the UI shows) —
  never the raw private domain model, medication names, diagnosis, raw activity, or
  calendar text beyond what a request needs.
- **Local is default; cloud is gated.** A local/offline agent path keeps health data on
  device. A cloud agent (Claude/ChatGPT cloud, or any connected backend behind an MCP
  client) is **opt-in, off by default**, sends only minimal non-identifying context,
  and requires its own privacy review + threat-model update — the same bar as the relay
  and the connected assistant backend ([`privacy.md`](../privacy.md),
  [`threat-model.md`](../threat-model.md)). The active backend is always disclosed.
- **Medical refusal holds** regardless of agent: no diagnosis, dosing, or treatment
  timing.
- **ZeitBoard ships no speech stack.** TTS/STT is the agent client's job; ZeitBoard's
  responsibility is to be perfectly drivable and to return concise, speakable results.

## Consequences

- Blind users get a *native* non-visual experience (voice + agent) **without the visual
  UI being compromised** — the clean resolution of the visual-first vs. accessibility
  tension. The screen-reader tables remain as a secondary fallback.
- The visual UI and the agent interface become **peer clients over one capability
  layer**; the Wails bindings already trending this way (`GetOverview`, `GetRhythm`,
  `GetProposals`) are the seed of that layer.
- Every new feature must define its **agent-facing read/action surface and its
  redaction** as part of the feature, mirroring the contracts-first discipline. This is
  a standing design constraint, not a later pass.
- A cloud agent is a transport of health data off-device and therefore inherits the
  full relay-grade privacy/threat-model review; the MCP connector must be usable with a
  purely local agent so the privacy-preserving path exists.
- No vendor lock-in: the capability layer is neutral; MCP and per-vendor skills are
  adapters.

## Status / next steps

Accepted. The local MCP connector is implemented in
[ADR-0012](0012-mcp-agent-connector.md): it exposes M3 read projections and M2
propose-only actions through a stateless local adapter. Cloud skill packaging remains a
future opt-in layer and requires its own privacy/threat review before shipment.
