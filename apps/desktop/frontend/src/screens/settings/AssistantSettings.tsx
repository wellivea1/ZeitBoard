import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";
import {
  cancelChatGPTSignIn,
  chooseAssistantModel,
  connectChatGPT,
  disconnectChatGPT,
  listAssistantModels,
  loadAssistantConnection,
  openAssistantPage,
  type AssistantConnection,
  type AssistantModel,
} from "../../data/assistant";
import { useLoaded } from "../../state/useLoaded";

// The assistant on the owner's ChatGPT plan (ADR-0066), in Settings › Sync
// because the assistant runs on the server. Signing in opens ChatGPT in the
// browser; this card only ever learns where the sign-in stands.

const unreadable: AssistantConnection = {
  reachable: false,
  offered: false,
  message: "The assistant's sign-in could not be read just now.",
};

// OpenAI asks that a first connection say, once, that the plan now pays.
const welcomedKey = "zeitboard.assistant.chatgptWelcomed";

function welcomed(): boolean {
  try {
    return window.localStorage.getItem(welcomedKey) === "true";
  } catch {
    return false;
  }
}

function rememberWelcomed() {
  try {
    window.localStorage.setItem(welcomedKey, "true");
  } catch {
    // Said again next time, which is better than never.
  }
}

function failure(reason: unknown, fallback: string) {
  return reason instanceof Error ? reason.message : fallback;
}

/** Says, once, that the plan now pays: OpenAI's own words, then ZeitBoard's. */
function PlanWelcome({ onClose }: { onClose: () => void }) {
  const button = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const active = document.activeElement;
    const previous = active instanceof HTMLElement && active !== document.body ? active : null;
    button.current?.focus();
    // The button that began the sign-in is gone by now; the card it became
    // takes the focus instead.
    return () =>
      (previous?.isConnected
        ? previous
        : document.getElementById("assistant-plan-card-title")
      )?.focus();
  }, []);
  const onKey = (event: KeyboardEvent) => {
    if (event.key === "Escape") onClose();
    // The one control keeps the focus, as a modal dialog does.
    if (event.key === "Tab") event.preventDefault();
  };
  // Over the whole window, and outside the settings section's own styles.
  return createPortal(
    <div className="plan-welcome-backdrop">
      <div
        className="plan-welcome"
        role="dialog"
        aria-modal="true"
        aria-labelledby="plan-welcome-title"
        aria-describedby="plan-welcome-body"
        onKeyDown={onKey}
      >
        <h2 id="plan-welcome-title">You're using your ChatGPT plan.</h2>
        <p id="plan-welcome-body">
          The assistant's messages now count against your plan's limits. ZeitBoard sends it planning
          facts only, never your notes or records.
        </p>
        <button ref={button} className="button primary" type="button" onClick={onClose}>
          Got it
        </button>
      </div>
    </div>,
    document.body,
  );
}

/** What reaches OpenAI, said before the owner signs in. */
function WhatIsSent() {
  return (
    <p className="assistant-plan-disclosure">
      The assistant sends planning facts only: windows, durations, counts and opaque ids, and dose
      timing without names when you ask about medication. Never titles, notes, labels or sleep
      records. They reach OpenAI under your ChatGPT account and its terms. OpenAI's sign-in
      documentation does not say how long it keeps such requests; ZeitBoard asks it to keep none.
    </p>
  );
}

function ModelChoice({
  connection,
  models,
  busy,
  onChoose,
}: {
  connection: AssistantConnection;
  models: AssistantModel[] | null | undefined;
  busy: boolean;
  onChoose: (slug: string) => void;
}) {
  if (models === undefined) return <p>Reading your plan's models…</p>;
  if (models === null) {
    return (
      <p>
        {connection.model ? `The assistant asks ${connection.model}. ` : ""}Your plan's models could
        not be read just now.
      </p>
    );
  }
  // A model the server's configuration chose may not be listed by name.
  const options =
    connection.model && !models.some((model) => model.slug === connection.model)
      ? [{ slug: connection.model, displayName: connection.model }, ...models]
      : models;
  return (
    <label htmlFor="assistant-plan-model">
      Model
      <select
        id="assistant-plan-model"
        value={connection.model ?? ""}
        disabled={busy}
        onChange={(event) => onChoose(event.target.value)}
      >
        {!connection.model && <option value="">Choose a model</option>}
        {options.map((model) => (
          <option key={model.slug} value={model.slug}>
            {model.displayName}
          </option>
        ))}
      </select>
    </label>
  );
}

