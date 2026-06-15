import { render, screen } from "@testing-library/react";
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
      "Timeline",
      "Medications",
      "Sharing",
      "Data Sources",
    ]) {
      expect(navigation).toHaveTextContent(label);
    }
    expect(screen.getByRole("link", { name: "Settings" })).toBeVisible();
    expect(screen.getByText("Sample data")).toBeVisible();
  });
});
