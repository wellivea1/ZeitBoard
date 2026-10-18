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
  reaching: [
    {
      contactId: "contact-office",
      label: "Typical office hours",
      hoursLabel: "Typical office hours: you set Monday to Friday, 9:00 AM to 5:00 PM.",
      windows: [
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

  // A day beginning in the strip's last hours ran its label past the window's
  // edge, and one beginning just after now ran its label into "Now". Each label
  // now has only its own stretch, and the stylesheet leaves off a label its
  // stretch cannot hold.
  it("gives each axis label only its own stretch of the strip", () => {
    const days = [
      { label: "Thu, Aug 6", offsetHours: 0.5 },
      { label: "Fri, Aug 7", offsetHours: 24.5 },
      { label: "Sat, Aug 8", offsetHours: 48.5 },
    ];
    const { container } = render(<OutlookPanel data={{ ...base, horizonHours: 50, days }} />);
    const rooms = [...container.querySelectorAll<HTMLElement>(".outlook-axis-room")].map(
      (room) => ({
        label: room.textContent,
        from: parseFloat(room.style.left),
        to: parseFloat(room.style.left) + parseFloat(room.style.width),
      }),
    );
    expect(rooms.map((room) => room.label)).toEqual(["Now", ...days.map((day) => day.label)]);
    expect(rooms[0]?.from).toBe(0);
    rooms.forEach((room, index) => {
      expect(room.to).toBeCloseTo(rooms[index + 1]?.from ?? 100);
    });
  });

  // Reachable hours and fixed events used to be separate lists restating days
  // the strip already showed. They are rows on the same axis now.
  it("draws reachable hours and events on the timeline's own axis", () => {
    const { container } = render(<OutlookPanel data={base} />);
    expect(container.querySelectorAll(".outlook-reach")).toHaveLength(
      base.reaching[0]!.windows.length,
    );
    const event = container.querySelector(".outlook-event") as HTMLElement | null;
    expect(event).not.toBeNull();
    expect(event?.style.left).toBe(`${(38.5 / base.horizonHours) * 100}%`);
    expect(container.querySelector(".outlook-office, .outlook-commitments")).toBeNull();
  });

  // Each person on the reaching list gets a row of their own, named for them,
  // in the order of the list; someone with no hours in the days shown keeps
  // theirs, and the words say why it is empty.
  it("gives each person shown a row of their own", () => {
    const pharmacy = {
      contactId: "contact-pharmacy",
      label: "The pharmacy",
      hoursLabel: "The pharmacy: you set Sunday, 10:00 AM to 4:00 PM.",
      windows: [],
    };
    const { container } = render(
      <OutlookPanel data={{ ...base, reaching: [...base.reaching, pharmacy] }} />,
    );
    const names = [...container.querySelectorAll(".outlook-row-name")];
    expect(names.map((name) => name.textContent)).toEqual(["Typical office hours", "The pharmacy"]);
    expect(names[1]).toHaveAttribute("title", pharmacy.hoursLabel);
    const pharmacyTrack = names[1]?.nextElementSibling;
    expect(pharmacyTrack?.querySelectorAll(".outlook-reach")).toHaveLength(0);
    expect(
      screen.getByText("The pharmacy is not available in the days shown."),
    ).toBeInTheDocument();
    expect(screen.getByRole("list", { name: pharmacy.hoursLabel })).toBeInTheDocument();
  });

  it("draws no reaching row or legend when nobody is shown", () => {
    const { container } = render(<OutlookPanel data={{ ...base, reaching: [] }} />);
    expect(container.querySelector(".outlook-row-name")).toBeNull();
    expect(container.querySelector('[data-legend="reach"]')).toBeNull();
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
      reaching: [],
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
    expect(screen.getByText(/not drawn/i)).toBeInTheDocument();
    // Home's lead sentence gives the reason; the figure says what brings it back.
    expect(screen.queryByText(/none has been recorded/i)).toBeNull();
    expect(screen.getByRole("link", { name: "Log sleep" })).toHaveAttribute("href", "#/log/sleep");
  });

  it("explains a refusal rather than drawing an empty timeline", () => {
    const refused: OutlookData = {
      ...base,
      status: "refused",
      segments: [],
      reaching: [],
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
