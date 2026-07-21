import { describe, expect, it } from "vitest";

import { defaultNightRule, evaluateNightWindow, type NightRule } from "./nightMode";

const rule: NightRule = { ...defaultNightRule, enabled: true, leadHours: 2 };
const clock = {
  status: "estimated",
  sleepStartAt: "2026-07-18T04:00:00Z", // predicted onset
  wakeAt: "2026-07-18T12:30:00Z",
};

describe("rhythm-linked night window", () => {
  it("engages the lead window before predicted sleep and releases at wake", () => {
    expect(evaluateNightWindow(rule, clock, new Date("2026-07-18T01:59:00Z"))).toEqual({
      active: false,
      source: "forecast",
    });
    expect(evaluateNightWindow(rule, clock, new Date("2026-07-18T02:00:00Z")).active).toBe(true);
    expect(evaluateNightWindow(rule, clock, new Date("2026-07-18T08:00:00Z")).active).toBe(true);
    expect(evaluateNightWindow(rule, clock, new Date("2026-07-18T12:30:00Z")).active).toBe(false);
  });

  it("tracks drift because the trigger is the forecast, not the clock", () => {
    const drifted = {
      ...clock,
      sleepStartAt: "2026-07-19T05:00:00Z",
      wakeAt: "2026-07-19T13:30:00Z",
    };
    expect(evaluateNightWindow(rule, drifted, new Date("2026-07-19T02:30:00Z")).active).toBe(false);
    expect(evaluateNightWindow(rule, drifted, new Date("2026-07-19T03:30:00Z")).active).toBe(true);
  });

  it("falls back to civil times when the estimator refuses, including midnight wrap", () => {
    const civil = { ...rule, fallbackStartLocal: "22:00", fallbackEndLocal: "07:00" };
    const refused = { status: "refused" };
    const at = (h: number, m: number) => {
      const d = new Date(2026, 6, 18, h, m); // local time
      return evaluateNightWindow(civil, refused, d);
    };
    expect(at(23, 0)).toEqual({ active: true, source: "civil" });
    expect(at(3, 0).active).toBe(true);
    expect(at(12, 0).active).toBe(false);
  });

  it("is honestly inactive without an estimate or fallback, and when disabled", () => {
    expect(evaluateNightWindow(rule, { status: "refused" }, new Date())).toEqual({
      active: false,
      source: null,
    });
    expect(
      evaluateNightWindow({ ...rule, enabled: false }, clock, new Date("2026-07-18T08:00:00Z")),
    ).toEqual({ active: false, source: null });
  });
});
