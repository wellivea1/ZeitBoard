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
  const eraseButton = screen.getByRole("button", { name: "Erase activity records" });
  expect(eraseButton).toBeDisabled();
  fireEvent.change(screen.getByLabelText("Type DELETE to erase activity records"), {
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
