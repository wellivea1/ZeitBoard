import { describe, expect, it, vi } from "vitest";

import { normalizeCalendar } from "./calendar";
import {
  addAccountCalendar,
  canConnectCalendarAccounts,
  connectCalendarAccount,
  loadCalendarAccounts,
  normalizeAccountCalendars,
  normalizeCalendarAccount,
  removeCalendarAccount,
} from "./calendarAccounts";

const account = {
  accountId: "calendar_account_test",
  provider: "google",
  label: "owner@example.test",
  connectedLabel: "Signed in Oct 12, 2026",
  calendars: 1,
};

const calendars = {
  account,
  calendars: [
    {
      calendarId: "owner@example.test",
      name: "Owner",
      primary: true,
      sourceId: "calendar_source_a",
    },
    { calendarId: "team@group.example.test", name: "Team", primary: false },
  ],
};

const report = {
  sourceId: "calendar_source_a",
  label: "Owner",
  kind: "ics",
  readOnly: true,
  imported: true,
  eventCount: 1,
  busyCount: 1,
  allDayCount: 0,
  coverageStartAt: "2025-10-12T00:00:00Z",
  coverageEndAt: "2028-10-12T00:00:00Z",
  coverageLabel: "Oct 12, 2025 to Oct 12, 2028",
  previewTruncated: false,
  events: [
    {
      eventId: "calendar_event_account_dentist",
      title: "Dentist",
      startLabel: "Oct 14, 2026, 3:00 PM EDT",
      endLabel: "Oct 14, 2026, 4:00 PM EDT",
      allDay: false,
      busy: true,
    },
  ],
  message: "Imported 1 calendar event.",
};

function desktop(methods: Record<string, unknown>) {
  return { go: { main: { App: methods } } };
}

describe("calendar accounts adapter", () => {
  it("reads an account and its calendars, and refuses what is malformed", () => {
    expect(normalizeCalendarAccount(account)).toEqual({ ...account, writing: false });
    expect(normalizeCalendarAccount({ ...account, writing: true })?.writing).toBe(true);
    expect(
      normalizeCalendarAccount({ ...account, problem: "The sign-in has ended." }),
    ).toMatchObject({
      problem: "The sign-in has ended.",
    });
    expect(normalizeCalendarAccount({ ...account, provider: "fastmail" })).toBeUndefined();
    expect(normalizeCalendarAccount({ ...account, calendars: -1 })).toBeUndefined();
    expect(normalizeAccountCalendars(calendars)?.calendars).toEqual([
      {
        calendarId: "owner@example.test",
        name: "Owner",
        primary: true,
        sourceId: "calendar_source_a",
      },
      { calendarId: "team@group.example.test", name: "Team", primary: false },
    ]);
    expect(
      normalizeAccountCalendars({ ...calendars, calendars: [{ calendarId: "x", primary: true }] }),
    ).toBeUndefined();
  });

  it("calls the desktop with what the owner chose, and needs it", async () => {
    expect(canConnectCalendarAccounts({})).toBe(false);
    await expect(loadCalendarAccounts({})).resolves.toEqual([]);
    const connect = vi.fn(async () => calendars);
    const add = vi.fn(async () => report);
    const remove = vi.fn(async () => ({ message: "owner@example.test is removed." }));
    const root = desktop({
      ConnectCalendarAccount: connect,
      GetCalendarAccounts: async () => [account],
      AddAccountCalendar: add,
      RemoveCalendarAccount: remove,
    });

    const input = {
      provider: "microsoft" as const,
      clientId: "synthetic-client",
      clientSecret: "",
    };
    await expect(connectCalendarAccount(input, root)).resolves.toMatchObject({ account });
    expect(connect).toHaveBeenCalledWith(input);
    await expect(loadCalendarAccounts(root)).resolves.toEqual([{ ...account, writing: false }]);
    const chosen = {
      accountId: account.accountId,
      calendarId: "owner@example.test",
      label: "Owner",
      zoneId: "UTC",
    };
    await expect(addAccountCalendar(chosen, root)).resolves.toMatchObject({ imported: true });
    expect(add).toHaveBeenCalledWith(chosen);
    await expect(removeCalendarAccount(account.accountId, root)).resolves.toBe(
      "owner@example.test is removed.",
    );
    expect(remove).toHaveBeenCalledWith({ accountId: account.accountId, confirmation: "REMOVE" });
  });

  it("keeps the account a calendar came from only on an imported calendar", () => {
    const from = {
      provider: "microsoft",
      accountLabel: "owner@example.test",
      refreshedLabel: "9:00",
      failure: "The sign-in to owner@example.test has ended.",
    };
    const source = {
      sourceId: "calendar_source_account",
      label: "Work",
      kind: "ics",
      readOnly: true,
      visibleEvents: 0,
      coverageLabel: "Oct 12 to Oct 12",
      coverageStart: "2025-10-12T00:00:00Z",
      coverageEnd: "2028-10-12T00:00:00Z",
      account: from,
    };
    const calendar = {
      status: "empty",
      message: "No calendar events in this range.",
      fixtureMode: false,
      zoneId: "UTC",
      startCivilDate: "2026-10-12",
      endCivilDate: "2026-10-12",
      updatedLabel: "Updated Oct 12",
      sources: [source],
      days: [
        {
          civilDate: "2026-10-12",
          label: "Mon, Oct 12",
          isToday: true,
          events: [],
          predictions: [],
        },
      ],
      warnings: [],
    };
    expect(normalizeCalendar(calendar)?.sources[0]?.account).toEqual(from);
    expect(
      normalizeCalendar({
        ...calendar,
        sources: [{ ...source, account: { ...from, provider: "x" } }],
      })?.sources[0]?.account,
    ).toBeUndefined();
  });
});
