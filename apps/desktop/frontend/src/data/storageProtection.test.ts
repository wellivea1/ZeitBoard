import { describe, expect, it } from "vitest";
import {
  loadStorageProtection,
  normalizeStorageProtection,
  storageProtectionUnavailable,
} from "./storageProtection";

function payload(overrides: Record<string, unknown> = {}) {
  return {
    state: "ok",
    headline: "Local files are restricted to your account.",
    detail:
      "These files are restricted to your account, not encrypted. Anyone who can read this disk from another operating system, and any program running as you, can still read them.",
    files: [
      { name: "Sleep database", ownerOnly: true, inherited: false },
      { name: "Backend token", ownerOnly: true, inherited: false },
    ],
    ...overrides,
  };
}

describe("normalizeStorageProtection", () => {
  it("accepts a complete report", () => {
    const parsed = normalizeStorageProtection(payload());
    expect(parsed?.state).toBe("ok");
    expect(parsed?.files).toHaveLength(2);
  });

  // An unknown permission is not a good one. Defaulting a missing flag to true
  // would paint an unchecked file green.
  it("treats a missing ownerOnly flag as not owner-only", () => {
    const parsed = normalizeStorageProtection(payload({ files: [{ name: "Sleep database" }] }));
    expect(parsed?.files[0]?.ownerOnly).toBe(false);
  });

  it("drops entries with no name to show", () => {
    const parsed = normalizeStorageProtection(
      payload({ files: [{ ownerOnly: true }, { name: "Backend token", ownerOnly: true }] }),
    );
    expect(parsed?.files.map((file) => file.name)).toEqual(["Backend token"]);
  });

  it("refuses a state this build cannot render", () => {
    expect(normalizeStorageProtection(payload({ state: "probably_fine" }))).toBeUndefined();
  });

  it("refuses a report with no detail, which is where the caveat lives", () => {
    expect(normalizeStorageProtection(payload({ detail: undefined }))).toBeUndefined();
  });

  it("carries the at-risk state through", () => {
    const parsed = normalizeStorageProtection(
      payload({
        state: "at_risk",
        headline: "Some local files are readable by other accounts on this computer.",
        files: [
          {
            name: "Sleep database",
            ownerOnly: false,
            inherited: true,
            note: "another account on this computer can read it",
          },
        ],
      }),
    );
    expect(parsed?.state).toBe("at_risk");
    expect(parsed?.files[0]?.note).toMatch(/another account/);
  });
});

describe("loadStorageProtection", () => {
  it("reads the report from the app", async () => {
    const root = { go: { main: { App: { GetStorageProtection: async () => payload() } } } };
    expect((await loadStorageProtection(root)).state).toBe("ok");
  });

  it("does not claim protection it could not check", async () => {
    const report = await loadStorageProtection({});
    expect(report).toEqual(storageProtectionUnavailable);
    expect(report.state).toBe("unknown");
  });
});

// The whole point of this readout is that it does not overstate what a file
// permission gives.
describe("wording", () => {
  it("never describes restricted files as encrypted", () => {
    const parsed = normalizeStorageProtection(payload());
    expect(parsed?.detail).toMatch(/not encrypted/);
    expect(parsed?.headline).not.toMatch(/encrypt/i);
  });
});
