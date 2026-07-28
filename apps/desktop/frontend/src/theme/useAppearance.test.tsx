import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { LocalAppearanceEnvelope } from "../data/localAgent";
import { AppearanceProvider, useAppearanceContext } from "./AppearanceProvider";

function TestSettings() {
  const { theme, effectiveTheme, reducedStimulation, setTheme, setReducedStimulation } =
    useAppearanceContext();
  return (
    <div>
      <label>
        Theme
        <select
          value={theme}
          onChange={(event) => setTheme(event.target.value as "auto" | "light" | "dark")}
          data-testid="theme-select"
        >
          <option value="auto">Auto</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </label>
      <label>
        Reduced stimulation
        <input
          type="checkbox"
          checked={reducedStimulation}
          onChange={(event) => setReducedStimulation(event.target.checked)}
          data-testid="reduced-toggle"
        />
      </label>
      <div data-testid="theme-state">{theme}</div>
      <div data-testid="effective-theme">{effectiveTheme}</div>
      <div data-testid="reduced-attr">{reducedStimulation ? "true" : "false"}</div>
    </div>
  );
}

describe("useAppearance", () => {
  afterEach(() => {
    delete (globalThis as { go?: unknown }).go;
    delete (globalThis as { runtime?: unknown }).runtime;
    localStorage.clear();
  });

  it("applies theme changes to the document element", async () => {
    render(
      <AppearanceProvider>
        <TestSettings />
      </AppearanceProvider>,
    );

    await waitFor(() =>
      expect(screen.getByTestId("effective-theme")).toHaveTextContent(/light|dark/),
    );

    fireEvent.change(screen.getByTestId("theme-select"), { target: { value: "dark" } });

    await waitFor(() => expect(screen.getByTestId("effective-theme")).toHaveTextContent("dark"));
    expect(localStorage.getItem("zeitboard-theme")).toBe("dark");
  });

  it("applies reduced-stimulation changes to the document element", async () => {
    render(
      <AppearanceProvider>
        <TestSettings />
      </AppearanceProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("reduced-attr")).toHaveTextContent("false"));

    fireEvent.click(screen.getByTestId("reduced-toggle"));

    await waitFor(() => expect(screen.getByTestId("reduced-attr")).toHaveTextContent("true"));
    expect(localStorage.getItem("zeitboard-reduced")).toBe("true");
  });

  it("keeps a newer agent command when an older UI save resolves with a conflict", async () => {
    let appearanceEvent: ((value: unknown) => void) | undefined;
    let resolveSave: ((value: LocalAppearanceEnvelope) => void) | undefined;
    const saved = new Promise<LocalAppearanceEnvelope>((resolve) => {
      resolveSave = resolve;
    });
    const initialState = {
      theme: "light",
      reducedStimulation: false,
      nightRule: {
        enabled: false,
        preset: "amber",
        leadHours: 2,
        fallbackStartLocal: "",
        fallbackEndLocal: "",
      },
    } as const;
    const commandState = { ...initialState, theme: "amber" as const };
    const save = vi.fn(() => saved);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          LoadLocalAppearanceState: async () => ({
            state: initialState,
            revision: 1,
            conflict: false,
          }),
          SaveLocalAppearanceState: save,
        },
      },
    };
    (globalThis as { runtime?: unknown }).runtime = {
      EventsOn: (_eventName: string, callback: (value: unknown) => void) => {
        appearanceEvent = callback;
        return () => {};
      },
    };

    render(
      <AppearanceProvider>
        <TestSettings />
      </AppearanceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("theme-state")).toHaveTextContent("light"));

    fireEvent.change(screen.getByTestId("theme-select"), { target: { value: "dark" } });
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith({
      state: { ...initialState, theme: "dark" },
      baseRevision: 1,
    });

    act(() => {
      appearanceEvent?.({ state: commandState, revision: 2, conflict: false });
    });
    await waitFor(() => expect(screen.getByTestId("theme-state")).toHaveTextContent("amber"));

    await act(async () => {
      resolveSave?.({
        state: { ...initialState, theme: "dark" },
        revision: 2,
        conflict: true,
      });
      await saved;
    });

    expect(screen.getByTestId("theme-state")).toHaveTextContent("amber");
    expect(localStorage.getItem("zeitboard-theme")).toBe("amber");
  });
});
