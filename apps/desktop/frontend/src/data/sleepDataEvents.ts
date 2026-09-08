export const sleepDataChangedEvent = "zeitboard:sleep-data-changed";

export function notifySleepDataChanged() {
  window.dispatchEvent(new Event(sleepDataChangedEvent));
}

// One bridge in the app root refreshes every existing evidence-dependent view.
// The event carries no health payload; each reader applies its own projection.
export function subscribeAnalysisUpdates(): () => void {
  const runtime = (
    globalThis as {
      runtime?: { EventsOn?: (name: string, callback: () => void) => unknown };
    }
  ).runtime;
  const dispose = runtime?.EventsOn?.("zeitboard:analysis-updated", notifySleepDataChanged);
  return () => {
    if (typeof dispose === "function") dispose();
  };
}
