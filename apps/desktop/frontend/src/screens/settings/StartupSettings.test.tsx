import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StartupSettings } from "./StartupSettings";

const initial = {
  available: true,
  registered: false,
  matchesCurrent: false,
  startHidden: false,
  trayAvailable: true,
  message: "",
};
function install(app: Record<string, unknown>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: app } };
}
afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

it("only saves startup after the owner chooses and submits it", async () => {
  const save = vi.fn(async (input: { enabled: boolean; startHidden: boolean }) => ({
    ...initial,
    registered: input.enabled,
    matchesCurrent: input.enabled,
    startHidden: input.startHidden,
  }));
  install({ GetStartupSettings: async () => initial, SetStartupSettings: save });
  render(<StartupSettings />);
  await screen.findByText("Off");
  expect(save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByLabelText("Start ZeitBoard when I sign in"));
  expect(save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Save startup settings" }));
  await screen.findByText("Registered with Windows");
  expect(save).toHaveBeenCalledWith({ enabled: true, startHidden: true });
  expect(screen.getByRole("status")).toHaveTextContent("consent settings are unchanged");
});

it("allows explicit quit when the tray is unavailable and prevents hiding", async () => {
  const quit = vi.fn(async () => undefined);
  install({
    GetStartupSettings: async () => ({ ...initial, trayAvailable: false }),
    QuitApp: quit,
  });
  render(<StartupSettings />);
  await screen.findByText(/The tray is unavailable/);
  expect(screen.getByRole("button", { name: "Hide to tray" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Quit ZeitBoard" }));
  await waitFor(() => expect(quit).toHaveBeenCalledOnce());
});

it("does not disguise missing startup services as an off registration", async () => {
  render(<StartupSettings />);
  await screen.findByText(/Open the ZeitBoard desktop app to manage startup/);
  expect(screen.getByRole("button", { name: "Save startup settings" })).toBeDisabled();
  install({ GetStartupSettings: async () => initial });
  fireEvent.click(screen.getByRole("button", { name: "Refresh startup status" }));
  await screen.findByText("Off");
  expect(screen.getByRole("button", { name: "Save startup settings" })).toBeEnabled();
});
