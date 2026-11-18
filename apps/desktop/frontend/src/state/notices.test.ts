import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { hideNotice, showAllNotices, syncNoticesWithDesktop } from "./notices";
import type { WailsRoot } from "../data/wailsBridge";

const storageKey = "zeitboard.notices.hidden";

function desktop(stored: { hidden: string[]; revision: number; stored: boolean }) {
  const state = { ...stored };
  const SaveClosedNotices = vi.fn(async (input: { baseRevision: number; hidden: string[] }) => {
    if (input.baseRevision === state.revision) {
      state.hidden = input.hidden;
      state.revision += 1;
      state.stored = true;
    }
    return { ...state };
  });
  const GetClosedNotices = vi.fn(async () => ({ ...state }));
  const root = { go: { main: { App: { GetClosedNotices, SaveClosedNotices } } } };
  return { root: root as unknown as WailsRoot, state, SaveClosedNotices };
}

function remembered(): string[] {
  return JSON.parse(window.localStorage.getItem(storageKey) ?? "[]") as string[];
}

describe("closed notes", () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(() => {
    Reflect.deleteProperty(globalThis, "go");
    window.localStorage.clear();
  });

  // The desktop's list is the one that lasts. A window whose storage was
  // cleared gets its closed notes back from it, where they used to return.
  it("takes the desktop's list as the window opens", async () => {
    const { root, SaveClosedNotices } = desktop({
      hidden: ["home.setup", "sharing.how", "a.note.this.build.dropped"],
      revision: 4,
      stored: true,
    });
    await syncNoticesWithDesktop(root);
    expect(remembered()).toEqual(["home.setup", "sharing.how"]);
    expect(SaveClosedNotices).not.toHaveBeenCalled();
  });

  it("hands over what the window remembers when the desktop has no list", async () => {
    window.localStorage.setItem(storageKey, JSON.stringify(["rhythm.corrections"]));
    const { root, state } = desktop({ hidden: [], revision: 0, stored: false });
    await syncNoticesWithDesktop(root);
    expect(state).toEqual({ hidden: ["rhythm.corrections"], revision: 1, stored: true });
    expect(remembered()).toEqual(["rhythm.corrections"]);
  });

  // An empty stored list means the owner chose to see every note again.
  it("does not bring back notes the owner chose to show again", async () => {
    window.localStorage.setItem(storageKey, JSON.stringify(["home.setup"]));
    const { root, SaveClosedNotices } = desktop({ hidden: [], revision: 3, stored: true });
    await syncNoticesWithDesktop(root);
    expect(remembered()).toEqual([]);
    expect(SaveClosedNotices).not.toHaveBeenCalled();
  });

  it("tells the desktop when a note is closed or all are shown again", async () => {
    const { root, state, SaveClosedNotices } = desktop({
      hidden: ["home.setup"],
      revision: 7,
      stored: true,
    });
    Object.assign(globalThis, { go: root.go });
    window.localStorage.setItem(storageKey, JSON.stringify(["home.setup"]));

    hideNotice("sharing.how");
    // Closed at once in the window, whatever the desktop says later.
    expect(remembered()).toEqual(["home.setup", "sharing.how"]);
    await vi.waitFor(() => expect(SaveClosedNotices).toHaveBeenCalledTimes(1));
    // Saved against the revision the desktop holds, not one the window guessed.
    expect(SaveClosedNotices).toHaveBeenLastCalledWith({
      baseRevision: 7,
      hidden: ["home.setup", "sharing.how"],
    });

    showAllNotices();
    await vi.waitFor(() => expect(state.hidden).toEqual([]));
    expect(state.stored).toBe(true);
  });

  it("works without a desktop, on the window's own copy", async () => {
    await syncNoticesWithDesktop({} as WailsRoot);
    hideNotice("home.estimate");
    expect(remembered()).toEqual(["home.estimate"]);
  });
});
