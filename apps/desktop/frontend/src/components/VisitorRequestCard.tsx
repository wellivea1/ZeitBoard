import { defaultSlot, type VisitorRequest } from "../data/visitorRequests";

import { useVisitorRequests } from "../state/visitorRequests";

import { useApprovalQueue } from "../state/approvalQueue";

import { reviewIsPending } from "../data/reviewQueue";

import { civilCandidates, localZone, selectedCivilInstant } from "../utils/civilTime";

export function VisitorRequestCard({ request }: { request: VisitorRequest }) {
  const queue = useVisitorRequests();

  const { now } = useApprovalQueue();

  const slot = queue.drafts[request.proposalId] ?? defaultSlot(request);

  const startAt = selectedCivilInstant(slot.start, slot.startAt);

  const endAt = selectedCivilInstant(slot.end, slot.endAt);

  const valid =
    Boolean(startAt && endAt) &&
    Date.parse(endAt) > Date.parse(startAt) &&
    Date.parse(startAt) >= Date.parse(request.windowStartAt) &&
    Date.parse(endAt) <= Date.parse(request.windowEndAt);

  const busy =
    queue.busyId !== null || queue.data.status !== "ok" || !reviewIsPending(request, now);

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

      {request.message && <p className="visitor-message">“{request.message}”</p>}

      {request.beyondHorizonNote && (
        <p className="diff-note" role="note">
          {request.beyondHorizonNote}
        </p>
      )}

      <p className="proposal-meta">Choose a block in {localZone()}.</p>

      <div className="visitor-slot">
        {(["start", "end"] as const).map((field) => {
          const id = `visitor-${field}-${request.proposalId}`;

          const candidates = civilCandidates(slot[field]);

          const selected = field === "start" ? startAt : endAt;

          return (
            <div className="visitor-slot-field" key={field}>
              <label htmlFor={id}>{field === "start" ? "Block starts" : "Block ends"}</label>

              <input
                id={id}
                type="datetime-local"
                value={slot[field]}
                disabled={busy}
                onChange={(event) =>
                  queue.setDraft(request, {
                    [field]: event.target.value,
                    [`${field}At`]: undefined,
                  })
                }
              />

              {candidates.length > 1 && (
                <>
                  <label htmlFor={`${id}-occurrence`}>
                    {field === "start" ? "Start clock occurrence" : "End clock occurrence"}
                  </label>
                  <select
                    id={`${id}-occurrence`}
                    value={selected}
                    disabled={busy}
                    onChange={(event) =>
                      queue.setDraft(request, { [`${field}At`]: event.target.value })
                    }
                  >
                    <option value="">Choose an occurrence</option>
                    {candidates.map((candidate) => (
                      <option key={candidate.instant} value={candidate.instant}>
                        {candidate.label}
                      </option>
                    ))}
                  </select>
                </>
              )}

              {slot[field] && candidates.length === 0 && (
                <small role="alert">This time does not exist in {localZone()}.</small>
              )}
            </div>
          );
        })}
      </div>

      {!valid && (
        <p className="diff-note">
          Choose an end after the start, within the requested window. Select the clock occurrence
          when a time repeats.
        </p>
      )}

      <p className="proposal-disclosure">{request.approvalDisclosure}</p>

      <div className="proposal-actions">
        <button
          className="button primary compact"
          type="button"
          disabled={busy || !valid}
          onClick={() => void queue.decide(request, "approved", slot)}
        >
          {queue.busyId === request.proposalId ? "Recording..." : "Accept this block"}
        </button>

        <button
          className="button secondary compact"
          type="button"
          disabled={busy}
          onClick={() => void queue.decide(request, "rejected", slot)}
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
