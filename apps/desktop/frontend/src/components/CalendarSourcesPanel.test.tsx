import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { CalendarSource } from "../data/calendar";
import { calendarWriteBackChangedEvent, type CalendarWriteBack } from "../data/calendarWriteBack";
import { clockTime, dayInSentence, relativeDay } from "../utils/relativeTime";
import { CalendarSourcesPanel } from "./CalendarSourcesPanel";

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("CalendarSourcesPanel", () => {
  it("requires typed confirmation before erasing an imported snapshot", async () => {
    const remove = vi.fn(async () => undefined);
    const onChanged = vi.fn();
    (globalThis as { go?: unknown }).go = {
      main: { App: { RemoveCalendarSource: remove } },
    };
    const sources: CalendarSource[] = [
      {
        sourceId: "calendar_source_imported",
        label: "Commitments",
        kind: "ics",
        readOnly: true,
        visibleEvents: 2,
        coverageLabel: "Jan 1 to Dec 31",
        coverageStart: "2026-01-01T00:00:00Z",
        coverageEnd: "2027-01-01T00:00:00Z",
      },
      {
        sourceId: "calendar_source_zeitboard",
        label: "ZeitBoard placements",
        kind: "zeitboard",
        readOnly: false,
        visibleEvents: 1,
        coverageLabel: "Local placements",
        coverageStart: "2026-01-01T00:00:00Z",
        coverageEnd: "2027-01-01T00:00:00Z",
      },
    ];
    render(<CalendarSourcesPanel sources={sources} available onChanged={onChanged} />);

    expect(screen.getAllByRole("button", { name: /^Remove/ })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Remove Commitments" }));
    const erase = screen.getByRole("button", { name: "Remove calendar" });
    expect(erase).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Type REMOVE/), { target: { value: "REMOVE" } });
    fireEvent.click(erase);

    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(remove).toHaveBeenCalledWith({
      sourceId: "calendar_source_imported",
      confirmation: "REMOVE",
    });
  });

  const work: CalendarSource = {
    sourceId: "calendar_source_caldav_work",
    label: "Work",
    kind: "caldav",
    readOnly: true,
    endpoint: "https://calendar.example/dav/owner/work/",
    visibleEvents: 4,
    coverageLabel: "Sep 1 to Oct 31",
    coverageStart: "2026-09-01T00:00:00Z",
    coverageEnd: "2026-11-01T00:00:00Z",
  };
  const file: CalendarSource = {
    sourceId: "calendar_source_file",
    label: "Holidays",
    kind: "ics",
    readOnly: true,
    visibleEvents: 2,
    coverageLabel: "Jan 1 to Dec 31",
    coverageStart: "2026-01-01T00:00:00Z",
    coverageEnd: "2027-01-01T00:00:00Z",
  };
  const off: CalendarWriteBack = {
    on: false,
    summary: "Accepted times stay in ZeitBoard.",
    problems: [],
  };
  const writing: CalendarWriteBack = {
    on: true,
    sourceId: work.sourceId,
    label: "Work",
    username: "owner",
    summary: "2 accepted times are in Work.",
    problems: [],
  };

  it("writes accepted times to a CalDAV calendar once its sign-in is shown to work", async () => {
    const enable = vi
      .fn()
      .mockRejectedValueOnce(new Error("Your calendar did not accept this sign-in."))
      .mockResolvedValueOnce(writing);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: { GetCalendarWriteBack: vi.fn(async () => off), EnableCalendarWriteBack: enable },
      },
    };
    render(<CalendarSourcesPanel sources={[file, work]} available onChanged={vi.fn()} />);

    // Only a CalDAV calendar can be written to.
    const offer = await screen.findByRole("button", { name: "Write accepted times to Work" });
    expect(screen.queryByRole("button", { name: "Write accepted times to Holidays" })).toBeNull();
    fireEvent.click(offer);
    const form = screen.getByRole("form", { name: "Write accepted times to Work" });
    expect(form).toHaveTextContent("Your own events are never changed.");

    fireEvent.change(screen.getByLabelText("Username"), { target: { value: " owner " } });
    fireEvent.change(screen.getByLabelText("App password"), {
      target: { value: "synthetic-app-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Start writing" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Your calendar did not accept this sign-in.",
    );
    expect(screen.getByLabelText("App password")).toHaveValue("");

    fireEvent.change(screen.getByLabelText("App password"), {
      target: { value: "synthetic-app-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Start writing" }));
    const status = await screen.findByRole("region", { name: "Accepted times are written here" });
    expect(status).toHaveTextContent("2 accepted times are in Work.");
    expect(enable).toHaveBeenLastCalledWith({
      sourceId: work.sourceId,
      username: "owner",
      password: "synthetic-app-password",
    });
    expect(screen.queryByRole("button", { name: "Write accepted times to Work" })).toBeNull();
  });

  it("leaves what the owner changed for them, and stops only when asked twice", async () => {
    const now = new Date();
    const start = new Date(now.getTime() + 26 * 3_600_000);
    const nextTry = new Date(now.getTime() + 5 * 60_000);
    const conflicted: CalendarWriteBack = {
      ...writing,
      problems: [
        {
          eventId: "calendar_event_changed",
          title: "Call the pharmacy",
          startAt: start.toISOString(),
          detail: "It was changed in your calendar after ZeitBoard wrote it.",
          conflict: true,
          removing: true,
        },
        {
          eventId: "calendar_event_waiting",
          title: "Grocery run",
          startAt: start.toISOString(),
          detail: "Your calendar's server had a problem (HTTP 503).",
          conflict: false,
          removing: false,
          retryAt: nextTry.toISOString(),
        },
      ],
    };
    const settled = { ...writing, problems: [conflicted.problems[1]] };
    const load = vi.fn(async () => conflicted);
    const resolve = vi.fn(async () => settled);
    const retry = vi.fn(async () => writing);
    const disable = vi.fn(async () => off);
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetCalendarWriteBack: load,
          ResolveCalendarWriteConflict: resolve,
          RetryCalendarWrites: retry,
          DisableCalendarWriteBack: disable,
        },
      },
    };
    render(<CalendarSourcesPanel sources={[work]} available onChanged={vi.fn()} />);

    const problems = await screen.findByRole("list", { name: "Not written yet" });
    expect(problems).toHaveTextContent("It was changed in your calendar after ZeitBoard wrote it.");
    // Worded as Plan words times: the day relative to today, then the clock.
    expect(problems).toHaveTextContent(`${relativeDay(start, now)} ${clockTime(start)}`);
    expect(problems).toHaveTextContent(
      `Trying again ${dayInSentence(nextTry, now)} at ${clockTime(nextTry)}.`,
    );
    fireEvent.click(screen.getByRole("button", { name: "Remove it anyway" }));
    await waitFor(() =>
      expect(resolve).toHaveBeenCalledWith({
        eventId: "calendar_event_changed",
        decision: "remove",
      }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Keep it in the calendar" })).toBeNull(),
    );

    fireEvent.click(screen.getByRole("button", { name: "Try again now" }));
    await waitFor(() => expect(retry).toHaveBeenCalledOnce());

    // The writer's announcement brings the status up to date.
    window.dispatchEvent(new Event(calendarWriteBackChangedEvent));
    await waitFor(() => expect(load).toHaveBeenCalledTimes(2));

    fireEvent.click(await screen.findByRole("button", { name: "Stop writing" }));
    expect(disable).not.toHaveBeenCalled();
    expect(screen.getByRole("group", { name: /^Stop writing to Work\?/ })).toHaveTextContent(
      "undoing an accepted time will no longer remove it",
    );
    fireEvent.click(screen.getByRole("button", { name: "Stop writing" }));
    await waitFor(() => expect(disable).toHaveBeenCalledOnce());
    expect(
      await screen.findByRole("button", { name: "Write accepted times to Work" }),
    ).toBeInTheDocument();
  });
});
