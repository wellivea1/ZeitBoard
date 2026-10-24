# ADR 0066: The assistant through the owner's ChatGPT plan

- Status: accepted
- Date: 2026-10-24
- Builds on [ADR-0008](0008-self-hostable-backend-byok-llm.md),
  [ADR-0010](0010-assistant-backend-byok.md) and
  [ADR-0064](0064-calendar-accounts-through-the-owners-registration.md).

## Context

The assistant runs on the owner's server and calls a model with a key the operator configures
(ADR-0010). An owner with a ChatGPT Plus or Pro plan already pays for models, but could use them
here only through an API key, which OpenAI bills separately.

OpenAI's Sign in with ChatGPT lets an app sign the owner in with their ChatGPT account and call the
Responses API on their plan. OpenAI offers the plan's usage to open-source projects and to personal
projects that run locally, and its DevKit names a self-hosted machine as a place the sign-in may
live. The DevKit is a Node library, and its sample keeps every token out of the window that shows
the app.

## Decision

1. **A second way to pay for the same assistant.** Everything ADR-0010 guards stays as it is:
   - the redacted planning view;
   - the medical refusal;
   - the strict JSON contract;
   - resolution by the scheduler;
   - one-use approval tokens and the audit.

   Only the credential changes, and the shape of the request. It names the model, gives the system
   prompt as `instructions`, sends one user message, asks OpenAI to keep nothing (`store: false`)
   and streams, as the DevKit does.
2. **The server holds the whole sign-in.** The steps:
   1. The server builds the authorization address. It carries PKCE (S256), a random state and
      nonce, and the server's host ID.
   2. The owner's computer opens that address in the browser. It catches the browser's return on
      `http://127.0.0.1:<port>/auth/callback`.
   3. The computer passes back the state, the code and the client ID the answer names.

   The code is useless without the PKCE verifier, and the verifier never leaves the server. So no
   token reaches a device, and the refresh token, which rotates on every use, has one holder. This
   replaces the review's first sketch, where the computer would trade the code itself and hand the
   refresh token over.
3. **Only where nothing else is configured.**
   - The server offers the sign-in when `assistant.provider` is unset, `disabled` or `chatgpt`.
   - A sign-in never replaces a provider the configuration names.
   - `assistant.model` sets the model a new connection starts with. Without it, the plan's first
     listed model is used. The owner can choose another of the plan's models.
4. **The DevKit's protocol, in Go, from the standard library.**
   - **Endpoints.** They come from `auth.openai.com`'s discovery document, and each must be on the
     issuer's origin.
   - **The client.** A first sign-in registers it: `dynamic_agent_client` stands in until the
     answer names the client. The client is kept before the code is exchanged, so a failed
     exchange registers no second client.
   - **Every token request** names the resource, `https://api.openai.com/v1`.
   - **The host ID** is a random `urn:uuid:`, made before the first sign-in and kept for good.
   - **Hints.** A first sign-in names ZeitBoard (`agent_name_hint`). Signing in again after a
     sign-in ended suggests the same account (`login_hint`). Signing in after a sign-in that did not
     share the plan asks for consent anew (`prompt=consent`).
   - **ID tokens** are believed when:
     - they are signed with RS256 by a key the issuer publishes;
     - they are issued by it to this client;
     - they are current, within five seconds;
     - they answer this sign-in's nonce;
     - they are for the account already kept.
   - **Refreshes:**
     - They run one at a time, and finish even when the request that asked is cancelled.
     - The rotated token is kept before anything else can fail.
     - A refreshed ID token is verified before its access token is used.
     - A refresh ChatGPT says spent ends the sign-in, but the account is kept, so signing in again
       is to that account.
   - **Disconnecting** revokes the grant, trying twice. It forgets the account but keeps the
     registration and host ID.
5. **Plan usage or nothing.** A sign-in that does not grant `chatgpt.tokens.use.direct` is revoked
   at once. Proving who the owner is gives the assistant nothing.
6. **Kept like the server's other secrets.**
   - The connection is one sealed setting. It is encrypted whole with the data key (AES-256-GCM),
     bound to its name, and overwritten by secure delete when replaced.
   - Access tokens live in memory only.
   - Connecting, choosing a model and disconnecting are audited with the device that did it. The
     server's own rotations are not.
7. **Problems the owner can act on.**
   - **What ChatGPT's codes become:**
     - usage limit
     - not answering
     - sign-in ended
     - plan not shared
     - not eligible (Plus or Pro needed)
     - not turned on for this sign-in
     - model gone
     - refused
   - **What the assistant does.** It answers at once with the reason, with no second call on a
     smaller context, and says no proposal was created.
   - **What is never passed on.** No message of ChatGPT's is shown, logged or kept. No problem
     reaches a device as a 401 or 403, which mean the device itself was refused.
8. **The device API**, each route device-authenticated:
   - `GET /v1/assistant/chatgpt` gives whether a sign-in is offered, and the state, email and model.
     It answers even when no sign-in is offered.
   - `GET …/models` lists the plan's models.
   - `POST …/sign-in` and `POST …/finish` run a sign-in.
   - `PUT …/model` chooses a model.
   - `DELETE /v1/assistant/chatgpt` disconnects.

   `/v1/status` and the MCP connector's `get_status` disclose the provider (`chatgpt`), what pays
   (`chatgpt_plan`) and the model, never the email.
9. **The portal cannot reach it.** The `chatgpt` package joins the imports the portal package may
   not make.

## Consequences

- **A plan pays for the assistant.** An owner with Plus or Pro can use it without an API key, and
  its requests count against the plan.
- **What reaches OpenAI.** It is what ADR-0010 sends, now tied to the owner's ChatGPT account
  rather than an API organisation.
  - OpenAI's Sign in with ChatGPT documentation states no retention or training terms for these
    requests. ZeitBoard asks OpenAI to store nothing and cannot check more.
  - `privacy.md` and the threat model say so.
- **An assistant message may take longer.**
  - A ChatGPT model call may take a minute.
  - The server allows an assistant message 150 seconds, enough for two calls. Before, its 30-second
    limit on responses cut short even an API key provider's second call.
- **Tested against a stand-in only.**
  - A synthetic OpenAI (`chatgpttest`) plays the issuer and the API in the tests.
  - A real sign-in needs the owner's own account and is one of the checks in `release-checks.md`.
- **The official mark.** OpenAI's guidelines show the ChatGPT mark on the sign-in button. The mark is
  OpenAI's asset and is not in the repository: adding it is a download for the owner to allow.

## Not done here

The owner's computer's side follows in the same slice:
- the loopback answer;
- the card in Settings › Sync;
- the one-time welcome;
- the rail's account of the plan and of each problem.
