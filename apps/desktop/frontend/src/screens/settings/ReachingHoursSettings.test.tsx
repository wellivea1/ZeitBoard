import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ReachingHoursSettings } from "./ReachingHoursSettings";

interface Saved {
  state: {
    enabled: boolean;
    label: string;
    startLocal: string;
    endLocal: string;
    days: number[];
    zoneId: string;
  };
  baseRevision: number;
}

function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}

function stub(state: Record<string, unknown> = {}, summary = "Weekdays, 9:00 AM to 5:00 PM.") {
  const saves: Saved[] = [];
  install({
    GetReachingHours: async () => ({
      state: {
        enabled: true,
        label: "Typical office hours",
        startLocal: "09:00",
        endLocal: "17:00",
        days: [1, 2, 3, 4, 5],
        zoneId: "America/New_York",
        ...state,
      },
      revision: 2,
      conflict: false,
      summary,
    }),
    SaveReachingHours: async (input: unknown) => {
      saves.push(input as Saved);
      const sent = input as Saved;
      return {
        state: sent.state,
        revision: sent.baseRevision + 1,
        conflict: false,
        summary: "Saved schedule.",
      };
    },
  });
  return saves;
}

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("ReachingHoursSettings", () => {
  it("says whose hours these are", async () => {
    stub();
    render(<ReachingHoursSettings />);
    expect(await screen.findByText(/someone else's hours, not yours/i)).toBeTruthy();
  });

  it("loads the stored schedule into the form", async () => {
    stub({ label: "The clinic", startLocal: "07:30", endLocal: "11:30", days: [2, 4] });
    render(<ReachingHoursSettings />);
    await waitFor(() =>
      expect((screen.getByLabelText(/whose hours/i) as HTMLInputElement).value).toBe("The clinic"),
    );
    expect((screen.getByLabelText(/^opens$/i) as HTMLInputElement).value).toBe("07:30");
    expect((screen.getByLabelText("Tue") as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText("Mon") as HTMLInputElement).checked).toBe(false);
  });

  it("saves the edited schedule with the revision it loaded", async () => {
    const saves = stub();
    render(<ReachingHoursSettings />);
    await screen.findByLabelText(/whose hours/i);

    fireEvent.change(screen.getByLabelText(/whose hours/i), {
      target: { value: "The all-night pharmacy" },
    });
    fireEvent.change(screen.getByLabelText(/^closes$/i), { target: { value: "23:00" } });
    fireEvent.click(screen.getByRole("button", { name: /^every day$/i }));
    fireEvent.click(screen.getByRole("button", { name: /save reaching hours/i }));

    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]).toMatchObject({
      baseRevision: 2,
      state: {
        label: "The all-night pharmacy",
        endLocal: "23:00",
        days: [0, 1, 2, 3, 4, 5, 6],
      },
    });
  });

  // An overnight window is the case the old hardcoded schedule could not even
  // express, so the form has to say what it means rather than look like a typo.
  it("explains a closing time that is earlier than the opening time", async () => {
    stub({ startLocal: "22:00", endLocal: "06:00" });
    render(<ReachingHoursSettings />);
    expect(await screen.findByText(/closes the next morning/i)).toBeTruthy();
  });

  it("explains equal times as open all day", async () => {
    stub({ startLocal: "00:00", endLocal: "00:00" });
    render(<ReachingHoursSettings />);
    expect(await screen.findByText(/open all day/i)).toBeTruthy();
  });

  it("shows the summary the outlook will print", async () => {
    stub({}, "The clinic you set: Tuesday and Thursday, 9:00 AM to 1:00 PM.");
    render(<ReachingHoursSettings />);
    expect(await screen.findByText(/Tuesday and Thursday/)).toBeTruthy();
  });

  // A lost edit reported as saved is worse than an error: the person walks away
  // believing a schedule is in force that is not.
  it("does not call a conflict a save", async () => {
    install({
      GetReachingHours: async () => ({
        state: {
          enabled: true,
          label: "Mine",
          startLocal: "09:00",
          endLocal: "17:00",
          days: [1],
          zoneId: "UTC",
        },
        revision: 2,
        conflict: false,
        summary: "Mine.",
      }),
      SaveReachingHours: async () => ({
        state: {
          enabled: true,
          label: "Theirs",
          startLocal: "10:00",
          endLocal: "16:00",
          days: [3],
          zoneId: "UTC",
        },
        revision: 5,
        conflict: true,
        summary: "Theirs.",
      }),
    });
    render(<ReachingHoursSettings />);
    await screen.findByLabelText(/whose hours/i);
    fireEvent.click(screen.getByRole("button", { name: /save reaching hours/i }));

    const status = await screen.findByRole("status");
    expect(status.textContent).toMatch(/was not applied/i);
    // The form shows what is actually stored, not the edit that lost.
    await waitFor(() =>
      expect((screen.getByLabelText(/whose hours/i) as HTMLInputElement).value).toBe("Theirs"),
    );
  });

  it("reports a failed save rather than looking like it worked", async () => {
    install({
      GetReachingHours: async () => ({
        state: {
          enabled: true,
          label: "Mine",
          startLocal: "09:00",
          endLocal: "17:00",
          days: [1],
          zoneId: "UTC",
        },
        revision: 1,
        conflict: false,
        summary: "Mine.",
      }),
      SaveReachingHours: async () => {
        throw new Error("choose at least one day, or turn reaching hours off");
      },
    });
    render(<ReachingHoursSettings />);
    await screen.findByLabelText(/whose hours/i);
    fireEvent.click(screen.getByRole("button", { name: /save reaching hours/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/at least one day/i);
  });

  it("lets the whole section be switched off", async () => {
    const saves = stub();
    render(<ReachingHoursSettings />);
    await screen.findByLabelText(/whose hours/i);

    fireEvent.click(screen.getByLabelText(/show reaching hours/i));
    fireEvent.click(screen.getByRole("button", { name: /save reaching hours/i }));
    await waitFor(() => expect(saves).toHaveLength(1));
    expect(saves[0]?.state.enabled).toBe(false);
  });

  it("discards an edit without saving it", async () => {
    const saves = stub({ label: "The clinic" });
    render(<ReachingHoursSettings />);
    await screen.findByLabelText(/whose hours/i);

    fireEvent.change(screen.getByLabelText(/whose hours/i), {
      target: { value: "Something else" },
    });
    fireEvent.click(screen.getByRole("button", { name: /discard changes/i }));
    expect((screen.getByLabelText(/whose hours/i) as HTMLInputElement).value).toBe("The clinic");
    expect(saves).toHaveLength(0);
  });
});
