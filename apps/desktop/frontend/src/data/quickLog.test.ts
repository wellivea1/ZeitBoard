import { describe, expect, it } from "vitest";
import {
  beginQuickSleep,
  completeQuickSleep,
  loadQuickLogState,
  normalizeQuickLogResult,
  normalizeQuickLogState,
  quickLogUnavailable,
} from "./quickLog";

function result(overrides: Record<string, unknown> = {}) {
  return {
    outcome: "record",
    reason: "Recorded.",
    recorded: true,
    entry: "obs_1",
    suggestionIsPrediction: false,
    state: { status: "ok", pending: false, pendingStale: false },
    ...overrides,
  };
}

function rootWith(name: string, method: (input?: unknown) => Promise<unknown>) {
  return { go: { main: { App: { [name]: method } } } };
}

describe("normalizeQuickLogState", () => {
  it("accepts an unfinished sleep", () => {
    const state = normalizeQuickLogState({
      status: "ok",
      pending: true,
      pendingLabel: "Sleep marked Mon 11:20 PM",
      pendingSince: "2026-08-08T23:20",
      pendingStale: true,
    });
    expect(state?.pending).toBe(true);
    expect(state?.pendingStale).toBe(true);
    expect(state?.pendingLabel).toBe("Sleep marked Mon 11:20 PM");
  });

  it("rejects a status this build does not know", () => {
    expect(normalizeQuickLogState({ status: "maybe", pending: false })).toBeUndefined();
  });

  // An absent flag is the safe reading of both: no unfinished sleep, and not
  // stale. Coercing a missing `pendingStale` to true would hide the one-tap
  // path for no reason; coercing a missing `pending` to true would offer to
  // close a sleep that was never marked.
  it("treats missing flags as false", () => {
    const state = normalizeQuickLogState({ status: "ok" });
    expect(state).toEqual({ status: "ok", pending: false, pendingStale: false });
  });
});

describe("normalizeQuickLogResult", () => {
  it("accepts each outcome the app can produce", () => {
    for (const outcome of [
      "record",
      "pending",
      "discarded",
      "confirm_onset",
      "confirm_short",
      "confirm_long",
      "confirm_stale",
      "reject",
    ]) {
      expect(normalizeQuickLogResult(result({ outcome }))?.outcome).toBe(outcome);
    }
  });

  // The outcome set is closed on purpose. A newer app sending an outcome this
  // build cannot render must fail loudly rather than fall through to whatever
  // the screen does by default — the gap between "recorded" and "needs an
  // answer" is a night in the log or a night lost.
  it("refuses an outcome it cannot render", () => {
    expect(normalizeQuickLogResult(result({ outcome: "logged_it_probably" }))).toBeUndefined();
  });

  it("refuses a result with no state to show afterwards", () => {
    expect(normalizeQuickLogResult(result({ state: undefined }))).toBeUndefined();
  });

  it("carries the prediction flag and the prefills", () => {
    const parsed = normalizeQuickLogResult(
      result({
        outcome: "confirm_onset",
        recorded: false,
        suggestedStartLocal: "2026-08-08T23:20",
        suggestedEndLocal: "2026-08-09T07:40",
        suggestionIsPrediction: true,
      }),
    );
    expect(parsed?.recorded).toBe(false);
    expect(parsed?.suggestionIsPrediction).toBe(true);
    expect(parsed?.suggestedStartLocal).toBe("2026-08-08T23:20");
  });

  it("does not let a missing flag imply a real observation", () => {
    const parsed = normalizeQuickLogResult({
      outcome: "confirm_short",
      reason: "That is short for a night.",
      state: { status: "ok", pending: true, pendingStale: false },
    });
    expect(parsed?.recorded).toBe(false);
    expect(parsed?.suggestionIsPrediction).toBe(false);
  });
});

describe("loadQuickLogState", () => {
  it("reads the state from the app", async () => {
    const root = rootWith("GetQuickLogState", async () => ({
      status: "ok",
      pending: true,
      pendingStale: false,
    }));
    expect((await loadQuickLogState(root)).pending).toBe(true);
  });

  it("is unavailable outside the desktop app", async () => {
    expect(await loadQuickLogState({})).toEqual(quickLogUnavailable);
  });

  it("is unavailable when the app answers with nonsense", async () => {
    const root = rootWith("GetQuickLogState", async () => "no");
    expect(await loadQuickLogState(root)).toEqual(quickLogUnavailable);
  });
});

describe("taps", () => {
  it("sends the tap and returns what came back", async () => {
    let called = 0;
    const root = rootWith("BeginQuickSleep", async () => {
      called += 1;
      return result({
        outcome: "pending",
        recorded: false,
        reason: "Sleep marked.",
        state: { status: "ok", pending: true, pendingStale: false },
      });
    });
    const tapped = await beginQuickSleep(root);
    expect(called).toBe(1);
    expect(tapped.outcome).toBe("pending");
    expect(tapped.state.pending).toBe(true);
  });

  it("fails rather than pretending a tap landed", async () => {
    await expect(completeQuickSleep({})).rejects.toThrow(/desktop app/);
  });

  it("fails when the answer cannot be trusted", async () => {
    const root = rootWith("CompleteQuickSleep", async () => ({ outcome: "record" }));
    await expect(completeQuickSleep(root)).rejects.toThrow();
  });
});
