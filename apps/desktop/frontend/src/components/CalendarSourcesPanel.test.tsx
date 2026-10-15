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

  // Adding the calendar again, address and all, was the only way to refresh it.
  it("refreshes the calendar written to at once, and any other at its address", async () => {
    const refresh = vi.fn(async () => ({
      sourceId: work.sourceId,
      label: "Work",
      kind: "caldav",
      readOnly: true,
      imported: true,
      eventCount: 3,
      busyCount: 2,
      allDayCount: 0,
      coverageStartAt: "2026-09-01T00:00:00Z",
      coverageEndAt: "2026-11-01T00:00:00Z",
      coverageLabel: "Sep 1 to Oct 31",
      previewTruncated: true,
      events: [],
      message: "Imported 3 events, 2 busy.",
    }));
    const onChanged = vi.fn();
    const home: CalendarSource = {
      ...work,
      sourceId: "calendar_source_caldav_home",
      label: "Home",
      endpoint: "https://calendar.example/dav/owner/home/",
    };
    (globalThis as { go?: unknown }).go = {
      main: {
        App: { GetCalendarWriteBack: vi.fn(async () => writing), RefreshCalendarSource: refresh },
      },
    };
    render(<CalendarSourcesPanel sources={[work, home]} available onChanged={onChanged} />);

    // The calendar written to keeps its sign-in.
    fireEvent.click(await screen.findByRole("button", { name: "Refresh Work" }));
    expect(await screen.findByText("Work: Imported 3 events, 2 busy.")).toBeVisible();
    expect(refresh).toHaveBeenCalledWith({ sourceId: work.sourceId });
    expect(onChanged).toHaveBeenCalledOnce();

    // Any other opens the form at its address, for its sign-in.
    fireEvent.click(screen.getByRole("button", { name: "Refresh Home" }));
    const form = screen.getByRole("region", { name: "Refresh Home" });
    expect(form).toContainElement(screen.getByDisplayValue(home.endpoint!));
    expect(screen.getByDisplayValue("Home")).toBeInTheDocument();
    expect(refresh).toHaveBeenCalledOnce();
  });
});

