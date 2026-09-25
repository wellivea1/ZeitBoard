import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { DataSourcesScreen } from "./DataSourcesScreen";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("DataSourcesScreen structure", () => {
  // After slice U-H this screen is about where records come from. Recording
  // last night is Log's job, and the two were only sharing a screen because
  // they had both grown there.
  it("leads with what is connected, then calendars, then importing", () => {
    render(<DataSourcesScreen />);
    const workspace = screen.getByRole("region", { name: "Data source review" });
    const headings = ["Connected", "Calendars", "Import sleep records"].map((name) =>
      screen.getByRole("heading", { name }),
    );

    expect(workspace.querySelector(".panel")).toBeNull();
    for (const [index, heading] of headings.slice(1).entries()) {
      expect(
        headings[index]!.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).not.toBe(0);
    }
  });

  // Calendars used to be added beside the calendar board in Plan. They are
  // set up here with the other inputs, and the board links to them.
  it("lists calendars here instead of in Plan", async () => {
    render(<DataSourcesScreen />);
    const calendars = screen.getByRole("region", { name: "Calendars" });
    expect(await within(calendars).findByText("Sample commitments")).toBeVisible();
  });

  // Somebody who used to add an entry here must be told where it went rather
  // than left hunting.
  it("says where the sleep log went", () => {
    render(<DataSourcesScreen />);
    expect(screen.queryByRole("heading", { name: "Add sleep entry" })).toBeNull();
    expect(screen.getByRole("link", { name: "Log" })).toHaveAttribute("href", "#/log/sleep");
  });
});
