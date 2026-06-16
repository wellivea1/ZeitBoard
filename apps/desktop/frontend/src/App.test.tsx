import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import App from "./App";

beforeEach(() => {
  window.location.hash = "#/overview";
});

describe("desktop navigation", () => {
  it("exposes every requested screen", () => {
    render(<App />);
    const navigation = screen.getByRole("navigation", { name: "Primary navigation" });

    for (const label of [
      "Overview",
      "Calendar",
      "Tasks",
      "Approvals",
      "Rhythm",
      "Medications",
      "Sharing",
      "Data Sources",
    ]) {
      expect(navigation).toHaveTextContent(label);
    }
    expect(screen.getByRole("link", { name: "Settings" })).toBeVisible();
    expect(screen.getByText("Sample data")).toBeVisible();
  });

  it("renders approval proposals with explicit actions", () => {
    window.location.hash = "#/approvals";
    render(<App />);

    expect(screen.getByRole("heading", { name: "Approvals" })).toBeVisible();
    expect(screen.getByText("Email Dr. Okafor")).toBeVisible();
    expect(screen.getAllByRole("button", { name: "Accept proposal" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Reject proposal" })).toHaveLength(2);
  });

  it("switches rhythm tabs between actogram and source review", () => {
    window.location.hash = "#/rhythm";
    render(<App />);

    expect(screen.getByRole("heading", { name: "Rhythm" })).toBeVisible();
    // Actogram is the default tab; correction/source review lives under Sources.
    expect(screen.getByRole("heading", { name: "Double-plot actogram" })).toBeVisible();
    expect(
      screen.getByText(
        "Approximate. Forecast widens with time and is shown as ranges, not hard lines.",
      ),
    ).toBeVisible();
    expect(
      screen.getByRole("img", {
        name: "Predicted sleep window: Jun 18, Jun 18, 11:21 PM earliest to Jun 19, 5:27 AM latest, 6 hr 6 min window, Forecast cycle 3, Low confidence",
      }),
    ).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Correction inspector" })).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "Sources" }));

    expect(screen.getByRole("heading", { name: "Correction inspector" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Undo correction" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Source conflicts and missingness" })).toBeVisible();
    expect(screen.getByText("Wearable sleep overlaps desktop activity")).toBeVisible();
  });

  it("renders the rhythm drift visualizer instead of a placeholder", () => {
    window.location.hash = "#/rhythm";
    render(<App />);

    fireEvent.click(screen.getByRole("tab", { name: "Drift" }));

    expect(screen.getByRole("heading", { name: "Sleep-onset drift" })).toBeVisible();
    expect(screen.getAllByText("+48 min per cycle").length).toBeGreaterThan(0);
    expect(screen.getByText("Theil-Sen fit")).toBeVisible();
    expect(
      screen.getByText(
        "Y-axis is unwrapped so the free-running trend stays readable across midnight.",
      ),
    ).toBeVisible();
    expect(
      screen.queryByText("The sleep-onset drift chart arrives with the sleep visualizer"),
    ).toBeNull();
  });

  it("keeps the legacy timeline route usable as Rhythm", () => {
    window.location.hash = "#/timeline";
    render(<App />);

    expect(screen.getByRole("heading", { name: "Rhythm" })).toBeVisible();
  });

  it("shows source missingness on the data sources screen", () => {
    window.location.hash = "#/data-sources";
    render(<App />);

    expect(screen.getByText("Permission missing")).toBeVisible();
    expect(screen.getByText("Last sync is stale")).toBeVisible();
  });
});
