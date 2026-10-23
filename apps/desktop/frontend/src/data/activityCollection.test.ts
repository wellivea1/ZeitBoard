import { describe, expect, it } from "vitest";
import { activityState, type ActivityCollection } from "./activityCollection";

const off: ActivityCollection = {
  enabled: false,
  running: false,
  supported: true,
  zoneId: "UTC",
  recordCount: 0,
  lastError: "",
  powerLog: { supported: true, enabled: false, entries: 0, sinceLabel: "", lastError: "" },
};

// Settings and Data Sources both say this, and once said it differently: a
// system that cannot record read "Off" in one and "Unavailable" in the other.
describe("activityState", () => {
  it("names each state once, for every screen", () => {
    expect(activityState(undefined)).toBe("unavailable");
    expect(activityState(null)).toBe("unavailable");
    expect(activityState({ ...off, supported: false })).toBe("unavailable");
    expect(activityState(off)).toBe("off");
    expect(activityState({ ...off, enabled: true })).toBe("stopped");
    expect(activityState({ ...off, enabled: true, running: true })).toBe("running");
  });

  // Collection left on where it cannot run, as in a data folder brought from
  // Windows, is not "stopped": no retry could start it there.
  it("calls collection left on where it cannot run unavailable", () => {
    expect(activityState({ ...off, supported: false, enabled: true })).toBe("unavailable");
  });
});
