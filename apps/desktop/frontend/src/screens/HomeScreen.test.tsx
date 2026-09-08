import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HomeScreen } from "./HomeScreen";
import { overviewUnavailable } from "../data/overview";

vi.mock("../state/approvals", () => ({ usePendingApprovalsCount: () => 0 }));
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
    expect(screen.queryByText("Likely awake")).toBeNull();
    expect(screen.getByText("Loading your rhythm…")).toBeVisible();
    reject(new Error("synthetic service failure"));
    expect(await screen.findByRole("heading", { name: "Estimate unavailable" })).toBeVisible();
    expect(screen.queryByText("Still learning your rhythm")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(await screen.findByText("Still learning your rhythm")).toBeVisible();
    expect(screen.getByRole("link", { name: "Import sleep records" })).toHaveAttribute(
      "href",
      "#/data-sources",
    );
    await waitFor(() => expect(GetOverview).toHaveBeenCalledTimes(2));
  });
});
