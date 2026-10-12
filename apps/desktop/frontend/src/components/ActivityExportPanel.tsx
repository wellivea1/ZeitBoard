import { useState } from "react";
import {
  accountActivityChangedEvent,
  canImportActivityExport,
  importActivityExport,
  loadAccountActivity,
  noAccountActivity,
  notifyAccountActivityChanged,
  previewActivityExport,
  removeAccountActivity,
  type AccountActivity,
  type ActivityExportPreview,
} from "../data/accountActivity";
import { useLoaded } from "../state/useLoaded";
import { appLocale } from "../utils/appLocale";
import { ConfirmDelete } from "./ConfirmDelete";
import { Notice } from "./Notice";

// Data Sources: bring in when a Google account was used, from a Takeout export
// the person chose (ADR-0059). What is read is said before anything is chosen,
// what was found is shown before anything is kept, and one action removes it.

const removeWord = "REMOVE";

function amount(value: number) {
  return value.toLocaleString(appLocale);
}

function PreviewCounts({ preview }: { preview: ActivityExportPreview }) {
  return (
    <dl className="sleep-import-counts" aria-label="What the export holds">
      <div>
        <dt>Entries read</dt>
        <dd>{amount(preview.entries)}</dd>
      </div>
      <div>
        <dt>Minutes with activity</dt>
        <dd>{amount(preview.minutes)}</dd>
      </div>
      <div>
        <dt>Nights suggested</dt>
        <dd>{amount(preview.nights)}</dd>
      </div>
    </dl>
  );
}

function previewSentence(preview: ActivityExportPreview) {
  const unusable =
    preview.unusable > 0
      ? ` ${amount(preview.unusable)} ${preview.unusable === 1 ? "entry has" : "entries have"} no time that can be used and ${preview.unusable === 1 ? "is" : "are"} left out.`
      : "";
  return `${preview.fileName} covers ${preview.spanLabel}.${unusable} Nothing is kept until you keep it.`;
}

function KeptActivity({
  activity,
  busy,
  onRemove,
}: {
  activity: AccountActivity;
  busy: boolean;
  onRemove: () => void;
}) {
  return (
    <div className="activity-export-kept">
      <p>
        <span className="count">{amount(activity.minutes)}</span> minutes with activity, from{" "}
        {activity.figure?.spanLabel}. Last imported {activity.importedLabel}.
      </p>
      <p className="activity-export-actions">
        <a href="#/rhythm/sources">See the nights they suggest</a>
        <button className="button ghost" type="button" disabled={busy} onClick={onRemove}>
          Remove imported activity
        </button>
      </p>
    </div>
  );
}

export function ActivityExportPanel() {
  const available = canImportActivityExport();
  const loaded = useLoaded(loadAccountActivity, {
    events: [accountActivityChangedEvent],
    fallback: () => noAccountActivity,
  });
  const activity = loaded.data;
  const [preview, setPreview] = useState<ActivityExportPreview | null>(null);
  const [busy, setBusy] = useState<"reading" | "keeping" | "removing" | null>(null);
  const [removing, setRemoving] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");

  const run = async (
    phase: "reading" | "keeping" | "removing",
    action: () => Promise<void>,
    failure: string,
  ) => {
    setBusy(phase);
    setError("");
    setStatus("");
    try {
      await action();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : failure);
    } finally {
      setBusy(null);
    }
  };

  const choose = () =>
    run(
      "reading",
      async () => {
        setPreview(null);
        const read = await previewActivityExport();
        if (read) setPreview(read);
      },
      "The export could not be read.",
    );

  const keep = (token: string) =>
    run(
      "keeping",
      async () => {
        loaded.set(await importActivityExport(token));
        setPreview(null);
        setStatus("Kept. Only the times were saved.");
        notifyAccountActivityChanged();
      },
      "The activity could not be kept.",
    );

  const remove = () =>
    run(
      "removing",
      async () => {
        loaded.set(await removeAccountActivity(removeWord));
        setRemoving(false);
        setStatus("The imported activity was removed.");
        notifyAccountActivityChanged();
      },
      "The imported activity could not be removed.",
    );

  return (
    <section
      className="sleep-import-panel activity-export-panel"
      aria-labelledby="activity-export-title"
    >
      <div className="data-source-section-heading">
        <h2 id="activity-export-title">Import Google activity</h2>
      </div>
      <div className="activity-export-disclosure">
        <p>
          From a Google Takeout export, ZeitBoard keeps only when each activity happened, to the
          minute. What you searched, watched or visited is passed over as the file is read and is
          never stored.
        </p>
        <p>
          The times stay on this computer. They are not synced or shared, and no assistant sees
          them. Long gaps in them are shown as suggested nights beside the ones you record, and
          change nothing else.
        </p>
      </div>
      <Notice id="import.activity">
        <p>
          In Google Takeout, select My Activity, open its formats and choose JSON. Then choose the
          .zip Takeout sends you here, or a MyActivity.json file from inside it.
        </p>
      </Notice>
      {activity?.imported && !removing && (
        <KeptActivity activity={activity} busy={busy !== null} onRemove={() => setRemoving(true)} />
      )}
      {removing && (
        <ConfirmDelete
          question="Remove the imported Google activity?"
          action="Remove activity"
          busyAction="Removing…"
          word={removeWord}
          busy={busy === "removing"}
          onConfirm={() => void remove()}
          onCancel={() => setRemoving(false)}
        >
          <p>
            Every imported time is erased from ZeitBoard, with the nights they suggest. Your export
            and your Google account are not changed.
          </p>
        </ConfirmDelete>
      )}
      {preview && (
        <div className="sleep-import-results">
          <PreviewCounts preview={preview} />
          <p className="form-status">{previewSentence(preview)}</p>
        </div>
      )}
      {!removing && (
        <div className="sleep-import-controls">
          {preview ? (
            <>
              <button
                className="button primary"
                type="button"
                disabled={busy !== null}
                onClick={() => void keep(preview.token)}
              >
                {busy === "keeping" ? "Keeping…" : "Keep these times"}
              </button>
              <button
                className="button ghost"
                type="button"
                disabled={busy !== null}
                onClick={() => setPreview(null)}
              >
                Not now
              </button>
            </>
          ) : (
            <button
              className="button secondary"
              type="button"
              disabled={!available || busy !== null}
              onClick={() => void choose()}
            >
              {busy === "reading"
                ? "Reading the export…"
                : activity?.imported
                  ? "Import another export"
                  : "Choose an export"}
            </button>
          )}
        </div>
      )}
      {!available && (
        <p className="form-status">Opening an export needs the ZeitBoard desktop app.</p>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {status && (
        <p className="form-status" role="status">
          {status}
        </p>
      )}
    </section>
  );
}
