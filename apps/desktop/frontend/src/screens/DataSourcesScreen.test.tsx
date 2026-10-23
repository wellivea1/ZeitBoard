import { render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { calendarDataChangedEvent } from "../data/calendar";
import { DataSourcesScreen } from "./DataSourcesScreen";

// A computer away from New York, which the screen once assumed for everyone.
vi.mock("../utils/civilTime", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils/civilTime")>()),
  localZone: () => "Europe/London",
}));

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("DataSourcesScreen structure", () => {
  // After slice U-H this screen is about where records come from. Recording
  // last night is Log's job, and the two were only sharing a screen because
  // they had both grown there.
  it("leads with what is connected, then calendars, then importing", () => {
    render(<DataSourcesScreen />);
    const workspace = screen.getByRole("region", { name: "Data source review" });
    const headings = ["Connected", "Calendars", "Import sleep records"].map((name) =>
      screen.getByRole("heading", { name }),
    );

    expect(workspace.querySelector(".panel")).toBeNull();
    for (const [index, heading] of headings.slice(1).entries()) {
      expect(
        headings[index]!.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).not.toBe(0);
    }
  });

  // Calendars used to be added beside the calendar board in Plan. They are
  // set up here with the other inputs, and the board links to them.
  it("lists calendars here instead of in Plan", async () => {
    render(<DataSourcesScreen />);
    const calendars = screen.getByRole("region", { name: "Calendars" });
    expect(await within(calendars).findByText("Sample commitments")).toBeVisible();
  });

  // Found while paging the log: the screen started as "empty", so until the
  // count arrived it said there were no sleep records.
  it("does not claim there are no sleep records before it has counted them", async () => {
    let answer!: (value: unknown) => void;
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetSleepSources: () => new Promise((resolve) => (answer = resolve)),
        },
      },
    };
    render(<DataSourcesScreen />);
    const connected = screen.getByRole("region", { name: "Data source review" });
    expect(within(connected).queryByText(/None yet/)).toBeNull();

    answer({
      status: "ready",
      message: "3 local sleep entries stored on this device.",
      total: 3,
      correctedCount: 0,
      suppressedCount: 0,
      sources: [
        {
          source: "Manual sleep log",
          provenance: "manual / user reported",
          total: 3,
          corrected: 0,
          suppressed: 0,
        },
      ],
    });
    expect(await within(connected).findByText(/3 records/)).toBeVisible();
  });

  // Calendars were read, and an added file's floating times placed, in New
  // York's zone whatever this computer's was.
  it("reads calendars in this computer's zone", async () => {
    const getCalendar = vi.fn(async () => ({}));
    (globalThis as { go?: unknown }).go = { main: { App: { GetCalendar: getCalendar } } };
    render(<DataSourcesScreen />);
    await waitFor(() => expect(getCalendar).toHaveBeenCalled());
    expect(getCalendar).toHaveBeenCalledWith(expect.objectContaining({ zoneId: "Europe/London" }));
  });

  // What the desktop answers for a day, holding the calendars named.
  const calendarWith = (labels: string[]) => ({
    status: "empty",
    message: "No calendar events in this range.",
    fixtureMode: false,
    zoneId: "Europe/London",
    startCivilDate: "2026-10-23",
    endCivilDate: "2026-10-23",
    updatedLabel: "Updated now",
    sources: labels.map((label, index) => ({
      sourceId: `calendar_source_${index}`,
      label,
      kind: "ics",
      readOnly: true,
      visibleEvents: 0,
      coverageLabel: "Oct 1 to Nov 1",
      coverageStart: "2026-10-01T00:00:00Z",
      coverageEnd: "2026-11-01T00:00:00Z",
    })),
    days: [
      {
        civilDate: "2026-10-23",
        label: "Fri, Oct 23",
        isToday: true,
        events: [],
        predictions: [],
      },
    ],
    warnings: [],
  });

  // A calendar removed while the screen was reading came back when that older
  // read answered last. Its answer is dropped, and the screen reads again.
  it("lets only the newest read of the calendars land", async () => {
    const answers: ((value: unknown) => void)[] = [];
    (globalThis as { go?: unknown }).go = {
      main: { App: { GetCalendar: () => new Promise((resolve) => answers.push(resolve)) } },
    };
    render(<DataSourcesScreen />);
    await waitFor(() => expect(answers).toHaveLength(1));
    window.dispatchEvent(new Event(calendarDataChangedEvent));
    answers[0]!(calendarWith(["Work", "Removed since"]));

    await waitFor(() => expect(answers).toHaveLength(2));
    const calendars = screen.getByRole("region", { name: "Calendars" });
    expect(within(calendars).queryByText("Removed since")).toBeNull();
    answers[1]!(calendarWith(["Work"]));
    expect(await within(calendars).findByText("Work")).toBeVisible();
    expect(within(calendars).queryByText("Removed since")).toBeNull();
  });

  // Both lists said there were no calendars while they were still being read,
  // and went on saying it when the read failed.
  it("says nothing of the calendars before they are read", async () => {
    let answer!: (value: unknown) => void;
    (globalThis as { go?: unknown }).go = {
      main: { App: { GetCalendar: () => new Promise((resolve) => (answer = resolve)) } },
    };
    render(<DataSourcesScreen />);
    const calendars = screen.getByRole("region", { name: "Calendars" });
    expect(within(calendars).queryByText(/No calendars yet/)).toBeNull();
    expect(screen.queryByRole("heading", { name: "Calendars", level: 3 })).toBeNull();

    answer(calendarWith([]));
    expect(await within(calendars).findByText(/No calendars yet/)).toBeVisible();
    expect(
      screen.getByRole("heading", { name: "Calendars", level: 3 }).closest("article"),
    ).toHaveTextContent("None added");
  });

  it("says when the calendars could not be read", async () => {
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetCalendar: async () => {
            throw new Error("The calendar store could not be opened.");
          },
        },
      },
    };
    render(<DataSourcesScreen />);
    const calendars = screen.getByRole("region", { name: "Calendars" });
    expect(await within(calendars).findByText("The calendars could not be read.")).toBeVisible();
    expect(within(calendars).queryByText(/No calendars yet/)).toBeNull();
    const row = screen.getByRole("heading", { name: "Calendars", level: 3 }).closest("article");
    expect(row).toHaveTextContent("They could not be read.");
    expect(row).not.toHaveTextContent("None added");
  });

  // The row read "Not connected" whatever Settings said.
  it("says what desktop activity is doing", async () => {
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetActivityCollection: async () => ({
            enabled: true,
            running: true,
            supported: true,
            zoneId: "Europe/London",
            recordCount: 14,
            lastError: "",
          }),
        },
      },
    };
    render(<DataSourcesScreen />);
    const row = (await screen.findByRole("heading", { name: "Desktop activity" })).closest(
      "article",
    );
    expect(row).toHaveTextContent(
      "Recording when this computer is active, idle or locked. 14 records kept.",
    );
    expect(row).toHaveTextContent("Running");
  });

  // Somebody who used to add an entry here must be told where it went rather
  // than left hunting.
  it("says where the sleep log went", () => {
    render(<DataSourcesScreen />);
    expect(screen.queryByRole("heading", { name: "Add sleep entry" })).toBeNull();
    expect(screen.getByRole("link", { name: "Log" })).toHaveAttribute("href", "#/log/sleep");
  });
});
