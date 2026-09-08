// Time-based claims expire even when no new observation arrives. Refresh only
// visible views, and immediately when returning from sleep or another window.
export function subscribeProjectionRefresh(request: () => void, eventName: string): () => void {
  const requestIfVisible = () => {
    if (document.visibilityState !== "hidden") request();
  };
  window.addEventListener(eventName, request);
  window.addEventListener("focus", requestIfVisible);
  document.addEventListener("visibilitychange", requestIfVisible);
  const interval = window.setInterval(requestIfVisible, 60_000);
  request();
  return () => {
    window.clearInterval(interval);
    window.removeEventListener(eventName, request);
    window.removeEventListener("focus", requestIfVisible);
    document.removeEventListener("visibilitychange", requestIfVisible);
  };
}
