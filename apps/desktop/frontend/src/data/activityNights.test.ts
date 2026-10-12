import { afterEach, describe, expect, it } from "vitest";
import {
  loadComputerNights,
  noComputerNights,
  normalizeComputerNights,
  normalizeNightsFigure,
} from "./activityNights";

const figure = {
  nights: 10,
  spanLabel: "Sep 1, 2026 to Sep 11, 2026",
  zoneId: "UTC",
  firstDate: "2026-09-01",
  days: 11,
  suggested: [
    [0, 1380, 1440],
    [1, 0, 420],
  ],
  recorded: [[3, 1410, 1440]],
  agreement: { recorded: 3, matched: 3, summary: "On those nights, suggestions typically began…" },
};

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("activity nights", () => {
  it("refuses a figure it could not draw truthfully", () => {
    for (const broken of [
      { ...figure, suggested: [[11, 0, 60]] },
      { ...figure, recorded: [[0, 600, 600]] },
      { ...figure, recorded: [[0, 600, 1441]] },
      { ...figure, days: 0 },
      { ...figure, firstDate: undefined },
    ]) {
      expect(normalizeNightsFigure(broken)).toBeUndefined();
    }
  });

  it("drops an agreement that cannot be", () => {
    const read = normalizeNightsFigure({
      ...figure,
      agreement: { recorded: 2, matched: 3, summary: "" },
    });
    expect(read?.agreement).toBeUndefined();
    expect(read?.nights).toBe(10);
  });

  it("reads what this computer suggests", async () => {
    expect(normalizeComputerNights({ collected: false })).toBe(noComputerNights);
    expect(normalizeComputerNights({ collected: true, figure })?.figure?.nights).toBe(10);
    expect(normalizeComputerNights({ collected: true })).toBeUndefined();

    await expect(loadComputerNights()).resolves.toBe(noComputerNights);
    (globalThis as unknown as { go?: unknown }).go = {
      main: { App: { GetComputerNights: async () => ({ collected: "yes" }) } },
    };
    await expect(loadComputerNights()).rejects.toThrow("could not be read");
  });
});