export function AssistantSettings({ syncKey }: { syncKey: string }) {
  const {
    data: connection,
    set,
    reload,
  } = useLoaded<AssistantConnection | null>(() => loadAssistantConnection(), {
    // Only this card changes the sign-in, and it keeps what each change
    // returns; the event is for the rail.
    key: syncKey,
    fallback: () => unreadable,
  });
  const connected = connection?.state === "connected";
  // Read only while connected. Not connected, the value is "not read", so a
  // new connection shows its models loading rather than an empty list.
  const models = useLoaded<AssistantModel[] | null | undefined>(
    () => (connected ? listAssistantModels() : Promise.resolve(undefined)),
    { key: connected ? `connected ${syncKey}` : "off", fallback: () => null },
  );
  const [waiting, setWaiting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [welcome, setWelcome] = useState(false);
  const locked = waiting || busy;

  const begin = () => {
    setError("");
    setMessage("");
  };

  const connect = () => {
    if (locked) return;
    begin();
    setWaiting(true);
    void connectChatGPT().then(
      (result) => {
        setWaiting(false);
        set(result);
        if (result.state !== "connected") {
          setError("Signing in did not finish.");
          return;
        }
        setMessage(`The assistant is signed in to ${result.email ?? "your ChatGPT account"}.`);
        if (!welcomed()) setWelcome(true);
      },
      (reason: unknown) => {
        setWaiting(false);
        setError(failure(reason, "Signing in did not finish."));
        reload();
      },
    );
  };

  const run = (
    operation: () => Promise<AssistantConnection>,
    done: (result: AssistantConnection) => string,
    fallback: string,
  ) => {
    if (locked) return;
    begin();
    setBusy(true);
    void operation().then(
      (result) => {
        setBusy(false);
        set(result);
        setMessage(done(result));
      },
      (reason: unknown) => {
        setBusy(false);
        setError(failure(reason, fallback));
        reload();
      },
    );
  };

  const choose = (slug: string) => {
    const name = models.data?.find((model) => model.slug === slug)?.displayName ?? slug;
    run(
      () => chooseAssistantModel(slug),
      () => `The assistant now asks ${name}.`,
      "The model could not be chosen.",
    );
  };

  const disconnect = () =>
    run(
      disconnectChatGPT,
      (result) =>
        result.revoked === false
          ? "Disconnected. ChatGPT did not confirm it withdrew ZeitBoard's access, so remove ZeitBoard in ChatGPT's settings too."
          : "Disconnected. ZeitBoard holds no access to your ChatGPT account.",
      "Disconnecting did not finish.",
    );

  const open = (page: "usage" | "help") =>
    void openAssistantPage(page).catch((reason: unknown) =>
      setError(failure(reason, "The page could not be opened.")),
    );

  const signInButton = (label: string) =>
    waiting ? (
      <div className="assistant-plan-waiting" role="status">
        <span>Finish signing in to ChatGPT in your browser. ZeitBoard waits ten minutes.</span>
        <button
          className="button ghost compact"
          type="button"
          onClick={() => void cancelChatGPTSignIn()}
        >
          Cancel
        </button>
      </div>
    ) : (
      <button className="button primary" type="button" disabled={busy} onClick={connect}>
        {label}
      </button>
    );

  let body;
  if (connection === undefined) {
    body = <p className="settings-copy">Checking…</p>;
  } else if (connection === null) {
    body = <p className="settings-copy">Signing in to ChatGPT needs the ZeitBoard desktop app.</p>;
  } else if (!connection.reachable) {
    body = <p className="settings-copy">{connection.message}</p>;
  } else if (!connection.offered) {
    // Said by the desktop for a server too old to offer a sign-in; otherwise
    // the server's configuration does not name ChatGPT (ADR-0066).
    body = (
      <p className="settings-copy">
        {connection.message ??
          "Your server does not offer signing in to ChatGPT. To offer it, set the assistant's provider to chatgpt in the server's configuration."}
      </p>
    );
  } else if (connection.state === "connected") {
    body = (
      <section
        className="data-control-card assistant-plan-card"
        aria-labelledby="assistant-plan-card-title"
      >
        <div>
          <h3 id="assistant-plan-card-title" tabIndex={-1}>
            Your ChatGPT plan
          </h3>
          <p>
            Signed in as {connection.email ?? "your ChatGPT account"}. The assistant uses your
            plan's limits; ZeitBoard charges nothing.
          </p>
        </div>
        <ModelChoice connection={connection} models={models.data} busy={locked} onChoose={choose} />
        <div className="assistant-plan-actions">
          <button className="button secondary" type="button" onClick={() => open("usage")}>
            Manage usage
          </button>
          <button className="button ghost" type="button" disabled={locked} onClick={disconnect}>
            Disconnect
          </button>
        </div>
      </section>
    );
  } else {
    const ended = connection.state === "sign_in_ended";
    body = (
      <section
        className="data-control-card assistant-plan-card"
        aria-labelledby="assistant-plan-card-title"
      >
        <div>
          <h3 id="assistant-plan-card-title" tabIndex={-1}>
            Your ChatGPT plan
          </h3>
          <p>
            {ended
              ? `The sign-in to ${connection.email ?? "ChatGPT"} has ended. Sign in again to keep using the assistant.`
              : "Use your ChatGPT Plus or Pro plan for the assistant, with no API key."}
          </p>
          {connection.problem === "plan_not_shared" && (
            <p className="assistant-plan-problem">
              ChatGPT did not share your plan with ZeitBoard. When ChatGPT asks, allow ZeitBoard to
              use it.
            </p>
          )}
        </div>
        <WhatIsSent />
        <div className="assistant-plan-actions">
          {signInButton(ended ? "Sign in again" : "Continue with ChatGPT")}
          {ended && !waiting && (
            <button className="button ghost" type="button" disabled={locked} onClick={disconnect}>
              Disconnect
            </button>
          )}
          <button className="button ghost compact" type="button" onClick={() => open("help")}>
            Learn more
          </button>
        </div>
      </section>
    );
  }

  return (
    <section
      className="settings-section assistant-plan-panel"
      aria-labelledby="assistant-plan-title"
    >
      <div className="data-control-intro">
        <h2 id="assistant-plan-title">Assistant</h2>
        <p className="settings-copy">
          The assistant runs on your server. It can use your ChatGPT plan instead of an API key.
        </p>
      </div>
      {body}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <p className="form-status" role="status" aria-live="polite">
        {message}
      </p>
      {welcome && (
        <PlanWelcome
          onClose={() => {
            rememberWelcomed();
            setWelcome(false);
          }}
        />
      )}
    </section>
  );
}
