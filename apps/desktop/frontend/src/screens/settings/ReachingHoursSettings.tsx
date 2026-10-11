import { Loading } from "../../components/Loading";
import { useEffect, useState } from "react";
import {
  MAX_REACHING_CONTACTS,
  loadReachingHours,
  newReachingContact,
  reachingHoursUnavailable,
  saveReachingHours,
  timeZones,
  type ReachingContact,
  type ReachingHoursEnvelope,
} from "../../data/reachingHours";
import { ReachingContactEditor } from "./ReachingContactEditor";

// The hours belong to whoever the person needs to reach, not to them. Everything
// on this panel is worded that way, because the difference is the whole point:
// a drifting rhythm makes "when is the clinic open, and will I be awake" a
// different question every day. The list is edited as a whole and saved once,
// so Discard puts back exactly what Home is using.

export function ReachingHoursSettings() {
  const [envelope, setEnvelope] = useState<ReachingHoursEnvelope>(reachingHoursUnavailable);
  const [draft, setDraft] = useState<ReachingContact[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [zones] = useState(timeZones);
  // The form waits for the saved list: editing a placeholder that the first
  // read then replaces would lose the edit.
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let current = true;
    void loadReachingHours().then((result) => {
      if (!current) return;
      setEnvelope(result);
      setDraft(result.state.contacts);
      setLoaded(true);
    });
    return () => {
      current = false;
    };
  }, []);

  const edit = (change: (contacts: ReachingContact[]) => ReachingContact[]) => {
    setDraft(change);
    setMessage("");
  };
  const replace = (contact: ReachingContact) =>
    edit((contacts) => contacts.map((known) => (known.id === contact.id ? contact : known)));
  const moveUp = (index: number) =>
    edit((contacts) => {
      const moved = [...contacts];
      const [person] = moved.splice(index, 1);
      if (person) moved.splice(index - 1, 0, person);
      return moved;
    });
  const remove = (id: string) => edit((contacts) => contacts.filter((known) => known.id !== id));
  const add = () => edit((contacts) => [...contacts, newReachingContact(zones[0] ?? "UTC")]);

  const submit = async () => {
    setBusy(true);
    setError("");
    try {
      const saved = await saveReachingHours({ contacts: draft }, envelope.revision);
      setEnvelope(saved);
      setDraft(saved.state.contacts);
      setMessage(
        saved.conflict
          ? "The list was changed elsewhere, so that version was kept and your edit was not applied."
          : "Saved. Home now uses this list.",
      );
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The list could not be saved.");
    } finally {
      setBusy(false);
    }
  };

  const full = draft.length >= MAX_REACHING_CONTACTS;

  return (
    <section className="settings-section reaching-panel">
      <div className="data-control-intro">
        <p className="section-kicker">Reaching people</p>
        <h2>When the people you need are available</h2>
        <p className="settings-copy">
          These are other people&apos;s hours, not yours &mdash; a clinic, a pharmacy, an employer,
          family in another country. Home draws a row for each person shown, marking how much of
          your predicted waking time overlaps their hours. Names stay on this computer: the
          assistant knows people only by their place in this list.
        </p>
      </div>

      {!loaded ? (
        <Loading />
      ) : (
        <div className="reaching-form">
          {draft.length === 0 ? (
            <p className="reaching-empty">
              Nobody is on the list, so Home does not say when anyone is available.
            </p>
          ) : (
            <ol className="reaching-people">
              {draft.map((contact, index) => (
                <li key={contact.id}>
                  <ReachingContactEditor
                    contact={contact}
                    position={index + 1}
                    zones={zones}
                    canMoveUp={index > 0}
                    onChange={replace}
                    onMoveUp={() => moveUp(index)}
                    onRemove={() => remove(contact.id)}
                  />
                </li>
              ))}
            </ol>
          )}

          <div className="reaching-add">
            <button
              type="button"
              className="button secondary compact"
              disabled={full}
              onClick={add}
            >
              Add someone
            </button>
            {full && (
              <small>
                Home has room for {MAX_REACHING_CONTACTS} people. Remove someone to add another.
              </small>
            )}
          </div>

          <div className="reaching-actions">
            <button
              className="button primary"
              type="button"
              disabled={busy}
              onClick={() => submit()}
            >
              Save reaching hours
            </button>
            <button
              className="button secondary"
              type="button"
              disabled={busy}
              onClick={() => {
                setDraft(envelope.state.contacts);
                setMessage("");
                setError("");
              }}
            >
              Discard changes
            </button>
          </div>

          {message && (
            <p className="settings-copy" role="status">
              {message}
            </p>
          )}
          {(error || envelope.message) && (
            <p className="form-error" role="alert">
              {error || envelope.message}
            </p>
          )}
        </div>
      )}
    </section>
  );
}
