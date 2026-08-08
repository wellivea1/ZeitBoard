import { render, screen, within } from "@testing-library/react";
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

  // The strip is a drawing; the same facts have to be readable without it, and
  // that costs the drawing nothing.
  it("states every stretch in words as well as in colour", () => {
    render(<OutlookPanel data={base} />);
    expect(screen.getByText(/Boundary uncertain, Wed 11:20 PM to 1:50 AM/)).toBeInTheDocument();
    expect(screen.getByText(/Likely asleep, Thu 1:50 AM to 8:10 AM/)).toBeInTheDocument();
  });

  // The distinction the whole office section exists for. "Possible" rests on a
  // boundary the model has not pinned down and must never be shown as a time
  // somebody could ring.
  it("never advertises a reachable time for a merely possible window", () => {
    render(<OutlookPanel data={base} />);
    // Scoped to the office list: the day label also appears as a mark on the
    // timeline, and the two say different things.
    const office = within(screen.getByRole("region", { name: /office hours/i }));

    const partial = office.getByText("Fri, Aug 7").closest("li");
    expect(partial).not.toBeNull();
    expect(within(partial as HTMLElement).queryByText(/^Awake /)).toBeNull();
    expect(within(partial as HTMLElement).getByText(/uncertain/)).toBeInTheDocument();

    const reachable = office.getByText("Thu, Aug 6").closest("li");
    expect(within(reachable as HTMLElement).getByText(/Awake 10:40 AM/)).toBeInTheDocument();
  });

  it("names a commitment that lands inside predicted sleep", () => {
    render(<OutlookPanel data={base} />);
    expect(screen.getByText("Falls entirely inside predicted sleep")).toBeInTheDocument();
  });

  it("says a suggestion is only a suggestion", () => {
    render(<OutlookPanel data={base} />);
    expect(screen.getByText(/every placement needs your approval/i)).toBeInTheDocument();
    expect(screen.getByText("No window long enough in the next three days")).toBeInTheDocument();
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
    expect(screen.getByText(/anchored to where you are in your cycle/i)).toBeInTheDocument();
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
