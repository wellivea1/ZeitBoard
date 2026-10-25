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
3. **Only where the configuration asks for it.**
   - The server offers the sign-in only when `assistant.provider` is `chatgpt`. Left unset, or
     `disabled`, the assistant stays off the network: as everywhere in ZeitBoard, nothing is on
     until the operator turns it on.
   - A sign-in never replaces a provider the configuration names.
   - `assistant.model` sets the model a new connection starts with. Without it, the plan's first
     listed model is used. The owner can choose another of the plan's models.

   (Changed the same day, after the automated review below. It had been offered also when the
   provider was unset or `disabled`, so an operator who had turned the assistant off would still
   have seen it offered.)
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

## The owner's computer

The computer's side follows the same rules: it relays and shows, and holds nothing.

- **The browser's return.** A loopback receiver (`internal/loopback`) listens on `127.0.0.1` alone,
  at `/auth/callback`, for this sign-in only.
  - It takes a GET whose Host header names that address exactly, which turns DNS rebinding away.
  - The state must be the sign-in's, given once and compared in constant time. No other parameter
    may come twice.
  - The first answer ends the wait. Anything else is answered and ignored, so a stray request cannot
    spoil the sign-in.
  - The page it answers with keeps nothing: no caching, no referrer, and a policy that runs no
    script.
- **Bindings.**
  - `ConnectChatGPT` asks the server for the address and opens only an `https` one. It waits ten
    minutes for the browser, or until cancelled; one sign-in waits at a time. Then it hands the
    server the state, the code and the client ID, and nothing else.
  - `GetAssistantConnection`, `ListAssistantModels`, `ChooseAssistantModel` and
    `DisconnectChatGPT` relay the server's answers.
  - `OpenAssistantPage` opens ChatGPT's usage settings or OpenAI's help, from a list the desktop
    holds. A server never chooses an address the window opens.
- **Settings › Sync › Assistant.** A card states where the sign-in stands:
  - nothing to sign in to;
  - a provider the server's configuration names;
  - not connected, with what is sent said before signing in;
  - waiting for the browser;
  - connected, with the account, the model, Manage usage and Disconnect;
  - ended, with Sign in again.
- **The welcome.** After the first connection, the window says once "You're using your ChatGPT
  plan." with "Got it", as OpenAI's guidelines ask.
- **The rail.**
  - The header says "ChatGPT plan", or what the sign-in needs.
  - The composer carries "Using your ChatGPT plan · Manage usage".
  - A reply that says why the assistant could not answer shows what to do beside it.
- **Waiting long enough.** An assistant message waits 160 seconds, past the server's 150. Before,
  the desktop's 10-second limit gave up on any model slower than that.
- **A desktop and server from either side of this change.** A desktop from before it reads the
  server's new status fields as invalid, so the two are updated together, as they are installed.

What is left of OpenAI's guidelines:
- **"Buy app credits".** ZeitBoard sells none.
- **An invitation banner.** Home stays free of prompts; the Settings card is the invitation.
- **The ChatGPT mark on the button.** It is OpenAI's asset to download, which the owner has not
  yet allowed.

## Update, 2026-10-24: an automated review

An automated reviewer read the server's half at medium effort and raised ten findings. Each is
fixed, with a test where it changes behaviour:

- **A context too long for the model.** It read as a refusal, so the assistant never asked again
  with less, as it does for every other provider. `context_length_exceeded`, an answer cut short and
  an answer too long are now `too_large`. The provider reports that as a context too large, and the
  assistant asks once more with a compacted context.
- **The plan withdrawn on a refresh.** The token that refresh had just rotated stayed live at
  OpenAI. It is now withdrawn, and the next sign-in asks ChatGPT for consent anew. So does a sign-in
  ended because the plan is no longer shared, wherever ChatGPT says so.
- **A refusal inside the stream.** An `error` or `response.failed` event that ends the sign-in did
  not end it, as the same code in a status does. It ends it now.
- **A refresh that keeps its token.** RFC 6749 section 6 lets an issuer keep the refresh token and
  leave it out of its answer. That ended the sign-in. Now the token in force carries on.
- **The server's response limit.** Finishing a sign-in can ask OpenAI four things in turn, each
  allowed fifteen seconds, and it could outlast the server's 30-second limit on a response. The
  sign-in routes now have two minutes, and the desktop waits 130 seconds.
- **Offered on a server set to `disabled`.** It is offered only when `assistant.provider` is
  `chatgpt` now (decision 3). A model left in the configuration from another provider can then no
  longer become the plan's.
- **Code quality.** The bounded reading of an answer, written three times, is one function now. A
  field the sign-in kept and never read is gone.

The same reviewer then read the computer's half. Its fixes:
- Once the browser has answered, Cancel no longer stops the server midway through the code's
  exchange.
- The browser tab says only that ZeitBoard has the answer, since the window says how the sign-in
  went.
- No answer of the server's reaches the owner as it came.
- The rail keeps what a sign-in still needs through a reply.
- The card claims no sign-in that did not connect.
- No example prompt is offered before signing in.
- A store that cannot be read is not taken for sync turned off.

The completion plan lists all ten findings.

## Update, 2026-10-25: a real sign-in

The owner signed in with their own ChatGPT account (`release-checks.md` section 6). OpenAI's side
behaved as this record assumed:
- its discovery document met every check;
- the plan listed eight models, and three of them, chosen in turn, each answered through the
  streamed Responses API;
- each server restart renewed the sign-in from the kept refresh token;
- disconnecting withdrew the token, and OpenAI confirmed it.

So nothing in the decision changes. The real model did find three faults in the assistant, which
every provider shares and no stand-in had shown:
- **A target with an answer.** A target sent with an answer is dropped, rather than the whole reply
  refused.
- **Freshness reasons.** The model is told what each one means, so an empty history is not called
  old records.
- **A proposal the server cannot make.** It is explained in the server's words, since the model
  wrote its answer as if the proposal were made.

`verification.md` records the run. The owner saw the use on ChatGPT's usage page. ChatGPT's own
list of connected apps after disconnecting waits on the owner's look.
