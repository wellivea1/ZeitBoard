# ADR 0050: One registry of agent actions

- Status: accepted
- Date: 2026-09-27
- Extends [ADR-0012](0012-mcp-agent-connector.md) (agents only propose),
  [ADR-0021](0021-display-actions.md) (appearance is a direct, reversible
  exception) and [ADR-0028](0028-desktop-local-agent-endpoint.md) (the desktop's local
  agent endpoint). Code: `core/agentactions`.

## Context

An agent can ask ZeitBoard for a change in three places:

- the chat assistant's model names a `recommended_action`;
- the server's MCP connector offers propose tools;
- the desktop's local MCP endpoint offers the same propose tools and one direct action.

Each place kept its own list of the three task proposals and its own copy of the rules for the
task target. The desktop kept a fourth list to title proposal cards, and three contracts listed the
actions again. The lists agreed only because every change was made by hand in each place.

They had already drifted:

- the local endpoint accepted an empty timing bound that the server refused;
- the two MCP endpoints described the same tool in different words.

Dose logging, the next action, would have meant editing six places and three contracts.

## Decision

1. **One registry.** `core/agentactions` lists every action an agent may ask for. Each entry
   gives:
   - its id and kind: a proposal, or a direct change;
   - the surfaces that offer it;
   - the title and description a tool list shows;
   - the title of a pending proposal's card.
2. **Every surface reads it.** The registry drives:
   - the chat assistant's schema prompt and validation;
   - the server connector's tool list and propose budget;
   - the local endpoint's tool list, propose budget and dispatch;
   - the desktop's proposal cards.
3. **One target.** What a task proposal names has one Go type, one JSON Schema and one
   validation, all in the registry. The server and the local endpoint both apply it.
4. **Versioned and tested.** The registry is `v1`. The contracts still list the actions, since
   they define the wire format. A test fails when a contract's action enum differs from the
   registry.
5. **Unchanged boundaries.**
   - A proposal only creates a pending proposal.
   - No action approves, applies or decides anything, and a registry test refuses such names.
   - Direct actions are offered only on the local endpoint (`set_appearance`, ADR-0021).

## Consequences

- Adding an action takes:
  - one registry entry;
  - the contract enums the drift test names;
  - its target, if it is new;
  - its handler.
- Both MCP endpoints describe each proposal in the same words, and a test holds each tool list to
  the registry.
- The local endpoint now refuses an empty timing bound. It says which target field was wrong.
- The server's direct proposal endpoint accepts a proposal that either MCP endpoint offers. Both
  endpoints send their proposals there, and the server cannot tell them apart.
