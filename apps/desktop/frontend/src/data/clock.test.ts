import { afterEach, describe, expect, it } from "vitest";
import { setWindowClock, windowClock } from "../utils/windowClock";
import { loadClockSetting, normalizeClockSetting, saveClockSetting } from "./clock";

function rootWith(methods: Record<string, (input?: unknown) => Promise<unknown>>) {
  return { go: { main: { App: methods } } };
}

const stored = { preference: "24", clock: "24", system: "12", revision: 3, conflict: false };

afterEach(() => {
  setWindowClock("12");
});

describe("the clock setting", () => {
  it("reads what the desktop says", () => {
    expect(normalizeClockSetting(stored)).toEqual(stored);
    expect(normalizeClockSetting({ ...stored, message: "Could not be stored." })?.message).toBe(
      "Could not be stored.",
    );
  });

  it("refuses a clock it does not know", () => {
    expect(normalizeClockSetting({ ...stored, clock: "13" })).toBeUndefined();
    expect(normalizeClockSetting({ ...stored, preference: "auto" })).toBeUndefined();
    expect(normalizeClockSetting(null)).toBeUndefined();
  });

  // Outside the desktop the browser's clock is the only one there is; a
  // desktop that does not answer leaves the window's clock as it is.
  it("falls back to the browser outside the desktop, and to nothing when the desktop fails", async () => {
    expect((await loadClockSetting({}))?.preference).toBe("system");
    expect(
      await loadClockSetting(
        rootWith({
          GetClockSetting: async () => {
            throw new Error("not ready");
          },
        }),
      ),
    ).toBeUndefined();
  });

  it("saves a choice with the revision it was made from", async () => {
    const sent: unknown[] = [];
    const saved = await saveClockSetting(
      "24",
      2,
      rootWith({
        SaveClockSetting: async (input) => {
          sent.push(input);
          return stored;
        },
      }),
    );
    expect(sent).toEqual([{ preference: "24", baseRevision: 2 }]);
    expect(saved.clock).toBe("24");
    await expect(saveClockSetting("12", 0, {})).rejects.toThrow(/desktop app/);
  });
});

describe("the window's clock", () => {
  it("is remembered on this computer for the first page drawn", () => {
    setWindowClock("24");
    expect(windowClock()).toBe("24");
    expect(window.localStorage.getItem("zeitboard.clock")).toBe("24");
  });
});
