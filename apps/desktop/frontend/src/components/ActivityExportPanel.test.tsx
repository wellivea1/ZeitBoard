import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ActivityExportPanel } from "./ActivityExportPanel";
import { ActivityNightsPanel } from "./ActivityNightsPanel";

const none = { imported: false };
const kept = {
  imported: true,
  minutes: 4210,
  nights: 12,
  spanLabel: "Aug 31, 2026 to Sep 13, 2026",
  importedLabel: "Sep 20, 2026",
  zoneId: "UTC",
  firstDate: "2026-08-31",
  days: 14,
  suggested: [[0, 1410, 1440]],
  recorded: [[7, 300, 780]],
  agreement: {
    recorded: 6,
    matched: 6,
    summary:
      "On those nights, suggestions typically began 30 minutes before your recorded sleep and ended 20 minutes after your recorded wake.",
  },
};
const preview = {
  token: "activity_export_01",
  fileName: "takeout-20260920T000000Z-001.zip",
  entries: 4211,
  unusable: 1,
  minutes: 4210,
  nights: 12,
  spanLabel: "Aug 31, 2026 to Sep 13, 2026",
};

function install(methods: Record<string, (input?: unknown) => Promise<unknown>>) {
  (globalThis as unknown as { go?: unknown }).go = { main: { App: methods } };
}

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  delete (globalThis as unknown as { go?: unknown }).go;
});

describe("ActivityExportPanel", () => {
  it("says what is read before anything is chosen, and keeps nothing until asked", async () => {
    let current: unknown = none;
    const importExport = vi.fn<(input: unknown) => Promise<unknown>>(async () => {
      current = kept;
      return kept;
    });
    install({
      GetAccountActivity: async () => current,
      PreviewActivityExport: async () => preview,
      ImportActivityExport: importExport,
    });
    render(<ActivityExportPanel />);

    expect(screen.getByText(/keeps only when each activity happened, to the minute/)).toBeVisible();
    fireEvent.click(await screen.findByRole("button", { name: "Choose an export" }));

    const counts = await screen.findByLabelText("What the export holds");
    expect(within(counts).getByText("4,210")).toBeVisible();
    expect(
      screen.getByText(
        "takeout-20260920T000000Z-001.zip covers Aug 31, 2026 to Sep 13, 2026. 1 entry has no time that can be used and is left out. Nothing is kept until you keep it.",
      ),
    ).toBeVisible();
    expect(importExport).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Keep these times" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Kept. Only the times were saved.");
    expect(importExport).toHaveBeenCalledWith({ token: "activity_export_01" });
    expect(
      screen.getByText(/minutes with activity, from Aug 31, 2026 to Sep 13, 2026/),
    ).toBeVisible();
    expect(screen.getByRole("link", { name: "See the nights they suggest" })).toHaveAttribute(
      "href",
      "#/rhythm/sources",
    );
  });

  it("removes everything once REMOVE is typed back", async () => {
    let current: unknown = kept;
    const remove = vi.fn(async () => {
      current = none;
      return none;
    });
    install({
      GetAccountActivity: async () => current,
      PreviewActivityExport: async () => preview,
      RemoveAccountActivity: remove,
    });
    render(<ActivityExportPanel />);

    fireEvent.click(await screen.findByRole("button", { name: "Remove imported activity" }));
    const confirm = screen.getByRole("group", { name: "Remove the imported Google activity?" });
    const action = within(confirm).getByRole("button", { name: "Remove activity" });
    expect(action).toBeDisabled();
    fireEvent.change(within(confirm).getByLabelText("Type REMOVE to confirm"), {
      target: { value: "REMOVE" },
    });
    fireEvent.click(action);

    expect(await screen.findByRole("status")).toHaveTextContent(
      "The imported activity was removed.",
    );
    expect(remove).toHaveBeenCalledWith({ confirmation: "REMOVE" });
    expect(screen.getByRole("button", { name: "Choose an export" })).toBeEnabled();
  });

  it("says why an export cannot be read", async () => {
    install({
      GetAccountActivity: async () => none,
      PreviewActivityExport: async () => {
        throw new Error(
          "This export holds My Activity as web pages. In Google Takeout, set My Activity's format to JSON and export it again.",
        );
      },
    });
    render(<ActivityExportPanel />);
    fireEvent.click(await screen.findByRole("button", { name: "Choose an export" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("set My Activity's format to JSON");
  });

  it("cannot open an export outside the desktop app", () => {
    render(<ActivityExportPanel />);
    expect(screen.getByRole("button", { name: "Choose an export" })).toBeDisabled();
    expect(screen.getByText("Opening an export needs the ZeitBoard desktop app.")).toBeVisible();
  });
});

describe("ActivityNightsPanel", () => {
  it("is absent until activity is imported", async () => {
    install({ GetAccountActivity: async () => none });
    const { container } = render(<ActivityNightsPanel />);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(container).toBeEmptyDOMElement();
  });

  it("measures the suggestions on recorded nights", async () => {
    install({ GetAccountActivity: async () => kept });
    render(<ActivityNightsPanel />);
    expect(
      await screen.findByRole("heading", { name: "Nights your Google activity suggests" }),
    ).toBeVisible();
    expect(
      screen.getByText(/nights you recorded while this activity was kept had a suggestion/),
    ).toHaveTextContent(
      "6 of 6 nights you recorded while this activity was kept had a suggestion beside them. On those nights, suggestions typically began 30 minutes before your recorded sleep and ended 20 minutes after your recorded wake.",
    );
    expect(screen.getByText(/no link or assistant sees them/)).toBeVisible();
  });
});
