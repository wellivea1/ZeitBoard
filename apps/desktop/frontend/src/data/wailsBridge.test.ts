import { describe, expect, it } from "vitest";
import { findWailsMethod } from "./wailsBridge";

describe("Wails bridge lookup", () => {
  it("finds a nested method and preserves its service binding", async () => {
    const service = {
      marker: "bound",
      Run(this: { marker: string }, input?: unknown) {
        return Promise.resolve({ marker: this.marker, input });
      },
    };

    const method = findWailsMethod({ go: { main: { App: service } } }, ["Run"]);
    expect(method).toBeDefined();

    await expect(method!({ value: 1 })).resolves.toEqual({
      marker: "bound",
      input: { value: 1 },
    });
  });

  it("skips malformed bridge branches", () => {
    const root = {
      go: {
        missingPackage: null,
        main: {
          missingService: 7,
          App: { Run: "not a function" },
        },
      },
    };

    expect(findWailsMethod(root, ["Run"])).toBeUndefined();
    expect(findWailsMethod({ go: "invalid" }, ["Run"])).toBeUndefined();
  });
});
