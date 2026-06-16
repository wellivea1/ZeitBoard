import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
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
      <div data-testid="effective-theme">{effectiveTheme}</div>
      <div data-testid="reduced-attr">{reducedStimulation ? "true" : "false"}</div>
    </div>
  );
}

describe("useAppearance", () => {
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
});
