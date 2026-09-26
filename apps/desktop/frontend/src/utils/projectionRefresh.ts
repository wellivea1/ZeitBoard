// Time-based claims expire even when no new observation arrives. Refresh only
// visible views, and immediately when returning from sleep or another window.
export function subscribeProjectionRefresh(
  request: () => void,
  eventNames: string | readonly string[],
): () => void {
  const events = typeof eventNames === "string" ? [eventNames] : eventNames;
  const requestIfVisible = () => {
    if (document.visibilityState !== "hidden") request();
  };
  for (const name of events) window.addEventListener(name, request);
  window.addEventListener("focus", requestIfVisible);
  document.addEventListener("visibilitychange", requestIfVisible);
  const interval = window.setInterval(requestIfVisible, 60_000);
  request();
  return () => {
    window.clearInterval(interval);
    for (const name of events) window.removeEventListener(name, request);
    window.removeEventListener("focus", requestIfVisible);
    document.removeEventListener("visibilitychange", requestIfVisible);
  };
}
