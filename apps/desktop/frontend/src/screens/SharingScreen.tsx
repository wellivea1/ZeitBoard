import { useEffect, useState } from "react";
import { Icon } from "../components/Icon";
import { PageHeader } from "../components/AppShell";
import { ShareLinkForm } from "../components/ShareLinkForm";
import { ShareLinkList } from "../components/ShareLinkList";
import {
  createShareLink,
  eraseShareLink,
  loadShareLinks,
  revokeShareLink,
  shareLinksUnavailable,
  type CreatedShareLink,
  type CreateShareLinkInput,
  type ShareLinksData,
} from "../data/sharing";

// Sharing talks to the owner's own server (roadmap slice 12a). Until this slice
// the screen said "link creation and recipient access are not connected in this
// build" while the instance had shipped create/list/revoke/erase months
// earlier: honest about the user's experience, wrong about the system, and the
// wrong way round of the two.

function stateFacts(data: ShareLinksData) {
  const active = data.links.filter((link) => link.state === "active").length;
  const transport =
    data.status === "ok"
      ? "Your own server"
      : data.status === "unavailable"
        ? "Portal switched off"
        : data.status === "off"
          ? "Sync is off"
          : "Unreachable";
  return [
    { label: "Policy", value: "Default deny" },
    {
      label: "Working links",
      value: data.status === "ok" ? String(active) : "Unknown",
    },
    { label: "Transport", value: transport },
  ];
}

function headline(data: ShareLinksData) {
  if (data.status !== "ok") return "No trusted view is being shared";
  const active = data.links.filter((link) => link.state === "active").length;
  if (active === 0) return "No trusted view is being shared";
  return active === 1 ? "One link is working right now" : `${active} links are working right now`;
}

export function SharingScreen() {
  const [data, setData] = useState<ShareLinksData>(shareLinksUnavailable);
  const [created, setCreated] = useState<CreatedShareLink | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let current = true;
    void loadShareLinks().then((loaded) => {
      if (current) setData(loaded);
    });
    return () => {
      current = false;
    };
  }, []);

  const submit = async (input: CreateShareLinkInput) => {
    setBusy(true);
    setError("");
    try {
      const result = await createShareLink(input);
      setData(result.links);
      if (result.status === "ok") setCreated(result);
      else setError(result.message ?? "The link could not be created.");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The link could not be created.");
    } finally {
      setBusy(false);
    }
  };

  const run = async (action: () => Promise<ShareLinksData>) => {
    setBusy(true);
    setError("");
    try {
      const result = await action();
      setData(result);
      if (result.status === "error") setError(result.message ?? "That did not work.");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "That did not work.");
    } finally {
      setBusy(false);
    }
  };

  const connected = data.status === "ok";

  return (
    <>
      <PageHeader
        title="Sharing"
        description="Choose the minimum a person needs, require an expiry, and keep revocation one click away."
        actions={
          <div className="status-cluster">
            <span
              className="sync-dot"
              data-mode={connected ? "synced" : "fixture"}
              aria-hidden="true"
            />
            <span>{connected ? "Your own server" : "Not connected"}</span>
          </div>
        }
      />

      <section className="sharing-workspace" aria-label="Share links">
        <header className="sharing-state">
          <div className="sharing-state-copy">
            <Icon name="shield" />
            <div>
              <p className="section-kicker">Current state</p>
              <h2>{headline(data)}</h2>
              <p>
                {data.status === "ok"
                  ? "Every link below is one you made. Revoking stops it immediately; erasing also removes the record that it existed."
                  : (data.message ??
                    "Sharing runs on your own server, and this desktop is not talking to one yet.")}
              </p>
            </div>
          </div>
          <dl className="sharing-state-facts">
            {stateFacts(data).map((fact) => (
              <div key={fact.label}>
                <dt>{fact.label}</dt>
                <dd>{fact.value}</dd>
              </div>
            ))}
          </dl>
        </header>

        <section className="sharing-template-section" aria-labelledby="sharing-links-title">
          <div className="sharing-section-heading">
            <div>
              <p className="section-kicker">{connected ? "Your links" : "Nothing to show"}</p>
              <h2 id="sharing-links-title">Links you have made</h2>
            </div>
            <p>Every permission starts off and must be explicitly granted by you.</p>
          </div>

          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}

          <ShareLinkList
            data={data}
            busy={busy}
            onRevoke={(profileId) => void run(() => revokeShareLink(profileId))}
            onErase={(profileId, confirmation) =>
              void run(() => eraseShareLink(profileId, confirmation))
            }
          />

          <ShareLinkForm
            data={data}
            busy={busy}
            created={created}
            onDismissCreated={() => setCreated(null)}
            onSubmit={(input) => void submit(input)}
          />
        </section>

        <aside className="sharing-guardrails" aria-labelledby="sharing-guardrails-title">
          <div>
            <p className="section-kicker">Every link, always</p>
            <h2 id="sharing-guardrails-title">Required guardrails</h2>
          </div>
          <ol>
            <li>A passcode is required; there is no open link.</li>
            <li>An expiry is required; permanent links are not offered.</li>
            <li>Revocation is immediate, and the access history stays readable.</li>
            <li>A failure renders as a contentless unavailable page.</li>
          </ol>
          <div className="sharing-private-boundary">
            <strong>Never in a trusted link</strong>
            <span>
              Medication, diagnosis, raw activity, location, private calendar text, and rhythm
              marker notes.
            </span>
          </div>
        </aside>
      </section>
    </>
  );
}
