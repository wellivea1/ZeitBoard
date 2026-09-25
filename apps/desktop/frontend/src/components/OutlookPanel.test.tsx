import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OutlookPanel } from "./OutlookPanel";
import type { OutlookData } from "../data/outlook";

const base: OutlookData = {
  status: "available",
  freshness: { state: "current", explanation: "Based on recent records.", trusted: true },
  horizonLabel: "Next 72 hours",
  horizonHours: 72,
  days: [{ label: "Thu, Aug 6", offsetHours: 6 }],
  segments: [
    {
      presence: "awake",
      observed: false,
      rangeLabel: "6:00 PM to 11:20 PM",
      dayLabel: "Wed",
      durationLabel: "5 hours 20 minutes",
      offsetHours: 0,
      durationHours: 5.33,
    },
    {
      presence: "uncertain",
      observed: false,
      rangeLabel: "11:20 PM to 1:50 AM",
      dayLabel: "Wed",
      durationLabel: "2 hours 30 minutes",
      offsetHours: 5.33,
      durationHours: 2.5,
    },
    {
      presence: "asleep",
      observed: false,
      rangeLabel: "1:50 AM to 8:10 AM",
      dayLabel: "Thu",
      durationLabel: "6 hours 20 minutes",
      offsetHours: 7.83,
      durationHours: 6.33,
    },
  ],
  nextSleepLabel: "Wed, Aug 5, 11:20 PM to 10:40 AM",
  officeHoursLabel: "Typical office hours, Monday to Friday 9:00 AM to 5:00 PM",
  officeWindows: [
    {
      dayLabel: "Thu, Aug 6",
      hoursLabel: "9:00 AM to 5:00 PM",
      status: "reachable",
      reachableLabel: "10:40 AM to 5:00 PM",
      detail: "Predicted awake for 6 hours 20 minutes of this window.",
      offsetHours: 15,
      durationHours: 8,
    },
    {
      dayLabel: "Fri, Aug 7",
      hoursLabel: "9:00 AM to 5:00 PM",
      status: "partial",
      detail:
        "Possibly awake for up to 1 hour 10 minutes, but this falls where the sleep boundary is uncertain.",
      offsetHours: 39,
      durationHours: 8,
    },
  ],
  commitments: [
    {
      title: "Dentist",
      whenLabel: "Fri, Aug 7, 8:30 AM to 9:30 AM",
      conflict: "inside_predicted_sleep",
      conflictLabel: "Falls entirely inside predicted sleep",
      offsetHours: 38.5,
      durationHours: 1,
    },
  ],
  opportunities: [
    {
      taskId: "task-call",
      title: "Ring the pharmacy",
      whenLabel: "Thu, Aug 6, 10:40 AM to 11:00 AM",
      needsApproval: true,
    },
    {
      taskId: "task-forms",
      title: "Post the forms",
      unplacedLabel: "No window long enough in the next three days",
      needsApproval: true,
    },
  ],
  awakeLabel: "30 hours 0 minutes",
  uncertainLabel: "9 hours 0 minutes",
  disclaimer: "This application does not provide medical advice.",
};

