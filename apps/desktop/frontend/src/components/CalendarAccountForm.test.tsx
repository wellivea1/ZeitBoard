import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CalendarImportPanel } from "./CalendarImportPanel";

const account = {
  accountId: "calendar_account_test",
  provider: "google",
  label: "owner@example.test",
  connectedLabel: "Signed in Oct 12, 2026",
  calendars: 0,
};

const signedIn = {
  account,
  calendars: [
    { calendarId: "owner@example.test", name: "Owner", primary: true },
    { calendarId: "team@group.example.test", name: "Team", primary: false },
  ],
};

const report = {
  sourceId: "calendar_source_account_owner",
  label: "Owner",
  kind: "ics",
  readOnly: true,
  imported: false,
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
  message: "Previewed 1 calendar event; 1 blocks scheduling.",
};

function desktop(methods: Record<string, unknown>) {
  (globalThis as { go?: unknown }).go = { main: { App: methods } };
}

function openAccountTab(onChanged = vi.fn()) {
  render(<CalendarImportPanel available zoneId="America/New_York" onChanged={onChanged} />);
  fireEvent.click(screen.getByRole("tab", { name: "Google or Microsoft" }));
  return onChanged;
}

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("CalendarAccountForm", () => {
  // Signing in takes the owner's own registration; the account's calendars
  // follow, each previewed before it is brought in.
  it("signs in to a Google account and brings a calendar in after a preview", async () => {
    let accounts: unknown[] = [];
    const connect = vi.fn(async () => {
      accounts = [account];
      return signedIn;
    });
    const preview = vi.fn(async () => report);
    const add = vi.fn(async () => ({ ...report, imported: true }));
    desktop({
      GetCalendarAccounts: async () => accounts,
      ConnectCalendarAccount: connect,
      PreviewAccountCalendar: preview,
      AddAccountCalendar: add,
    });
    const onChanged = openAccountTab();

    const signIn = screen.getByRole("button", { name: "Sign in with Google" });
    expect(signIn).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Client ID"), {
      target: { value: " synthetic-client.apps.example.test " },
    });
    expect(signIn).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Client secret"), {
      target: { value: "synthetic-desktop-secret" },
    });
    fireEvent.click(signIn);

    expect(await screen.findByRole("button", { name: "Preview Owner" })).toBeVisible();
    expect(connect).toHaveBeenCalledWith({
      provider: "google",
      clientId: "synthetic-client.apps.example.test",
      clientSecret: "synthetic-desktop-secret",
    });
    // With an account signed in, signing in to another waits behind a button.
    expect(screen.queryByLabelText("Client secret")).toBeNull();
    expect(screen.getByText("Main calendar")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Preview Owner" }));
    expect(await screen.findByText(report.message)).toBeVisible();
    expect(preview).toHaveBeenCalledWith({
      accountId: account.accountId,
      calendarId: "owner@example.test",
      label: "Owner",
      zoneId: "America/New_York",
    });
    fireEvent.click(screen.getByRole("button", { name: "Bring in Owner" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(add).toHaveBeenCalledOnce();
    expect(screen.getByText("Brought in · read every hour")).toBeVisible();
    expect(screen.getByRole("button", { name: "Preview Team" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Sign in to another account" }));
    expect(screen.getByLabelText("Client secret")).toHaveValue("");
    fireEvent.click(screen.getByRole("button", { name: "Not now" }));
    expect(screen.queryByLabelText("Client ID")).toBeNull();
  });

  it("waits for the browser, and cancelling keeps what was typed", async () => {
    let refuse: (reason: string) => void = () => undefined;
    const connect = vi.fn(
      () =>
        new Promise((_, reject) => {
          refuse = reject;
        }),
    );
    const cancel = vi.fn(async () => {
      refuse("signing in was cancelled");
    });
    desktop({
      GetCalendarAccounts: async () => [],
      ConnectCalendarAccount: connect,
      CancelCalendarSignIn: cancel,
    });
    openAccountTab();

    fireEvent.click(screen.getByRole("radio", { name: "Microsoft" }));
    expect(screen.queryByLabelText("Client secret")).toBeNull();
    fireEvent.click(screen.getByText("Registering ZeitBoard with Microsoft"));
    expect(screen.getByText(/enter http:\/\/localhost/)).toBeVisible();
    fireEvent.change(screen.getByLabelText("Client ID"), {
      target: { value: "00000000-0000-0000-0000-000000000000" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in with Microsoft" }));

    expect(await screen.findByText(/Finish signing in in your browser/)).toBeVisible();
    expect(screen.getByLabelText("Client ID")).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Signing in was cancelled.");
    expect(cancel).toHaveBeenCalledOnce();
    expect(screen.getByLabelText("Client ID")).toHaveValue("00000000-0000-0000-0000-000000000000");
    expect(screen.getByRole("button", { name: "Sign in with Microsoft" })).toBeEnabled();
  });

  // An ended sign-in says so and offers signing in again; removing an
  // account asks for REMOVE and says what became of its access.
  it("offers signing in again after a sign-in ends, and removes an account on REMOVE", async () => {
    const ended = {
      ...account,
      calendars: 2,
      problem: "The sign-in to owner@example.test has ended. Sign in to it again.",
    };
    let accounts: unknown[] = [ended];
    const reconnect = vi.fn(async () => signedIn);
    const remove = vi.fn(async () => {
      accounts = [];
      return { message: "owner@example.test and its calendars are removed from ZeitBoard." };
    });
    desktop({
      GetCalendarAccounts: async () => accounts,
      ReconnectCalendarAccount: reconnect,
      RemoveCalendarAccount: remove,
    });
    const onChanged = openAccountTab();

    expect(await screen.findByText(ended.problem)).toBeVisible();
    expect(
      screen.getByText(/Google account · Signed in Oct 12, 2026 · 2 calendars brought in/),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Sign in to another account" })).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Sign in again" }));
    await waitFor(() => expect(reconnect).toHaveBeenCalledWith({ accountId: account.accountId }));

    fireEvent.click(await screen.findByRole("button", { name: "Remove owner@example.test" }));
    const confirm = screen.getByRole("button", { name: "Remove account" });
    expect(confirm).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Type REMOVE/), { target: { value: "REMOVE" } });
    fireEvent.click(confirm);
    expect(
      await screen.findByText("owner@example.test and its calendars are removed from ZeitBoard."),
    ).toBeVisible();
    expect(remove).toHaveBeenCalledWith({ accountId: account.accountId, confirmation: "REMOVE" });
    expect(onChanged).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.queryByText(ended.problem)).toBeNull());
  });
});
