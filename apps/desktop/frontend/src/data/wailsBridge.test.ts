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

  it("sends no argument when there is no input, as a method without parameters requires", async () => {
    // Wails serialises the arguments a binding receives: an explicit
    // undefined arrives in Go as [null], which a method without parameters
    // rejects without ever answering.
    const received: unknown[][] = [];
    const service = {
      Run(...args: unknown[]) {
        received.push(args);
        return Promise.resolve(JSON.stringify(args));
      },
    };
    const method = findWailsMethod({ go: { main: { App: service } } }, ["Run"])!;

    await expect(method()).resolves.toBe("[]");
    await expect(method(undefined)).resolves.toBe("[]");
    await expect(method(null)).resolves.toBe("[null]");
    await expect(method({ cursor: "c1" })).resolves.toBe('[{"cursor":"c1"}]');
    expect(received.map((args) => args.length)).toEqual([0, 0, 1, 1]);
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
