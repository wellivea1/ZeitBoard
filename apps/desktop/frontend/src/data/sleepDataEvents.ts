export const sleepDataChangedEvent = "zeitboard:sleep-data-changed";

export function notifySleepDataChanged() {
  window.dispatchEvent(new Event(sleepDataChangedEvent));
}
