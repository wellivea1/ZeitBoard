import { describe, expect, it } from "vitest";
import { loadSetupGuide, normalizeSetupGuide, setupStepWords } from "./setupGuide";

function rootWith(method: () => Promise<unknown>) {
  return { go: { main: { App: { GetSetupGuide: method } } } };
}

describe("normalizeSetupGuide", () => {
  it("keeps each step the window has words for, once, in the desktop's order", () => {
    expect(
      normalizeSetupGuide({
        steps: [
          { id: "reaching", done: true },
          { id: "phone", done: false },
          { id: "teleport", done: true },
          { id: "reaching", done: false },
          { id: "calendar" },
        ],
      }),
    ).toEqual([
      { id: "reaching", done: true },
      { id: "phone", done: false },
      // An absent flag is not done.
      { id: "calendar", done: false },
    ]);
  });

  it("refuses a payload it cannot read", () => {
    expect(normalizeSetupGuide({})).toBeUndefined();
    expect(normalizeSetupGuide({ steps: ["phone"] })).toBeUndefined();
  });

  // Every step names a place where it is done.
  it("gives every step a title, a sentence and a link", () => {
    for (const words of Object.values(setupStepWords)) {
      expect(words.title).not.toBe("");
      expect(words.body).toMatch(/\.$/);
      expect(words.link.href).toMatch(/^#\//);
    }
  });
});

describe("loadSetupGuide", () => {
  it("reads the steps from the desktop", async () => {
    const steps = await loadSetupGuide(
      rootWith(async () => ({ steps: [{ id: "phone", done: true }] })),
    );
    expect(steps).toEqual([{ id: "phone", done: true }]);
  });

  // Outside the desktop, or when the read fails, there is nothing to set up
  // rather than a list of steps that only look undone.
  it("offers no steps it cannot vouch for", async () => {
    expect(await loadSetupGuide({})).toEqual([]);
    expect(
      await loadSetupGuide(
        rootWith(async () => {
          throw new Error("the store is closed");
        }),
      ),
    ).toEqual([]);
    expect(await loadSetupGuide(rootWith(async () => ({ steps: 3 })))).toEqual([]);
  });
});