describe("OutlookPanel", () => {
  it("draws all three presence states", () => {
    const { container } = render(<OutlookPanel data={base} />);
    const bands = container.querySelectorAll(".outlook-band");
    expect(bands).toHaveLength(3);
    expect(container.querySelector('.outlook-band[data-presence="uncertain"]')).not.toBeNull();
    expect(container.querySelector('.outlook-band[data-presence="asleep"]')).not.toBeNull();
  });

  // The day labels used to live inside .outlook-track, which clips its
  // overflow, so the strip had tick marks and nothing saying which day each one
  // began. They render in a sibling that is not clipped.
  it("puts the day labels outside the clipped track", () => {
    const { container } = render(<OutlookPanel data={base} />);
    const axis = container.querySelector(".outlook-axis");
    expect(axis).not.toBeNull();
    expect(axis?.closest(".outlook-track")).toBeNull();
    expect(axis?.textContent).toContain("Thu, Aug 6");
    expect(container.querySelector(".outlook-track")?.textContent).toBe("");
  });

  // Reachable hours and fixed events used to be separate lists restating days
  // the strip already showed. They are rows on the same axis now.
  it("draws reachable hours and events on the timeline's own axis", () => {
    const { container } = render(<OutlookPanel data={base} />);
    expect(container.querySelectorAll(".outlook-reach")).toHaveLength(base.officeWindows.length);
    const event = container.querySelector(".outlook-event") as HTMLElement | null;
    expect(event).not.toBeNull();
    expect(event?.style.left).toBe(`${(38.5 / base.horizonHours) * 100}%`);
    expect(container.querySelector(".outlook-office, .outlook-commitments")).toBeNull();
  });

  // The strip is a drawing; the same facts have to be readable without it, and
  // that costs the drawing nothing.
  it("states every stretch in words as well as in colour", () => {
    render(<OutlookPanel data={base} />);
    expect(screen.getByText(/Boundary uncertain, Wed 11:20 PM to 1:50 AM/)).toBeInTheDocument();
    expect(screen.getByText(/Likely asleep, Thu 1:50 AM to 8:10 AM/)).toBeInTheDocument();
  });

  // The distinction the reachable row exists for. "Possible" rests on a
  // boundary the model has not pinned down and must never be drawn or worded as
  // a time somebody could ring.
  it("never advertises a reachable time for a merely possible window", () => {
    const { container } = render(<OutlookPanel data={base} />);
    const partial = container.querySelector('.outlook-reach[data-status="partial"]');
    expect(partial).not.toBeNull();
    expect(partial?.getAttribute("title")).toMatch(/uncertain/);
    expect(partial?.getAttribute("title")).not.toMatch(/Awake /);

    const words = screen.getByText(/^Fri, Aug 7, .*uncertain/);
    expect(words.textContent).not.toMatch(/Predicted awake/);
    expect(screen.getByText(/^Thu, Aug 6, .*Predicted awake/)).toBeInTheDocument();
  });

  // The most useful thing this view can point at, so it is the one mark that
  // is not neutral, and it says why.
  it("flags a commitment that lands inside predicted sleep", () => {
    const { container } = render(<OutlookPanel data={base} />);
    const event = container.querySelector(".outlook-event");
    expect(event).toHaveAttribute("data-conflict", "true");
    expect(event?.getAttribute("title")).toContain("Falls entirely inside predicted sleep");
  });

  // Suggestions are decisions, and decisions live in Plan, where they can be
  // made. Listing them here as well was one of Home's three copies of them.
  it("does not list task suggestions", () => {
    render(<OutlookPanel data={base} />);
    expect(screen.queryByText("Ring the pharmacy")).toBeNull();
  });

  it("shows nothing but a reason when the view is withheld", () => {
    const withheld: OutlookData = {
      ...base,
      status: "withheld",
      segments: [],
      officeWindows: [],
      commitments: [],
      opportunities: [],
      withheldMessage: "Sleep was expected by now and none has been recorded.",
      freshness: {
        state: "withheld",
        reason: "expected_sleep_unrecorded",
        explanation: "Sleep was expected by now and none has been recorded.",
        trusted: false,
      },
    };
    const { container } = render(<OutlookPanel data={withheld} />);
    expect(container.querySelectorAll(".outlook-band")).toHaveLength(0);
    expect(screen.getByText(/not being shown/i)).toBeInTheDocument();
    expect(screen.getByText(/none has been recorded/i)).toBeInTheDocument();
  });

  it("explains a refusal rather than drawing an empty timeline", () => {
    const refused: OutlookData = {
      ...base,
      status: "refused",
      segments: [],
      officeWindows: [],
      commitments: [],
      opportunities: [],
      refusal: { code: "insufficient_data", message: "need at least 7 usable sleep episodes" },
    };
    const { container } = render(<OutlookPanel data={refused} />);
    expect(container.querySelectorAll(".outlook-band")).toHaveLength(0);
    expect(screen.getByText(/Not enough history/i)).toBeInTheDocument();
    expect(screen.getByText(/7 usable sleep episodes/)).toBeInTheDocument();
  });
});
