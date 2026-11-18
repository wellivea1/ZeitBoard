import { useMemo, useSyncExternalStore } from "react";
import { findWailsMethod, type WailsRoot } from "../data/wailsBridge";

// Notes that explain how something works are useful once and noise after.
// Each can be closed with its × and is then listed in Settings › Display,
// where it can be shown again, one at a time or all together. Anything a
// decision depends on (a consent disclosure, a warning about deleting, what
// a share link reveals) is not a note and is never hidden.
//
// Every note is registered here, so Settings can name what is hidden and a
// note cannot be added without a title to list it under.
//
// The desktop keeps the list, in a settings file of its own. The window's
// storage holds a copy so the first page is drawn without the closed notes,
// and stands in when there is no desktop. Kept in the window alone, the list
// went whenever the webview's data was cleared, and every note came back.

export const noticeCatalog = {
  "home.setup": {
    title: "More you can set up",
    where: "Home",
  },
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
    title: "Your own events are never changed",
    where: "Data Sources",
  },
  "import.how": {
    title: "How importing works",
    where: "Data Sources",
  },
  "import.activity": {
    title: "Exporting from Google Takeout",
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

/** Keeps the list in this window and redraws; it does not tell the desktop. */
function remember(ids: NoticeId[]) {
  memory = ids;
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(ids));
  } catch {
    // Remembered for this session only.
  }
  window.dispatchEvent(new Event(noticesChangedEvent));
}

interface StoredNotices {
  hidden: NoticeId[];
  revision: number;
  stored: boolean;
}

function normalizeStored(value: unknown): StoredNotices | undefined {
  if (typeof value !== "object" || value === null) return undefined;
  const { hidden, revision, stored } = value as Record<string, unknown>;
  if (
    !Array.isArray(hidden) ||
    typeof revision !== "number" ||
    !Number.isSafeInteger(revision) ||
    revision < 0 ||
    typeof stored !== "boolean"
  )
    return undefined;
  // A note this build no longer has is dropped, as it is from the window's copy.
  return { hidden: hidden.filter(isNoticeId), revision, stored };
}

async function storedOnDesktop(root: WailsRoot): Promise<StoredNotices | undefined> {
  const load = findWailsMethod(root, ["GetClosedNotices"]);
  return load ? normalizeStored(await load()) : undefined;
}

/**
 * Hands the list to the desktop. The revision is read just before, so a note
 * closed before the window first heard from the desktop is not turned away as
 * out of date.
 */
async function keep(ids: NoticeId[], root: WailsRoot = globalThis as unknown as WailsRoot) {
  const save = findWailsMethod(root, ["SaveClosedNotices"]);
  if (!save) return;
  try {
    const stored = await storedOnDesktop(root);
    if (stored) await save({ baseRevision: stored.revision, hidden: ids });
  } catch {
    // Remembered in this window; the next close or reopen hands it over again.
  }
}

function write(ids: NoticeId[]) {
  remember(ids);
  void keep(ids);
}

/**
 * Takes the desktop's list as the window opens. Where the desktop has none
 * yet and this window remembers closed notes, they are handed over, so
 * nothing the owner closed comes back.
 */
export async function syncNoticesWithDesktop(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<void> {
  try {
    const stored = await storedOnDesktop(root);
    if (!stored) return;
    if (stored.stored) {
      if (stored.hidden.join(" ") !== read().join(" ")) remember(stored.hidden);
    } else if (read().length > 0) {
      await keep(read(), root);
    }
  } catch {
    // The window's own copy stands.
  }
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
