import { useMemo, useSyncExternalStore } from "react";

// Notes that explain how something works are useful once and noise after.
// Each can be closed with its × and is then listed in Settings › Display,
// where it can be shown again, one at a time or all together. Anything a
// decision depends on (a consent disclosure, a warning about deleting, what
// a share link reveals) is not a note and is never hidden.
//
// Every note is registered here, so Settings can name what is hidden and a
// note cannot be added without a title to list it under.

export const noticeCatalog = {
  "home.estimate": {
    title: "What the forecast is",
    where: "Home",
  },
  "tasks.conflict": {
    title: "Keeping one version of a task",
    where: "Plan › Tasks",
  },
  "medications.boundary": {
    title: "Logging only",
    where: "Log › Medications",
  },
  "markers.boundary": {
    title: "What a context marker does",
    where: "Log › Context",
  },
  "rhythm.corrections": {
    title: "Corrections keep the original",
    where: "Rhythm",
  },
  "calendars.ownership": {
    title: "Your calendars are read, never changed",
    where: "Data Sources",
  },
  "import.how": {
    title: "How importing works",
    where: "Data Sources",
  },
  "sharing.how": {
    title: "How a share link works",
    where: "Sharing",
  },
} as const satisfies Record<string, { title: string; where: string }>;

export type NoticeId = keyof typeof noticeCatalog;

const storageKey = "zeitboard.notices.hidden";
export const noticesChangedEvent = "zeitboard:notices-changed";

// Used when storage is unavailable, so a closed note stays closed for the
// session even if it cannot be remembered past it.
let memory: NoticeId[] = [];

function isNoticeId(value: unknown): value is NoticeId {
  return typeof value === "string" && Object.hasOwn(noticeCatalog, value);
}

function read(): NoticeId[] {
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (raw === null) return memory;
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter(isNoticeId) : [];
  } catch {
    return memory;
  }
}

function write(ids: NoticeId[]) {
  memory = ids;
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(ids));
  } catch {
    // Remembered for this session only.
  }
  window.dispatchEvent(new Event(noticesChangedEvent));
}

export function hideNotice(id: NoticeId) {
  const hidden = read();
  if (!hidden.includes(id)) write([...hidden, id]);
}

export function showNotice(id: NoticeId) {
  write(read().filter((hiddenId) => hiddenId !== id));
}

export function showAllNotices() {
  write([]);
}

function subscribe(callback: () => void) {
  window.addEventListener(noticesChangedEvent, callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener(noticesChangedEvent, callback);
    window.removeEventListener("storage", callback);
  };
}

// A string, so an unchanged list is an unchanged snapshot.
const snapshot = () => read().join(" ");

/** The notes closed so far, in the order they were closed. */
export function useHiddenNotices(): NoticeId[] {
  const value = useSyncExternalStore(subscribe, snapshot, () => "");
  return useMemo(() => (value ? (value.split(" ") as NoticeId[]) : []), [value]);
}

export function useNotice(id: NoticeId) {
  const hidden = useHiddenNotices().includes(id);
  return { hidden, hide: () => hideNotice(id) };
}
