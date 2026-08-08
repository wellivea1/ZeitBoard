import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { DataSourcesScreen } from "./DataSourcesScreen";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("DataSourcesScreen structure", () => {
  // After slice U-H this screen is about where records come from. Recording
  // last night is Log's job, and the two were only sharing a screen because
  // they had both grown there.
  it("leads with provenance in one ruled workspace", () => {
    const { container } = render(<DataSourcesScreen />);
    const workspace = screen.getByRole("region", { name: "Data source review" });
    const sourceHeading = screen.getByRole("heading", { name: "Source status" });
    const importHeading = screen.getByRole("heading", { name: /Import/ });

    expect(workspace.querySelector(".panel")).toBeNull();
    expect(
      sourceHeading.compareDocumentPosition(importHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
    expect(container).toHaveTextContent(
      "Local ICS files and read-only CalDAV snapshots are managed in Calendar",
    );
    expect(container).not.toHaveTextContent("Out of scope for this local sleep-data slice");
  });

  // Somebody who used to add an entry here must be told where it went rather
  // than left hunting.
  it("says where the sleep log went", () => {
    render(<DataSourcesScreen />);
    expect(screen.queryByRole("heading", { name: "Add sleep entry" })).toBeNull();
    expect(screen.getByRole("link", { name: "Log" })).toHaveAttribute("href", "#/log/sleep");
  });
});
