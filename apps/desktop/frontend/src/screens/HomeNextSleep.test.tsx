import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { OutlookSegment } from "../data/outlook";
import { HomeScreen } from "./HomeScreen";

vi.mock("../state/approvalQueue", () => ({
  useApprovalQueue: () => ({
    pendingCount: 0,
    ready: true,
    incomplete: false,
    breakdown: { suggestions: 0, conflicts: 0, assistant: 0, requests: 0 },
  }),
}));

// The forecast puts now inside a sleep: three hours asleep, then the waking
// boundary. There is no onset ahead, only this sleep's end.
vi.mock("../data/fixture", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../data/fixture")>();
  const segment = (
    presence: OutlookSegment["presence"],
    offsetHours: number,
    durationHours: number,
  ): OutlookSegment => ({
    presence,
    observed: false,
    rangeLabel: "",
    dayLabel: "",
    durationLabel: "",
    offsetHours,
    durationHours,
  });
  return {
    ...actual,
    outlookFixture: {
      ...actual.outlookFixture,
      horizonStart: new Date().toISOString(),
      segments: [segment("asleep", 0, 3), segment("uncertain", 3, 2), segment("awake", 5, 14)],
    },
  };
});

describe("Home's sleep panel", () => {
  // Found by using the app at 3 AM: the panel was titled "Next sleep" and
  // showed only a waking window.
  it("leads with waking when the forecast already has you asleep", async () => {
    render(<HomeScreen />);
    expect(await screen.findByText("Likely waking")).toBeVisible();
    expect(screen.queryByText("Next sleep")).toBeNull();
  });
});
