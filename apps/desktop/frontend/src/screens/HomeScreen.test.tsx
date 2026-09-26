import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HomeScreen } from "./HomeScreen";
import { overviewUnavailable } from "../data/overview";

vi.mock("../state/approvalQueue", () => ({
  useApprovalQueue: () => ({
    pendingCount: 0,
    ready: true,
    incomplete: false,
    breakdown: { suggestions: 0, conflicts: 0, assistant: 0, requests: 0 },
  }),
}));
vi.mock("../state/approvals", () => ({
  useApprovals: () => ({ pending: [], busyProposalId: null, decide: vi.fn() }),
}));
afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("Home data recovery", () => {
  it("does not flash a sample forecast while the desktop loads, and offers retry on failure", async () => {
    let reject: (reason: Error) => void = () => {};
    const GetOverview = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((_, no) => {
            reject = no;
          }),
      )
      .mockResolvedValue({
        ...overviewUnavailable,
        status: "empty",
        empty: true,
        state: "No sleep entries yet",
      });
    (globalThis as { go?: unknown }).go = { main: { App: { GetOverview } } };
    render(<HomeScreen />);
    expect(screen.queryByText("Sample data")).toBeNull();
    expect(screen.queryByText(/You have been awake/)).toBeNull();
    expect(screen.getByText("Reading your records…")).toBeVisible();
    reject(new Error("synthetic service failure"));
    expect(await screen.findByText(/could not be read just now/)).toBeVisible();
    expect(screen.getByRole("heading", { name: "Your rhythm is not available yet" })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Ways to add nights" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByRole("heading", { name: "Ways to add nights" })).toBeVisible();
    expect(screen.getByRole("link", { name: "Import a file" })).toHaveAttribute(
      "href",
      "#/data-sources",
    );
    await waitFor(() => expect(GetOverview).toHaveBeenCalledTimes(2));
  });
});

describe("Home before the first forecast", () => {
  function serve(overview: Record<string, unknown>) {
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetOverview: async () => ({
            currentEstimatedState: "Need more sleep data",
            timeSinceWake: "Not available",
            predictedNextSleepWindow: "Not enough local data",
            driftEstimate: "Not enough local data",
            confidence: "low",
            confidenceReasons: [],
            nextUsefulTaskWindow: "No reliable proposal",
            sharingStatus: "No active trusted view; local data only",
            fixtureMode: false,
            empty: false,
            ...overview,
          }),
          GetQuickLogState: async () => ({ status: "ok", pending: false, pendingStale: false }),
        },
      },
    };
  }

  it("counts the nights so far and goes straight to each way of adding more", async () => {
    serve({
      status: "refused",
      refusal: { code: "insufficient_data", message: "need 7; found 3" },
      progress: { nights: 3, needed: 7 },
    });
    const { container } = render(<HomeScreen />);

    expect(
      await screen.findByText(
        "You have recorded 3 of the 7 nights ZeitBoard needs before it can look ahead.",
      ),
    ).toBeVisible();
    expect(screen.getByRole("heading", { name: "Fig. 1 Nights so far" })).toBeVisible();
    expect(container.querySelectorAll(".nights-tally li")).toHaveLength(7);
    expect(container.querySelectorAll(".nights-tally li[data-recorded]")).toHaveLength(3);
    expect(screen.getByText(/lasts three hours or more/)).toBeVisible();

    const links = screen
      .getAllByRole("link")
      .map((link) => [link.textContent, link.getAttribute("href")]);
    expect(links).toEqual(
      expect.arrayContaining([
        ["Add a past night", "#/log/sleep/add"],
        ["Import a file", "#/data-sources"],
        ["Set up sync", "#/settings/sync"],
      ]),
    );
    // The recording buttons are the first way, and they are on the page.
    expect(screen.getByRole("button", { name: /going to sleep/ })).toBeVisible();
  });

  it("says why records give no forecast when more nights alone would not help", async () => {
    serve({
      status: "refused",
      refusal: {
        code: "ambiguous_cycle_index",
        message: "cannot identify missing sleep cycles across a 36.2-hour gap",
      },
    });
    const { container } = render(<HomeScreen />);

    expect(await screen.findByText("Your records do not give a forecast yet.")).toBeVisible();
    expect(screen.getByRole("heading", { name: "Fig. 1 Why there is no forecast" })).toBeVisible();
    expect(screen.getByText(/too far apart to count the cycles/)).toBeVisible();
    expect(screen.getByText(/across a 36.2-hour gap/)).toBeVisible();
    expect(container.querySelector(".nights-tally")).toBeNull();
    expect(screen.getByRole("link", { name: "Add a past night" })).toHaveAttribute(
      "href",
      "#/log/sleep/add",
    );
    expect(screen.queryByRole("link", { name: "Set up sync" })).toBeNull();
  });
});
