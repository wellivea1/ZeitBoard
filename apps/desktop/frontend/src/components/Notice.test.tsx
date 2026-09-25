import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { HiddenNotesSettings } from "../screens/settings/HiddenNotesSettings";
import { noticeCatalog, showAllNotices } from "../state/notices";
import { Notice } from "./Notice";

beforeEach(() => {
  window.localStorage.clear();
  showAllNotices();
});

function page() {
  return render(
    <>
      <Notice id="markers.boundary">Markers are context only.</Notice>
      <Notice id="import.how">Importing explained.</Notice>
      <HiddenNotesSettings />
    </>,
  );
}

describe("dismissible notes", () => {
  it("closes with its ×, stays closed, and comes back from Settings", () => {
    const { unmount } = page();
    expect(screen.getByRole("complementary", { name: "What a context marker does" })).toBeVisible();
    expect(screen.getByText("None are hidden.")).toBeVisible();

    fireEvent.click(
      screen.getByRole("button", { name: "Hide the note “What a context marker does”" }),
    );
    expect(screen.queryByText("Markers are context only.")).toBeNull();
    const list = screen.getByRole("list");
    expect(within(list).getByText("What a context marker does")).toBeVisible();
    expect(within(list).getByText("Log › Context")).toBeVisible();

    // Remembered across a reload of the page.
    unmount();
    page();
    expect(screen.queryByText("Markers are context only.")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Show again" }));
    expect(screen.getByText("Markers are context only.")).toBeVisible();
    expect(screen.getByText("None are hidden.")).toBeVisible();
  });

  it("shows every hidden note again at once", () => {
    page();
    fireEvent.click(
      screen.getByRole("button", { name: "Hide the note “What a context marker does”" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Hide the note “How importing works”" }));
    fireEvent.click(screen.getByRole("button", { name: "Show all 2 again" }));
    expect(screen.getByText("Markers are context only.")).toBeVisible();
    expect(screen.getByText("Importing explained.")).toBeVisible();
  });

  it("names every note in the catalogue with a title and a place", () => {
    for (const entry of Object.values(noticeCatalog)) {
      expect(entry.title.length).toBeGreaterThan(3);
      expect(entry.where.length).toBeGreaterThan(3);
    }
  });
});
