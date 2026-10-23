import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { setWindowClock, windowClock } from "../../utils/windowClock";
import { ClockSettings } from "./ClockSettings";

interface Saved {
  preference: string;
  baseRevision: number;
}

function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}

function stub(system: "12" | "24" = "12") {
  const saves: Saved[] = [];
  install({
    GetClockSetting: async () => ({
      preference: "system",
      clock: system,
      system,
      revision: 1,
      conflict: false,
    }),
    SaveClockSetting: async (input: unknown) => {
      const sent = input as Saved;
      saves.push(sent);
      const clock = sent.preference === "system" ? system : sent.preference;
      return {
        preference: sent.preference,
        clock,
        system,
        revision: sent.baseRevision + 1,
        conflict: false,
      };
    },
  });
  return saves;
}

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
  setWindowClock("12");
});

describe("ClockSettings", () => {
  // Each choice shows the same afternoon on its own clock.
  it("shows each clock as the time it writes, with Windows' own marked", async () => {
    stub("24");
    render(<ClockSettings />);
    const windows = await screen.findByRole("radio", { name: /As Windows does/ });
    expect(windows).toBeChecked();
    expect(windows.closest("label")).toHaveTextContent("15:40");
    expect(windows.closest("label")).toHaveTextContent("now the 24-hour clock");
    expect(screen.getByRole("radio", { name: /^12-hour/ }).closest("label")).toHaveTextContent(
      "3:40 PM",
    );
    expect(screen.getByRole("radio", { name: /^24-hour/ }).closest("label")).toHaveTextContent(
      "15:40",
    );
  });

  // Away from Windows there is no regional format to follow, and the first
  // choice offered to follow Windows anyway.
  it.each(["macos", "linux"])("names ZeitBoard's own default on %s", async (platform) => {
    install({
      GetClockSetting: async () => ({
        preference: "system",
        clock: "12",
        system: "12",
        platform,
        revision: 1,
        conflict: false,
      }),
    });
    render(<ClockSettings />);
    const first = await screen.findByRole("radio", { name: /ZeitBoard's default/ });
    expect(first).toBeChecked();
    expect(first.closest("label")).toHaveTextContent(
      "The 12-hour clock, until you choose another.",
    );
    expect(screen.queryByText(/Windows/)).toBeNull();
  });

  // The browser's clock once stood in for a desktop that did not answer, and
  // offered "As Windows does" on any system.
  it("says the setting could not be read rather than stand in for it", async () => {
    install({
      GetClockSetting: async () => {
        throw new Error("not ready");
      },
    });
    render(<ClockSettings />);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The clock setting could not be read.",
    );
    expect(screen.queryByRole("radio")).toBeNull();
  });

  // Outside the desktop the clock is the browser's, and was offered as
  // Windows' on any system.
  it("names the browser's clock as the browser's outside the desktop", async () => {
    render(<ClockSettings />);
    const first = await screen.findByRole("radio", { name: /As this browser does/ });
    expect(first).toBeChecked();
    expect(screen.queryByText(/Windows/)).toBeNull();
  });

  it("saves a choice and puts it in use at once", async () => {
    const saves = stub("12");
    render(<ClockSettings />);
    fireEvent.click(await screen.findByRole("radio", { name: /^24-hour/ }));
    await waitFor(() => expect(saves).toEqual([{ preference: "24", baseRevision: 1 }]));
    await waitFor(() => expect(windowClock()).toBe("24"));
    expect(screen.getByRole("radio", { name: /^24-hour/ })).toBeChecked();
  });

  it("says when another edit won", async () => {
    install({
      GetClockSetting: async () => ({
        preference: "system",
        clock: "12",
        system: "12",
        revision: 1,
        conflict: false,
      }),
      SaveClockSetting: async () => ({
        preference: "12",
        clock: "12",
        system: "12",
        revision: 4,
        conflict: true,
      }),
    });
    render(<ClockSettings />);
    fireEvent.click(await screen.findByRole("radio", { name: /^24-hour/ }));
    expect(await screen.findByRole("status")).toHaveTextContent(/yours was not/);
    expect(screen.getByRole("radio", { name: /^12-hour/ })).toBeChecked();
  });
});
