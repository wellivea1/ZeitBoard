import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuickLogBar } from "./QuickLogBar";

interface Calls {
  begin: number;
  complete: number;
  discard: number;
  confirm: unknown[];
}

function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}

function stub(overrides: Record<string, unknown> = {}): Calls {
  const calls: Calls = { begin: 0, complete: 0, discard: 0, confirm: [] };
  install({
    GetQuickLogState: async () => ({ status: "ok", pending: false, pendingStale: false }),
    BeginQuickSleep: async () => {
      calls.begin += 1;
      return {
        outcome: "pending",
        reason: "Sleep marked. Tap “I woke up” when you get up.",
        state: {
          status: "ok",
          pending: true,
          pendingLabel: "Sleep marked Sat 11:20 PM",
          pendingStale: false,
        },
      };
    },
    CompleteQuickSleep: async () => {
      calls.complete += 1;
      return {
        outcome: "record",
        reason: "Recorded 8 hours 10 minutes.",
        recorded: true,
        entry: "obs_1",
        state: { status: "ok", pending: false, pendingStale: false },
      };
    },
    DiscardQuickSleep: async () => {
      calls.discard += 1;
      return {
        outcome: "discarded",
        reason: "The unfinished sleep was discarded. Nothing was recorded.",
        state: { status: "ok", pending: false, pendingStale: false },
      };
    },
    ConfirmQuickSleep: async (input: unknown) => {
      calls.confirm.push(input);
      return {
        outcome: "record",
        reason: "Recorded.",
        recorded: true,
        entry: "obs_2",
        state: { status: "ok", pending: false, pendingStale: false },
      };
    },
    ...overrides,
  });
  return calls;
}

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
  vi.restoreAllMocks();
});

