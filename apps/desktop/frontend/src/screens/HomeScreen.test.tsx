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
    expect(screen.queryByText("Still learning your rhythm")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Still learning your rhythm")).toBeVisible();
    expect(screen.getByRole("link", { name: "Import sleep records" })).toHaveAttribute(
      "href",
      "#/data-sources",
    );
    await waitFor(() => expect(GetOverview).toHaveBeenCalledTimes(2));
  });
});
