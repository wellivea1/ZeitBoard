import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ProposalRecord } from "../data/proposals";
import { WeekScreen } from "./WeekScreen";

const decide = vi.fn();
let pending: ProposalRecord[] = [];

vi.mock("../state/approvals", () => ({
  useApprovals: () => ({ pending, busyProposalId: null, decide }),
}));

function suggestion(id: string, title: string, offsetHours?: number): ProposalRecord {
  const start = offsetHours === undefined ? undefined : Date.now() + offsetHours * 3_600_000;
  return {
    id,
    origin: "scheduler",
    kind: "Place",
    title,
    to: "Later today",
    rhythmContext: "about 4 hr after wake",
    confidence: "Medium",
    explanationCodes: [],
    reasonLabels: ["In a likely-awake window"],
    createdLabel: "Proposed by Scheduler",
    expiresLabel: "",
    decision: "pending",
    canUndo: false,
    ...(start === undefined
      ? {}
      : {
          startAt: new Date(start).toISOString(),
          endAt: new Date(start + 90 * 60_000).toISOString(),
        }),
  };
}

beforeEach(() => {
  decide.mockReset();
  pending = [];
  window.localStorage.clear();
});

describe("Plan › Week", () => {
  it("lays four days side by side and opens an event's details", async () => {
    const { container } = render(<WeekScreen />);
    expect(await screen.findByRole("region", { name: "Days with sleep and plans" })).toBeVisible();
    expect(container.querySelectorAll(".week-day-head")).toHaveLength(4);
    expect(container.querySelector(".week-day-head[data-today] em")).toHaveTextContent("Today");

    fireEvent.click(screen.getByRole("button", { name: /^Sample fixed event,/ }));
    const details = screen.getByRole("region", { name: "Selected event" });
    expect(within(details).getByRole("heading", { name: "Sample fixed event" })).toBeVisible();
    expect(within(details).getByText("From your calendar")).toBeVisible();

    fireEvent.click(screen.getByText(/List these events/));
    expect(screen.getByRole("table")).toBeVisible();
  });

  it("decides a suggestion on the board, and sends one with no time to Tasks", async () => {
    pending = [suggestion("p1", "Deep work", 1), suggestion("p2", "Call the bank")];
    render(<WeekScreen />);

    fireEvent.click(
      await screen.findByRole("button", { name: "Accept the suggested time for Deep work" }),
    );
    expect(decide).toHaveBeenCalledWith("p1", "approved");

    // Opening it shows why it was suggested, with the same two answers.
    fireEvent.click(screen.getByRole("button", { name: /^Deep work, suggested for/ }));
    const details = screen.getByRole("region", { name: "Selected suggestion" });
    expect(within(details).getByText(/In a likely-awake window/)).toBeVisible();
    fireEvent.click(within(details).getByRole("button", { name: "Decline" }));
    expect(decide).toHaveBeenLastCalledWith("p1", "rejected");

    expect(screen.getByText(/One suggestion has no exact time yet/)).toBeVisible();
    expect(screen.getByRole("link", { name: "decide it in Tasks" })).toHaveAttribute(
      "href",
      "#/plan/tasks",
    );
  });

  it("offers a suggestion across midnight once, and opens it from either day", async () => {
    // Only the clock is fixed, so Testing Library's waits keep running.
    vi.useFakeTimers({ toFake: ["Date"] });
    try {
      // Noon in UTC is the same date almost everywhere; the sample calendar
      // is drawn in UTC, where this suggestion crosses midnight.
      vi.setSystemTime(new Date("2026-09-24T12:00:00Z"));
      pending = [
        {
          ...suggestion("p1", "Deep work"),
          startAt: "2026-09-24T23:30:00.000Z",
          endAt: "2026-09-25T01:00:00.000Z",
        },
      ];
      const { container } = render(<WeekScreen />);

      expect(
        await screen.findAllByRole("button", { name: "Accept the suggested time for Deep work" }),
      ).toHaveLength(1);
      expect(screen.getAllByRole("button", { name: /^Deep work, suggested for/ })).toHaveLength(1);

      // The half hour after midnight is drawn in the next day, without controls.
      const rest = container.querySelector(".week-block[data-continued]");
      expect(rest).toHaveTextContent("Deep work");
      expect(rest?.querySelector("button")).toBeNull();
      fireEvent.click(rest!);
      expect(screen.getByRole("region", { name: "Selected suggestion" })).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
  });

  it("remembers how many days to show and steps by that many", async () => {
    const { container } = render(<WeekScreen />);
    await screen.findByRole("region", { name: "Days with sleep and plans" });
    const title = () => screen.getByRole("heading", { level: 2 }).textContent;
    const before = title();

    fireEvent.click(screen.getByRole("button", { name: "Week" }));
    await waitFor(() => expect(container.querySelectorAll(".week-day-head")).toHaveLength(7));
    expect(window.localStorage.getItem("zeitboard.week.days")).toBe("7");

    fireEvent.click(screen.getByRole("button", { name: "The 7 days after" }));
    expect(title()).not.toBe(before);
    fireEvent.click(screen.getByRole("button", { name: "Today" }));
    expect(container.querySelector(".week-day-head[data-today]")).not.toBeNull();
  });
});
