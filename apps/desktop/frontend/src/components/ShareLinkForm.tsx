import { useState } from "react";
import type { CreatedShareLink, CreateShareLinkInput, ShareLinksData } from "../data/sharing";

// The disclosure is shown *above* the button, not after the link exists: an
// owner must not be able to create one without having seen what it reveals and
// what revocation cannot undo (portal-design exposure gate, item 7).

const emptyForm = {
  label: "",
  passcode: "",
  expiresInDays: 14,
  wakingWindows: true,
  allowRequests: false,
  allowMessages: false,
};

export function ShareLinkForm({
  data,
  busy,
  created,
  onSubmit,
  onDismissCreated,
}: {
  data: ShareLinksData;
  busy: boolean;
  created: CreatedShareLink | null;
  onSubmit: (input: CreateShareLinkInput) => void;
  onDismissCreated: () => void;
}) {
  const [form, setForm] = useState(emptyForm);
  const [copied, setCopied] = useState(false);

  if (created?.linkUrl) {
    return (
      <section className="sharing-created" aria-labelledby="sharing-created-title">
        <p className="section-kicker">Copy it now</p>
        <h3 id="sharing-created-title">Your link exists once</h3>
        <p>
          Your server keeps only a scrambled copy of this address, so nothing — not this app, not
          the server — can show it again. Copy it now and send it with the passcode by a different
          route.
        </p>
        <output className="sharing-created-url">{created.linkUrl}</output>
        {created.expiresLabel && <p className="sharing-created-expiry">{created.expiresLabel}</p>}
        <div className="sharing-created-actions">
          <button
            className="button primary compact"
            type="button"
            onClick={() => {
              void navigator.clipboard?.writeText(created.linkUrl ?? "").then(
                () => setCopied(true),
                () => setCopied(false),
              );
            }}
          >
            Copy link
          </button>
          <button
            className="button secondary compact"
            type="button"
            onClick={() => {
              setCopied(false);
              setForm(emptyForm);
              onDismissCreated();
            }}
          >
            Done
          </button>
          {copied && <span role="status">Copied.</span>}
        </div>
      </section>
    );
  }

  if (data.status !== "ok") return null;

  const submit = () => {
    onSubmit({
      label: form.label,
      passcode: form.passcode,
      expiresInDays: form.expiresInDays,
      grants: {
        wakingWindows: form.wakingWindows,
        allowRequests: form.allowRequests,
        allowMessages: form.allowMessages,
      },
    });
  };

  return (
    <form
      className="sharing-create"
      aria-labelledby="sharing-create-title"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <div>
        <p className="section-kicker">New link</p>
        <h3 id="sharing-create-title">Share when you are awake</h3>
      </div>

      {data.disclosure && (
        <p className="sharing-disclosure" role="note">
          {data.disclosure}
        </p>
      )}

      <label>
        <span>Name (stays on this device)</span>
        <input
          value={form.label}
          onChange={(event) => setForm({ ...form, label: event.target.value })}
          maxLength={80}
          required
        />
      </label>

      <label>
        <span>Passcode (at least {data.minPasscodeLength} characters, required)</span>
        <input
          type="password"
          value={form.passcode}
          onChange={(event) => setForm({ ...form, passcode: event.target.value })}
          minLength={data.minPasscodeLength}
          autoComplete="new-password"
          required
        />
      </label>

      <label>
        <span>Expires after (days, at most {data.maxDays})</span>
        <input
          type="number"
          min={1}
          max={data.maxDays}
          value={form.expiresInDays}
          onChange={(event) => setForm({ ...form, expiresInDays: Number(event.target.value) || 0 })}
          required
        />
      </label>

      <fieldset className="sharing-grants">
        <legend>What this link may do</legend>
        <label>
          <input
            type="checkbox"
            checked={form.wakingWindows}
            onChange={(event) => setForm({ ...form, wakingWindows: event.target.checked })}
          />
          <span>See when you are likely awake</span>
        </label>
        <label>
          <input
            type="checkbox"
            checked={form.allowRequests}
            onChange={(event) => setForm({ ...form, allowRequests: event.target.checked })}
          />
          <span>Ask you for a time (you decide each one)</span>
        </label>
        <label>
          <input
            type="checkbox"
            checked={form.allowMessages}
            onChange={(event) => setForm({ ...form, allowMessages: event.target.checked })}
          />
          {/* Threads are P5-c. The grant is stored so an existing link does not
              need re-issuing later, and the label says so rather than implying
              a feature that is not there. */}
          <span>Send a short message (not delivered yet)</span>
        </label>
      </fieldset>

      <button className="button primary" type="submit" disabled={busy}>
        Create link
      </button>
    </form>
  );
}
