import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { DataSourcesScreen } from "./DataSourcesScreen";

describe("DataSourcesScreen structure", () => {
  it("leads with provenance in one ruled workspace", () => {
    const { container } = render(<DataSourcesScreen />);
    const workspace = screen.getByRole("region", { name: "Data source review" });
    const sourceHeading = screen.getByRole("heading", { name: "Source status" });
    const entryHeading = screen.getByRole("heading", { name: "Add sleep entry" });

    expect(workspace.querySelector(".panel")).toBeNull();
    expect(
      sourceHeading.compareDocumentPosition(entryHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
    expect(container).toHaveTextContent(
      "Local ICS files and read-only CalDAV snapshots are managed in Calendar",
    );
    expect(container).not.toHaveTextContent("Out of scope for this local sleep-data slice");
  });
});
