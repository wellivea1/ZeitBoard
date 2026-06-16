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
    expect(screen.getByRole("heading", { name: "Sleep observations" })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Correction inspector" })).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "Sources" }));

    expect(screen.getByRole("heading", { name: "Correction inspector" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Undo correction" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Source conflicts and missingness" })).toBeVisible();
    expect(screen.getByText("Wearable sleep overlaps desktop activity")).toBeVisible();
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