describe("QuickLogBar", () => {
  it("offers two taps and records the night on the second", async () => {
    const calls = stub();
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /going to sleep/i }));
    expect(calls.begin).toBe(1);
    expect(await screen.findByText("Sleep marked Sat 11:20 PM")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /i woke up/i }));
    await waitFor(() => expect(calls.complete).toBe(1));
    expect(await screen.findByText(/Recorded 8 hours 10 minutes/)).toBeTruthy();
    // The unfinished sleep is gone, so the discard escape hatch goes with it.
    expect(screen.queryByRole("button", { name: /discard/i })).toBeNull();
  });

  it("stays out of the way when quick logging is unavailable", async () => {
    install({
      GetQuickLogState: async () => ({
        status: "unavailable",
        pending: false,
        pendingStale: false,
      }),
    });
    const { container } = render(<QuickLogBar />);
    await waitFor(() => expect(container.querySelector(".quick-log")).toBeNull());
  });

  // A question is not a record. Nothing may reach the log until the person
  // answers it.
  it("asks instead of recording when the pair is implausible", async () => {
    const calls = stub({
      CompleteQuickSleep: async () => ({
        outcome: "confirm_short",
        reason: "That is 30 minutes — a nap, or the sleep tap was a mistake?",
        suggestedStartLocal: "2026-08-09T07:10",
        suggestedEndLocal: "2026-08-09T07:40",
        state: { status: "ok", pending: true, pendingStale: false },
      }),
    });
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /i woke up/i }));
    const form = await screen.findByRole("form", { name: /confirm the sleep times/i });
    expect(within(form).getByText(/a nap, or the sleep tap was a mistake/)).toBeTruthy();
    expect(calls.confirm).toHaveLength(0);

    // confirm_short is the only outcome where "this was a nap" is the likely
    // answer, so it is the only one that offers it.
    fireEvent.click(within(form).getByLabelText(/this was a nap/i));
    fireEvent.click(within(form).getByRole("button", { name: /record it/i }));
    await waitFor(() => expect(calls.confirm).toHaveLength(1));
    expect(calls.confirm[0]).toMatchObject({
      startLocal: "2026-08-09T07:10",
      endLocal: "2026-08-09T07:40",
      classification: "nap",
    });
  });

  // The whole project exists so that a forecast is never mistaken for a record.
  // A prefill drawn from the estimator has to say so on the field itself.
  it("labels a prefill that came from the forecast", async () => {
    stub({
      CompleteQuickSleep: async () => ({
        outcome: "confirm_onset",
        reason: "No sleep was marked. When did you fall asleep?",
        suggestedStartLocal: "2026-08-08T23:20",
        suggestedEndLocal: "2026-08-09T07:40",
        suggestionIsPrediction: true,
        state: { status: "ok", pending: false, pendingStale: false },
      }),
    });
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /i woke up/i }));
    expect(await screen.findByText(/predicted/i)).toBeTruthy();
    expect((screen.getByLabelText(/fell asleep/i) as HTMLInputElement).value).toBe(
      "2026-08-08T23:20",
    );
  });

  it("does not label a time the person marked themselves", async () => {
    stub({
      CompleteQuickSleep: async () => ({
        outcome: "confirm_long",
        reason: "That is 18 hours. Did you forget to tap when you woke?",
        suggestedStartLocal: "2026-08-08T13:20",
        suggestedEndLocal: "2026-08-09T07:20",
        state: { status: "ok", pending: true, pendingStale: false },
      }),
    });
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /i woke up/i }));
    await screen.findByRole("form", { name: /confirm the sleep times/i });
    expect(screen.queryByText(/predicted/i)).toBeNull();
    // No nap offer either: 18 hours is a missed wake tap, not a nap.
    expect(screen.queryByLabelText(/this was a nap/i)).toBeNull();
  });

  // Found in the preview: React reuses the input element when one question
  // replaces another, so an uncontrolled field showed the previous question's
  // time while the app's own suggestion said something else. Recording it would
  // have written a time that belonged to a different question.
  it("does not carry a time over from the previous question", async () => {
    let next: Record<string, unknown> = {
      outcome: "confirm_short",
      reason: "That is 30 minutes.",
      suggestedStartLocal: "2026-08-09T07:10",
      suggestedEndLocal: "2026-08-09T07:40",
      state: { status: "ok", pending: true, pendingStale: false },
    };
    const calls = stub({
      CompleteQuickSleep: async () => next,
      GetQuickLogState: async () => ({
        status: "ok",
        pending: true,
        pendingLabel: "Sleep marked Sat 11:20 PM",
        pendingStale: false,
      }),
    });
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /i woke up/i }));
    await screen.findByRole("form", { name: /confirm the sleep times/i });
    expect((screen.getByLabelText(/fell asleep/i) as HTMLInputElement).value).toBe(
      "2026-08-09T07:10",
    );

    // A second tap replaces the question while the form is still on screen,
    // which is where React reuses the element.
    next = {
      outcome: "confirm_onset",
      reason: "No sleep was marked. When did you fall asleep?",
      suggestedStartLocal: "2026-08-08T23:20",
      suggestedEndLocal: "2026-08-09T07:40",
      suggestionIsPrediction: true,
      state: { status: "ok", pending: false, pendingStale: false },
    };
    fireEvent.click(screen.getByRole("button", { name: /i woke up/i }));
    await waitFor(() =>
      expect((screen.getByLabelText(/fell asleep/i) as HTMLInputElement).value).toBe(
        "2026-08-08T23:20",
      ),
    );

    // And what is shown is what gets recorded.
    fireEvent.click(screen.getByRole("button", { name: /record it/i }));
    await waitFor(() => expect(calls.confirm).toHaveLength(1));
    expect(calls.confirm[0]).toMatchObject({ startLocal: "2026-08-08T23:20" });
  });

  it("records the time the person typed over the suggestion", async () => {
    const calls = stub({
      CompleteQuickSleep: async () => ({
        outcome: "confirm_long",
        reason: "That is 18 hours. Did you forget to tap when you woke?",
        suggestedStartLocal: "2026-08-08T13:20",
        suggestedEndLocal: "2026-08-09T07:20",
        state: { status: "ok", pending: true, pendingStale: false },
      }),
    });
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /i woke up/i }));
    const start = (await screen.findByLabelText(/fell asleep/i)) as HTMLInputElement;
    fireEvent.change(start, { target: { value: "2026-08-08T23:45" } });
    expect(start.value).toBe("2026-08-08T23:45");

    fireEvent.click(screen.getByRole("button", { name: /record it/i }));
    await waitFor(() => expect(calls.confirm).toHaveLength(1));
    expect(calls.confirm[0]).toMatchObject({
      startLocal: "2026-08-08T23:45",
      endLocal: "2026-08-09T07:20",
      classification: "principal",
    });
  });

  it("says an old unfinished sleep will be asked about rather than assumed", async () => {
    install({
      GetQuickLogState: async () => ({
        status: "ok",
        pending: true,
        pendingLabel: "Sleep marked Fri 9:00 AM",
        pendingStale: true,
      }),
    });
    render(<QuickLogBar />);
    expect(await screen.findByText(/will ask for the real times/i)).toBeTruthy();
  });

  it("lets an unfinished sleep be discarded without recording anything", async () => {
    const calls = stub({
      GetQuickLogState: async () => ({
        status: "ok",
        pending: true,
        pendingLabel: "Sleep marked Sat 11:20 PM",
        pendingStale: false,
      }),
    });
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /discard/i }));
    await waitFor(() => expect(calls.discard).toBe(1));
    expect(await screen.findByText(/Nothing was recorded/)).toBeTruthy();
    expect(calls.complete).toBe(0);
  });

  it("reports a failed tap rather than looking like it worked", async () => {
    stub({
      BeginQuickSleep: async () => {
        throw new Error("the store is locked");
      },
    });
    render(<QuickLogBar />);

    fireEvent.click(await screen.findByRole("button", { name: /going to sleep/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/the store is locked/);
  });
});
