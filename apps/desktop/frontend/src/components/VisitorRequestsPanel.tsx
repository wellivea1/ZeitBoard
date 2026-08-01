// Requests that arrived through a share link (ADR-0030).
//
// This is a separate surface from synced assistant proposals on purpose.
// Approving one is not a yes/no: the visitor asked for a window and the owner
// answers with an exact block inside it, which the backend re-checks. The
// generic proposal controls cannot express that, so they are not reused here.

import { useCallback, useEffect, useState } from "react";
import {
  decideVisitorRequest,
  defaultSlot,
  loadVisitorRequests,
  type VisitorRequest,
  type VisitorRequestsData,
} from "../data/visitorRequests";

function VisitorRequestCard({
  request,
  busy,
  onDecide,
}: {
  request: VisitorRequest;
  busy: boolean;
  onDecide: (
    request: VisitorRequest,
    decision: "approved" | "rejected",
    slot: { start: string; end: string },
  ) => void;
}) {
  const initial = defaultSlot(request);
  const [start, setStart] = useState(initial.start);
  const [end, setEnd] = useState(initial.end);
  const startId = `visitor-start-${request.proposalId}`;
  const endId = `visitor-end-${request.proposalId}`;

  return (
    <article className="proposal-card" data-origin="visitor">
      <div className="proposal-header">
        <span className="proposal-kind">Request</span>
        <div>
          <p className="section-kicker">{request.linkLabel}</p>
          <h3>
            {request.handle ? `${request.handle} asked for a time` : "Someone asked for a time"}
          </h3>
        </div>
      </div>

      <p className="proposal-change">
        <strong>{request.windowLabel}</strong>
        {request.durationLabel && <small>For {request.durationLabel}.</small>}
      </p>

      {request.message && <p className="visitor-message">&ldquo;{request.message}&rdquo;</p>}

      {request.beyondHorizonNote && (
        <p className="diff-note" role="note">
          {request.beyondHorizonNote}
        </p>
      )}

      <div className="visitor-slot">
        <div className="visitor-slot-field">
          <label htmlFor={startId}>Block starts</label>
          <input
            id={startId}
            type="datetime-local"
            value={start}
            min={request.windowStartLocal}
            max={request.windowEndLocal}
            disabled={busy}
            onChange={(event) => setStart(event.target.value)}
          />
        </div>
        <div className="visitor-slot-field">
          <label htmlFor={endId}>Block ends</label>
          <input
            id={endId}
            type="datetime-local"
            value={end}
            min={request.windowStartLocal}
            max={request.windowEndLocal}
            disabled={busy}
            onChange={(event) => setEnd(event.target.value)}
          />
        </div>
      </div>

      <p className="proposal-disclosure">{request.approvalDisclosure}</p>

      <div className="proposal-actions">
        <button
          className="button primary compact"
          type="button"
          disabled={busy || !request.decisionToken}
          onClick={() => onDecide(request, "approved", { start, end })}
        >
          {busy ? "Recording..." : "Accept this block"}
        </button>
        <button
          className="button ghost compact"
          type="button"
          disabled={busy || !request.decisionToken}
          onClick={() => onDecide(request, "rejected", { start, end })}
        >
          Decline
        </button>
        <small>
          {request.createdLabel}, {request.expiresLabel}
        </small>
      </div>
    </article>
  );
}

export function VisitorRequestsPanel() {
  const [data, setData] = useState<VisitorRequestsData>({ status: "off", requests: [] });
  const [busyId, setBusyId] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");

  const refresh = useCallback(() => {
    let current = true;
    void loadVisitorRequests().then((loaded) => {
      if (current) setData(loaded);
    });
    return () => {
      current = false;
    };
  }, []);

  useEffect(() => refresh(), [refresh]);

  // Omit, don't disable: with the portal off there is nothing here to explain.
  if (data.status === "off") return null;

  const onDecide = (
    request: VisitorRequest,
    decision: "approved" | "rejected",
    slot: { start: string; end: string },
  ) => {
    if (!request.decisionToken) return;
    setBusyId(request.proposalId);
    void decideVisitorRequest({
      proposalId: request.proposalId,
      decision,
      token: request.decisionToken,
      ...(decision === "approved" ? { startLocal: slot.start, endLocal: slot.end } : {}),
    }).then((result) => {
      setBusyId(null);
      if (result.status === "ok") {
        setData(result);
        setAnnouncement(
          decision === "approved"
            ? "Accepted. They will see the exact time you chose."
            : "Declined. They are told only that the time did not work.",
        );
        return;
      }
      setAnnouncement(result.message ?? "The decision could not be recorded.");
    });
  };

  return (
    <section className="panel visitor-requests-panel" aria-labelledby="visitor-requests-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">From your share links</p>
          <h2 id="visitor-requests-title">Time requests</h2>
        </div>
      </div>

      {data.status === "error" && (
        <p className="diff-note" role="alert">
          {data.message ?? "Could not load requests from your links."}
        </p>
      )}

      {data.status === "ok" && data.requests.length === 0 && (
        <p className="diff-note">No one has asked for a time yet.</p>
      )}

      <div className="proposal-list">
        {data.requests.map((request) => (
          <VisitorRequestCard
            key={request.proposalId}
            request={request}
            busy={busyId === request.proposalId}
            onDecide={onDecide}
          />
        ))}
      </div>

      <p role="status" aria-live="polite" className="sr-only">
        {announcement}
      </p>
    </section>
  );
}
