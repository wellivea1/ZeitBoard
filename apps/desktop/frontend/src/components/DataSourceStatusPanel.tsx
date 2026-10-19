import type { ReactNode } from "react";
import {
  activityState,
  activityStateLabels,
  type ActivityCollection,
  type ActivityState,
} from "../data/activityCollection";
import type { BackendSyncStatus } from "../data/backendSync";
import type { CalendarSource } from "../data/calendar";
import type { SleepSourceSummary, SleepSources } from "../data/sleepEntries";
import { sourceStatusWording, type SourceStatus } from "../data/sourceStatus";
import { Icon } from "./Icon";
import { momentInSentence } from "../utils/relativeTime";

type SourceRowProps = {
  detail: ReactNode;
  icon: "calendar" | "clock" | "sources";
  name: string;
  state?: "available" | "error" | "off";
  status: string;
};

function SourceRow({ detail, icon, name, state = "off", status }: SourceRowProps) {
  return (
    <article className="source-ledger-row">
      <Icon name={icon} />
      <div>
        <h3>{name}</h3>
        <p>{detail}</p>
      </div>
      <span className="source-ledger-state" data-state={state}>
        {status}
      </span>
    </article>
  );
}

function sleepDetail(summary: SleepSourceSummary) {
  const changed = [
    summary.corrected > 0 ? `${summary.corrected} corrected` : "",
    summary.suppressed > 0 ? `${summary.suppressed} excluded` : "",
  ].filter(Boolean);
  return [
    `${summary.total} ${summary.total === 1 ? "record" : "records"}`,
    summary.provenance,
    ...changed,
  ].join(" · ");
}

const syncStates: Record<
  BackendSyncStatus["status"],
  { label: string; state: SourceRowProps["state"] }
> = {
  connected: { label: "Connected", state: "available" },
  error: { label: "Error", state: "error" },
  off: { label: "Off", state: "off" },
};

function SyncRow({ status }: { status: BackendSyncStatus }) {
  const lastSync = status.lastSyncAt
    ? `last synced ${momentInSentence(new Date(status.lastSyncAt), new Date())}`
    : "not synced yet";
  const shown = syncStates[status.status] ?? syncStates.off;
  return (
    <SourceRow
      icon="sources"
      name="Server sync"
      detail={
        status.enabled ? (
          `Your own server · ${status.pushedCount} sent, ${status.pulledCount} received · ${lastSync}`
        ) : (
          <>
            Everything stays on this device. <a href="#/settings/sync">Set up sync</a>
          </>
        )
      }
      status={shown.label}
      state={shown.state}
    />
  );
}

const activityRowStates: Record<ActivityState, SourceRowProps["state"]> = {
  unavailable: "off",
  off: "off",
  stopped: "error",
  running: "available",
};

// What this computer records of itself, in the state Settings › This computer
// shows. The row once read "Not connected" whatever the setting was.
function ActivityRow({ status }: { status: ActivityCollection }) {
  const state = activityState(status);
  const settings = <a href="#/settings/computer">Settings</a>;
  const records = `${status.recordCount} ${status.recordCount === 1 ? "record" : "records"}`;
  const details: Record<ActivityState, ReactNode> = {
    unavailable: "This system cannot record it.",
    off: <>Off unless you turn it on in {settings}.</>,
    stopped: (
      <>
        {status.lastError || "It has stopped."} {records} kept. Retry in {settings}.
      </>
    ),
    running: `${records} of when this computer was active, idle or locked${
      status.powerLog.enabled ? ", and sleep and wake read from Windows" : ""
    }`,
  };
  return (
    <SourceRow
      icon="sources"
      name="Desktop activity"
      detail={details[state]}
      status={activityStateLabels[state]}
      state={activityRowStates[state]}
    />
  );
}

// The nights more than one record describes are listed where the estimator's
// view of the sources is: Rhythm › Sources.
function OverlapNote({ count }: { count: number }) {
  if (count === 0) return null;
  return (
    <p className="diff-note">
      {count === 1
        ? "1 night has more than one record."
        : `${count} nights have more than one record.`}{" "}
      <a href="#/rhythm/sources">See how the estimator combines them</a>
    </p>
  );
}

export function DataSourceStatusPanel({
  sleepSources,
  sourceStatuses = [],
  syncStatus,
  calendarSources,
  activity,
}: {
  /** Undefined while loading, like each row here: it waits rather than claiming none. */
  sleepSources?: SleepSources;
  /** What a phone found when it last checked each source (ADR-0054). */
  sourceStatuses?: SourceStatus[];
  syncStatus?: BackendSyncStatus;
  calendarSources?: CalendarSource[];
  activity?: ActivityCollection;
}) {
  const summaries = sleepSources?.sources ?? [];
  const localUnavailable = sleepSources?.status === "unavailable";
  const importedCalendars = calendarSources?.filter((source) => source.readOnly) ?? [];
  return (
    <section className="data-source-registry" aria-labelledby="source-registry-title">
      <div className="data-source-section-heading">
        <h2 id="source-registry-title">Connected</h2>
      </div>
      <div className="source-ledger">
        {!sleepSources ? null : summaries.length === 0 ? (
          <SourceRow
            icon="clock"
            name="Sleep records"
            detail={
              localUnavailable ? sleepSources.message : "None yet. Log a night or import a file."
            }
            status={localUnavailable ? "Unavailable" : "Empty"}
            state={localUnavailable ? "error" : "off"}
          />
        ) : (
          summaries.map((summary) => (
            <SourceRow
              key={`${summary.source}-${summary.provenance}`}
              icon={summary.source === "Manual sleep log" ? "clock" : "sources"}
              name={summary.source}
              detail={sleepDetail(summary)}
              status="Available"
              state="available"
            />
          ))
        )}
        {sourceStatuses.map((status) => {
          const wording = sourceStatusWording(status);
          return (
            <SourceRow
              key={status.source}
              icon="sources"
              name={wording.name}
              detail={wording.detail}
              status={wording.label}
              state={wording.state}
            />
          );
        })}
        {syncStatus && <SyncRow status={syncStatus} />}
        {calendarSources && (
          <SourceRow
            icon="calendar"
            name="Calendars"
            detail={
              importedCalendars.length > 0
                ? importedCalendars.map((source) => source.label).join(" · ")
                : "None added. Suggested times only avoid what ZeitBoard knows about."
            }
            status={importedCalendars.length > 0 ? `${importedCalendars.length} added` : "None"}
            state={importedCalendars.length > 0 ? "available" : "off"}
          />
        )}
        {activity && <ActivityRow status={activity} />}
      </div>
      <OverlapNote count={sleepSources?.overlapCount ?? 0} />
    </section>
  );
}