// Calendars ZeitBoard can read again by itself refresh at once from their
// rows, and say where they come from.
describe("CalendarSourcesPanel fetched calendars", () => {
  // A subscribed calendar names its host, never its address, says how its
  // last reading went, and refreshes at once from its row.
  it("refreshes a subscribed calendar from its address at once", async () => {
    const refresh = vi.fn(async () => ({
      sourceId: "calendar_source_subscription_work",
      label: "Work",
      kind: "ics",
      readOnly: true,
      imported: true,
      eventCount: 3,
      busyCount: 3,
      allDayCount: 0,
      coverageStartAt: "2026-01-01T00:00:00Z",
      coverageEndAt: "2027-01-01T00:00:00Z",
      coverageLabel: "Jan 1, 2026 to Jan 1, 2027",
      previewTruncated: false,
      events: [],
      message: "Imported 3 calendar events.",
    }));
    (globalThis as { go?: unknown }).go = {
      main: {
        App: { RefreshCalendarSource: refresh, GetCalendarWriteBack: async () => ({ on: false }) },
      },
    };
    const subscribed: CalendarSource = {
      sourceId: "calendar_source_subscription_work",
      label: "Work",
      kind: "ics",
      readOnly: true,
      visibleEvents: 3,
      coverageLabel: "Jan 1 to Dec 31",
      coverageStart: "2026-01-01T00:00:00Z",
      coverageEnd: "2027-01-01T00:00:00Z",
      subscription: {
        host: "calendar.example.test",
        refreshedLabel: "10:42 AM",
        failure: "calendar.example.test answered HTTP 503.",
      },
    };
    render(<CalendarSourcesPanel sources={[subscribed]} available onChanged={vi.fn()} />);

    expect(screen.getByText(/^Subscribed/)).toBeVisible();
    expect(
      screen.getByText("From calendar.example.test, read 10:42 AM and every hour"),
    ).toBeVisible();
    expect(
      screen.getByText("calendar.example.test answered HTTP 503. The copy read 10:42 AM is kept."),
    ).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Refresh Work" }));
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(screen.queryByLabelText("Collection URL")).toBeNull();
  });

  // A calendar from an account takes accepted times with the account's own
  // sign-in: nothing is typed, the browser asks once, and stopping leaves the
  // account signed in for reading (ADR-0065).
  // An account that already lets ZeitBoard add events asks nothing in the
  // browser, so there is no sign-in to cancel while writing is turned on.
  it("turns writing on without the browser when the account already allows it", async () => {
    let finish: (status: unknown) => void = () => undefined;
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetCalendarWriteBack: async () => ({
            on: false,
            summary: "Accepted times stay in ZeitBoard.",
            problems: [],
          }),
          EnableAccountWriteBack: vi.fn(
            () =>
              new Promise((resolve) => {
                finish = resolve;
              }),
          ),
        },
      },
    };
    const owner: CalendarSource = {
      sourceId: "calendar_source_account_owner",
      label: "Owner",
      kind: "ics",
      readOnly: true,
      visibleEvents: 0,
      coverageLabel: "Oct 12, 2025 to Oct 12, 2028",
      coverageStart: "2025-10-12T00:00:00Z",
      coverageEnd: "2028-10-12T00:00:00Z",
      account: {
        provider: "microsoft",
        accountLabel: "owner@example.test",
        refreshedLabel: "09:42",
        writes: true,
      },
    };
    render(<CalendarSourcesPanel sources={[owner]} available onChanged={vi.fn()} />);

    fireEvent.click(await screen.findByRole("button", { name: "Write accepted times to Owner" }));
    expect(
      screen.getByText("Microsoft already lets ZeitBoard add events to your calendars."),
    ).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Allow writing" }));
    expect(
      await screen.findByText("Checking that ZeitBoard can add events to Owner…"),
    ).toBeVisible();
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
    finish({
      on: true,
      sourceId: owner.sourceId,
      label: "Owner",
      provider: "microsoft",
      summary: "The next time you accept is added to Owner.",
      problems: [],
    });
    expect(
      await screen.findByRole("region", { name: "Accepted times are written here" }),
    ).toBeVisible();
  });

  it("writes accepted times to a calendar from an account once the account allows it", async () => {
    const owner: CalendarSource = {
      sourceId: "calendar_source_account_owner",
      label: "Owner",
      kind: "ics",
      readOnly: true,
      visibleEvents: 0,
      coverageLabel: "Oct 12, 2025 to Oct 12, 2028",
      coverageStart: "2025-10-12T00:00:00Z",
      coverageEnd: "2028-10-12T00:00:00Z",
      account: { provider: "google", accountLabel: "owner@example.test", refreshedLabel: "09:42" },
    };
    const writingThere: CalendarWriteBack = {
      on: true,
      sourceId: owner.sourceId,
      label: "Owner",
      provider: "google",
      summary: "The next time you accept is added to Owner.",
      problems: [],
    };
    const pending: { refuse?: (reason: string) => void } = {};
    const enable = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            pending.refuse = reject;
          }),
      )
      .mockResolvedValueOnce(writingThere);
    const cancel = vi.fn(async () => pending.refuse?.("signing in was cancelled"));
    (globalThis as { go?: unknown }).go = {
      main: {
        App: {
          GetCalendarWriteBack: async () => ({
            on: false,
            summary: "Accepted times stay in ZeitBoard.",
            problems: [],
          }),
          EnableAccountWriteBack: enable,
          CancelCalendarSignIn: cancel,
        },
      },
    };
    render(<CalendarSourcesPanel sources={[owner]} available onChanged={vi.fn()} />);

    fireEvent.click(await screen.findByRole("button", { name: "Write accepted times to Owner" }));
    const offer = screen.getByRole("region", { name: "Write accepted times to Owner" });
    expect(offer).toHaveTextContent(
      "Google asks in your browser whether ZeitBoard may add events to calendars you own.",
    );
    expect(screen.queryByLabelText("App password")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Allow writing" }));
    expect(
      await screen.findByText("If your browser asks, allow ZeitBoard to add events."),
    ).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Signing in was cancelled.");
    expect(cancel).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: "Allow writing" }));
    const status = await screen.findByRole("region", { name: "Accepted times are written here" });
    expect(status).toHaveTextContent("The next time you accept is added to Owner.");
    expect(enable).toHaveBeenLastCalledWith({ sourceId: owner.sourceId });
    fireEvent.click(screen.getByRole("button", { name: "Stop writing" }));
    expect(screen.getByText(/The Google account stays signed in for reading/)).toBeVisible();
  });

  // A calendar from an account names the account, says how its last reading
  // went, and refreshes at once with the account's sign-in (ADR-0064).
  it("refreshes a calendar from an account at once and names the account", async () => {
    const refresh = vi.fn(async () => ({
      sourceId: "calendar_source_account_work",
      label: "Team",
      kind: "ics",
      readOnly: true,
      imported: true,
      eventCount: 2,
      busyCount: 2,
      allDayCount: 0,
      coverageStartAt: "2026-01-01T00:00:00Z",
      coverageEndAt: "2027-01-01T00:00:00Z",
      coverageLabel: "Jan 1, 2026 to Jan 1, 2027",
      previewTruncated: false,
      events: [],
      message: "Imported 2 calendar events.",
    }));
    (globalThis as { go?: unknown }).go = {
      main: {
        App: { RefreshCalendarSource: refresh, GetCalendarWriteBack: async () => ({ on: false }) },
      },
    };
    const fromAccount: CalendarSource = {
      sourceId: "calendar_source_account_work",
      label: "Team",
      kind: "ics",
      readOnly: true,
      visibleEvents: 2,
      coverageLabel: "Jan 1 to Dec 31",
      coverageStart: "2026-01-01T00:00:00Z",
      coverageEnd: "2027-01-01T00:00:00Z",
      account: {
        provider: "microsoft",
        accountLabel: "owner@example.test",
        refreshedLabel: "10:42 AM",
        failure: "graph.microsoft.com answered HTTP 503.",
      },
    };
    render(<CalendarSourcesPanel sources={[fromAccount]} available onChanged={vi.fn()} />);

    expect(screen.getByText(/^Microsoft account/)).toBeVisible();
    expect(screen.getByText("From owner@example.test, read 10:42 AM and every hour")).toBeVisible();
    expect(
      screen.getByText("graph.microsoft.com answered HTTP 503. The copy read 10:42 AM is kept."),
    ).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Refresh Team" }));
    await waitFor(() => expect(refresh).toHaveBeenCalledWith({ sourceId: fromAccount.sourceId }));
    expect(screen.queryByLabelText("Collection URL")).toBeNull();
  });
});
