import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ActivityCollectionSettings } from "./ActivityCollectionSettings";

const initial = {
  enabled: false,
  running: false,
  supported: true,
  zoneId: "UTC",
  recordCount: 4,
  lastError: "",
};
function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}
afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

it("requires an explicit enable action and lets the owner erase independently", async () => {
  const set = vi.fn(async (input: { enabled: boolean; zoneId: string }) => ({
    ...initial,
    ...input,
    running: input.enabled,
  }));
  const erase = vi.fn(async () => ({ ...initial, recordCount: 0 }));
  install({
    GetActivityCollection: async () => initial,
    SetActivityCollection: set,
    DeleteActivityData: erase,
  });
  render(<ActivityCollectionSettings />);
  await screen.findByText("Off");
  expect(set).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Enable local activity collection" }));
  await screen.findByText("Running");
  expect(set).toHaveBeenCalledWith({ enabled: true, zoneId: "UTC" });
  const eraseButton = screen.getByRole("button", { name: "Delete activity records" });
  expect(eraseButton).toBeDisabled();
  fireEvent.change(screen.getByLabelText("Type DELETE to delete activity records"), {
    target: { value: "DELETE" },
  });
  fireEvent.click(eraseButton);
  await waitFor(() => expect(erase).toHaveBeenCalledWith("DELETE"));
  await screen.findByText("Off");
});

it("fails visibly when the bridge is missing or malformed and offers refresh", async () => {
  render(<ActivityCollectionSettings />);
  await screen.findByText(/Open the ZeitBoard desktop app/);
  expect(screen.getByRole("button", { name: "Enable local activity collection" })).toBeDisabled();
  install({ GetActivityCollection: async () => ({ enabled: true }) });
  fireEvent.click(screen.getByRole("button", { name: "Refresh activity status" }));
  await screen.findByText(/Activity status is invalid/);
  install({ GetActivityCollection: async () => initial });
  fireEvent.click(screen.getByRole("button", { name: "Refresh activity status" }));
  await screen.findByText("Off");
  expect(screen.getByRole("button", { name: "Enable local activity collection" })).toBeEnabled();
});

it("shows a failed collector as stopped and requires an explicit retry", async () => {
  const failed = { ...initial, enabled: true, lastError: "Records could not be saved." };
  const set = vi.fn(async () => ({ ...failed, running: true, lastError: "" }));
  install({ GetActivityCollection: async () => failed, SetActivityCollection: set });
  render(<ActivityCollectionSettings />);
  await screen.findByText("Stopped");
  expect(screen.getByRole("alert")).toHaveTextContent("Records could not be saved.");
  fireEvent.click(screen.getByRole("button", { name: "Retry collection" }));
  await screen.findByText("Running");
  expect(set).toHaveBeenCalledWith({ enabled: true, zoneId: "UTC" });
});

// Away from Windows nothing records desktop activity, and the card offered a
// time zone and a switch that could never turn it on.
it("says plainly where desktop activity cannot be recorded", async () => {
  install({ GetActivityCollection: async () => ({ ...initial, supported: false }) });
  render(<ActivityCollectionSettings />);
  await screen.findByText("ZeitBoard cannot record desktop activity on this system.");
  expect(screen.queryByRole("button", { name: "Enable local activity collection" })).toBeNull();
  expect(screen.queryByLabelText("Activity time zone")).toBeNull();
  expect(screen.queryByText(/Windows/)).toBeNull();
});

// Reading Windows' log is a choice of its own: it says what it reads before it
// is made, and waits for desktop activity to be on.
it("offers to read sleep and wake from Windows once activity is on", async () => {
  const powerLog = { supported: true, enabled: false, entries: 0, sinceLabel: "", lastError: "" };
  const read = vi.fn(async () => ({
    ...initial,
    enabled: true,
    running: true,
    recordCount: 14,
    powerLog: { ...powerLog, enabled: true, entries: 10, sinceLabel: "Oct 1, 2026" },
  }));
  install({
    GetActivityCollection: async () => ({ ...initial, powerLog }),
    SetPowerLogReading: read,
  });
  const { unmount } = render(<ActivityCollectionSettings />);
  const card = await screen.findByRole("region", { name: "Sleep and wake from Windows" });
  expect(card).toHaveTextContent("ZeitBoard can read those times and nothing else Windows logs");
  expect(screen.getByRole("button", { name: "Read sleep and wake from Windows" })).toBeDisabled();
  expect(card).toHaveTextContent("Turn on desktop activity first.");
  unmount();

  install({
    GetActivityCollection: async () => ({ ...initial, enabled: true, running: true, powerLog }),
    SetPowerLogReading: read,
  });
  render(<ActivityCollectionSettings />);
  fireEvent.click(await screen.findByRole("button", { name: "Read sleep and wake from Windows" }));
  await screen.findByText("10 times, from Oct 1, 2026");
  expect(read).toHaveBeenCalledWith({ enabled: true });
  expect(screen.getByRole("button", { name: "Stop reading from Windows" })).toBeEnabled();
});
