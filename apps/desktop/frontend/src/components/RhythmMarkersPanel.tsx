import { useState, type FormEvent } from "react";
import {
  rhythmMarkerDeleteConfirmation,
  rhythmMarkerKindLabels,
  type RhythmMarkerInput,
  type RhythmMarkerKind,
  type RhythmMarkersData,
} from "../data/rhythmMarkers";
import { RhythmMarkerGlyph } from "./RhythmMarkerGlyph";

function localInputNow() {
  const now = new Date();
  return new Date(now.getTime() - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

function browserZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "America/New_York";
}

const markerKinds = Object.keys(rhythmMarkerKindLabels) as RhythmMarkerKind[];

export function RhythmMarkersPanel({
  data,
  busy,
  exporting,
  error,
  announcement,
  onAdd,
  onDelete,
  onExport,
}: {
  data: RhythmMarkersData;
  busy: boolean;
  exporting: boolean;
  error: string;
  announcement: string;
  onAdd: (input: RhythmMarkerInput) => Promise<void>;
  onDelete: (markerId: string, confirmation: string) => Promise<void>;
  onExport: () => void;
}) {
  const available = data.status !== "unavailable";
  const [kind, setKind] = useState<RhythmMarkerKind>("travel");
  const [startLocal, setStartLocal] = useState(localInputNow);
  const [endLocal, setEndLocal] = useState("");
  const [zoneId, setZoneId] = useState(browserZone);
  const [note, setNote] = useState("");
  const [eraseID, setEraseID] = useState("");
  const [confirmation, setConfirmation] = useState("");

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!available || busy || !startLocal || !zoneId.trim()) return;
    void onAdd({
      kind,
      startLocal,
      endLocal,
      zoneId: zoneId.trim(),
      note: note.trim(),
    }).then(
      () => {
        setStartLocal(localInputNow());
        setEndLocal("");
        setNote("");
      },
      () => undefined,
    );
  };

  const erase = (markerId: string) => {
    if (busy || confirmation !== rhythmMarkerDeleteConfirmation) return;
    void onDelete(markerId, confirmation).then(
      () => {
        setEraseID("");
        setConfirmation("");
      },
      () => undefined,
    );
  };

  return (
    <section className="rhythm-marker-workspace" aria-labelledby="rhythm-marker-title">
      <header className="rhythm-marker-heading">
        <div>
          <p className="section-kicker">Self-reported context</p>
          <h2 id="rhythm-marker-title">Rhythm markers</h2>
          <p>{data.message}</p>
        </div>
        <div className="rhythm-marker-export">
          <button
            className="button secondary"
            type="button"
            disabled={!available || exporting}
            onClick={onExport}
          >
            {exporting ? "Preparing export..." : "Export markers"}
          </button>
          <small>Owner export includes private notes.</small>
        </div>
      </header>

      <div className="rhythm-marker-boundary" role="note">
        Markers annotate context only. They do not change the estimate, establish cause, provide a
        diagnosis, or recommend treatment. A wrong marker must be erased and replaced; records are
        never edited in place.
      </div>

      <div className="rhythm-marker-layout">
        <form className="rhythm-marker-entry" onSubmit={submit}>
          <div className="rhythm-marker-section-label">
            <span>Add marker</span>
            <small>Local device only</small>
          </div>
          <label>
            <span>Context type</span>
            <select
              value={kind}
              disabled={!available || busy}
              onChange={(event) => setKind(event.target.value as RhythmMarkerKind)}
            >
              {markerKinds.map((markerKind) => (
                <option value={markerKind} key={markerKind}>
                  {rhythmMarkerKindLabels[markerKind]}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>Started</span>
            <input
              type="datetime-local"
              value={startLocal}
              disabled={!available || busy}
              onChange={(event) => setStartLocal(event.target.value)}
            />
          </label>
          <label>
            <span>Ended (optional)</span>
            <input
              type="datetime-local"
              value={endLocal}
              disabled={!available || busy}
              onChange={(event) => setEndLocal(event.target.value)}
            />
          </label>
          <label>
            <span>IANA time zone</span>
            <input
              value={zoneId}
              disabled={!available || busy}
              spellCheck="false"
              onChange={(event) => setZoneId(event.target.value)}
            />
          </label>
          <label className="rhythm-marker-note-field">
            <span>Private note (optional)</span>
            <textarea
              aria-label="Private note (optional)"
              value={note}
              maxLength={500}
              disabled={!available || busy}
              onChange={(event) => setNote(event.target.value)}
            />
            <small>{note.length}/500. Never included in trusted sharing.</small>
          </label>
          <button className="button primary" type="submit" disabled={!available || busy}>
            {busy ? "Saving..." : "Append marker"}
          </button>
        </form>

        <div className="rhythm-marker-ledger" aria-label="Recorded rhythm markers">
          <div className="rhythm-marker-ledger-head" aria-hidden="true">
            <span>Type</span>
            <span>Recorded interval</span>
            <span>Private note</span>
            <span>Action</span>
          </div>
          {data.markers.length === 0 ? (
            <div className="rhythm-marker-empty">
              <strong>{available ? "No markers recorded" : "Desktop service unavailable"}</strong>
              <span>
                {available
                  ? "The actogram remains unchanged until you append context."
                  : "This browser preview does not invent health context."}
              </span>
            </div>
          ) : (
            data.markers.map((marker) => (
              <article className="rhythm-marker-row" key={marker.markerId}>
                <div className="rhythm-marker-kind">
                  <RhythmMarkerGlyph kind={marker.kind} label={marker.kindLabel} />
                  <span>
                    <strong>{marker.kindLabel}</strong>
                    <small>User reported</small>
                  </span>
                </div>
                <div className="rhythm-marker-range">
                  <time dateTime={marker.startAt}>{marker.rangeLabel}</time>
                  <small>{marker.zoneId}</small>
                </div>
                <p>{marker.note || "No note"}</p>
                <button
                  className="rhythm-marker-erase-link"
                  type="button"
                  disabled={busy}
                  aria-expanded={eraseID === marker.markerId}
                  onClick={() => {
                    setEraseID(marker.markerId);
                    setConfirmation("");
                  }}
                >
                  Erase
                </button>
                {eraseID === marker.markerId && (
                  <div className="rhythm-marker-erase" role="group" aria-label="Permanent erasure">
                    <p>
                      This physically deletes the marker and private note. It is distinct from
                      suppressing an observation, and it cannot be undone.
                    </p>
                    <label>
                      <span>Type DELETE</span>
                      <input
                        value={confirmation}
                        autoFocus
                        disabled={busy}
                        onChange={(event) => setConfirmation(event.target.value)}
                      />
                    </label>
                    <div>
                      <button
                        className="button danger"
                        type="button"
                        disabled={busy || confirmation !== rhythmMarkerDeleteConfirmation}
                        onClick={() => erase(marker.markerId)}
                      >
                        Permanently erase
                      </button>
                      <button
                        className="button secondary"
                        type="button"
                        disabled={busy}
                        onClick={() => {
                          setEraseID("");
                          setConfirmation("");
                        }}
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                )}
              </article>
            ))
          )}
        </div>
      </div>

      <p className="form-error" role="alert">
        {error}
      </p>
      <p className="sr-only" aria-live="polite">
        {announcement}
      </p>
    </section>
  );
}
