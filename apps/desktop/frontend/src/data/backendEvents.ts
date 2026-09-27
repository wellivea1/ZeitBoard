import { calendarDataChangedEvent } from "./calendar";
import { agentProposalsChangedEvent } from "./agentProposals";
import { calendarWriteBackChangedEvent } from "./calendarWriteBack";
import { medicationDataChangedEvent } from "./medications";
import { rhythmMarkersChangedEvent } from "./rhythmMarkers";
import { sleepDataChangedEvent } from "./sleepDataEvents";

// The backend announces changes it made on its own: a finished analysis, a
// sync that downloaded or erased records, something an agent proposed, or
// accepted times written to a calendar. One bridge in the app root turns each
// announcement into the window events the affected views already listen to.
// Announcements carry no health payload; each view re-reads its own projection.
const announcements: Record<string, readonly string[]> = {
  "zeitboard:analysis-updated": [sleepDataChangedEvent],
  "zeitboard:agent-proposed": [agentProposalsChangedEvent],
  "zeitboard:calendar-writes": [calendarWriteBackChangedEvent],
  "zeitboard:sync-applied": [
    sleepDataChangedEvent,
    medicationDataChangedEvent,
    rhythmMarkersChangedEvent,
    calendarDataChangedEvent,
  ],
};

export function subscribeBackendEvents(): () => void {
  const runtime = (
    globalThis as {
      runtime?: { EventsOn?: (name: string, callback: () => void) => unknown };
    }
  ).runtime;
  const disposers = Object.entries(announcements).map(([announcement, views]) =>
    runtime?.EventsOn?.(announcement, () => {
      for (const view of views) window.dispatchEvent(new Event(view));
    }),
  );
  return () => {
    for (const dispose of disposers) if (typeof dispose === "function") dispose();
  };
}
