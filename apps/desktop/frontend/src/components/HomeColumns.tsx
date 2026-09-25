import { MedicationQuickTaps } from "./MedicationQuickTaps";
import type { DiaryDay } from "../data/homeLead";
import type { MedicationEventInput, MedicationsData } from "../data/medications";
import { useApprovals } from "../state/approvals";
import { useApprovalQueue } from "../state/approvalQueue";
import { blockWording } from "../utils/relativeTime";

// Home's three columns, set like a printed page: what is waiting on you, what
// is in the diary, and the doses to record. Each is a list under a small-caps
// title and a rule; nothing is boxed.

function plural(count: number, one: string, many: string) {
  return `${count} ${count === 1 ? one : many}`;
}

export function NeedsYou() {
  const approvals = useApprovals();
  const { breakdown, pendingCount, ready, incomplete } = useApprovalQueue();
  const now = new Date();
  const shown = approvals.pending.slice(0, 3);
  const others = [
    breakdown.conflicts > 0 && {
      key: "conflicts",
      title: plural(
        breakdown.conflicts,
        "task changed on two devices",
        "tasks changed on two devices",
      ),
      text: "Choose which version to keep before it can be planned.",
      link: "Choose a version",
    },
    breakdown.assistant > 0 && {
      key: "assistant",
      title: plural(
        breakdown.assistant,
        "proposal from your assistant",
        "proposals from your assistant",
      ),
      text: "Nothing changes until you accept it.",
      link: "Review in Plan",
    },
    breakdown.requests > 0 && {
      key: "requests",
      title: plural(breakdown.requests, "time request", "time requests"),
      text: "From people you share a link with.",
      link: "Review in Plan",
    },
  ].filter(Boolean) as { key: string; title: string; text: string; link: string }[];
  const more = Math.max(0, approvals.pending.length - shown.length);
  const nothing = shown.length === 0 && others.length === 0;

  return (
    <section className="home-column" aria-labelledby="waiting-title">
      <h2 id="waiting-title" className="section-title">
        Waiting on you
        {pendingCount > 0 && <span className="count">{pendingCount}</span>}
      </h2>
      {shown.map((proposal) => (
        <article className="home-entry" key={proposal.id}>
          <h3>{proposal.title}</h3>
          <p>
            A suggested time,{" "}
            <strong>{blockWording(proposal.startAt, proposal.endAt, proposal.to, now)}</strong>.
          </p>
          <div className="home-entry-actions">
            <button
              className="button ghost"
              type="button"
              disabled={approvals.busyProposalId !== null}
              aria-label={`Accept the suggested time for ${proposal.title}`}
              onClick={() => approvals.decide(proposal.id, "approved")}
            >
              Accept
            </button>
            <button
              className="button ghost"
              type="button"
              data-quiet
              disabled={approvals.busyProposalId !== null}
              aria-label={`Decline the suggested time for ${proposal.title}`}
              onClick={() => approvals.decide(proposal.id, "rejected")}
            >
              Decline
            </button>
          </div>
        </article>
      ))}
      {others.map((item) => (
        <article className="home-entry" key={item.key}>
          <h3>{item.title}</h3>
          <p>{item.text}</p>
          <div className="home-entry-actions">
            <a className="button ghost" href="#/plan/tasks">
              {item.link}
            </a>
          </div>
        </article>
      ))}
      {more > 0 && (
        <a className="home-more" href="#/plan/tasks">
          {plural(more, "more suggestion", "more suggestions")} in Plan
        </a>
      )}
      {nothing && (
        <p className="home-empty">
          {!ready
            ? "Checking…"
            : incomplete
              ? "Some sources could not be checked."
              : "Nothing is waiting on you."}
        </p>
      )}
    </section>
  );
}

export function Diary({ days }: { days: DiaryDay[] }) {
  return (
    <section className="home-column" aria-labelledby="diary-title">
      <h2 id="diary-title" className="section-title">
        In the diary
      </h2>
      {days.length === 0 ? (
        <p className="home-empty">Nothing is fixed in the next three days.</p>
      ) : (
        days.map((day) => (
          <div className="diary-day" key={day.key}>
            <h3>{day.label}</h3>
            <ul>
              {day.entries.map((entry) => (
                <li key={entry.key} data-kind={entry.kind}>
                  <span className="diary-time">{entry.time}</span>
                  <span>{entry.title}</span>
                  {entry.note && <em>{entry.note}</em>}
                </li>
              ))}
            </ul>
          </div>
        ))
      )}
      <a className="home-more" href="#/plan/week">
        Open the week
      </a>
    </section>
  );
}

export function Doses({
  data,
  busy,
  available,
  onLog,
}: {
  data: MedicationsData | null;
  busy: boolean;
  available: boolean;
  onLog: (input: MedicationEventInput) => Promise<void>;
}) {
  const active = (data?.medications ?? []).filter((medication) => medication.active);
  return (
    <section className="home-column" aria-labelledby="doses-title">
      <h2 id="doses-title" className="section-title">
        Doses and notes
      </h2>
      {active.length > 0 ? (
        <MedicationQuickTaps
          medications={data?.medications ?? []}
          events={data?.events ?? []}
          available={available}
          busy={busy}
          onLog={onLog}
        />
      ) : (
        <p className="home-empty">
          No medications listed. <a href="#/log/medications">Add one in Log</a>.
        </p>
      )}
      <p className="home-note">
        <a href="#/log/markers">Note an unusual day</a>: travel, illness or an obligation that
        explains a night.
      </p>
    </section>
  );
}
