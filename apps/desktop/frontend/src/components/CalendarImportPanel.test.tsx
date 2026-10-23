import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CalendarImportPanel } from "./CalendarImportPanel";

const report = {
  sourceId: "calendar_source_test",
  label: "Commitments",
  kind: "ics",
  readOnly: true,
  imported: false,
  eventCount: 1,
  busyCount: 1,
  allDayCount: 0,
  coverageStartAt: "2026-01-01T00:00:00Z",
  coverageEndAt: "2027-01-01T00:00:00Z",
  coverageLabel: "Jan 1, 2026 to Jan 1, 2027",
  previewTruncated: false,
  events: [
    {
      eventId: "calendar_event_test",
      title: "Private appointment",
      startLabel: "Jul 22, 10:00 AM EDT",
      endLabel: "Jul 22, 11:00 AM EDT",
      allDay: false,
      busy: true,
    },
  ],
  message: "Previewed 1 calendar event; 1 blocks scheduling.",
};

afterEach(() => {
  delete (globalThis as { go?: unknown }).go;
});

describe("CalendarImportPanel", () => {
  it("requires an ICS preview before commit and reports the completed import", async () => {
    const preview = vi.fn(async () => report);
    const commit = vi.fn(async () => ({ ...report, imported: true }));
    const onChanged = vi.fn();
    (globalThis as { go?: unknown }).go = {
      main: { App: { PreviewCalendarFile: preview, ImportCalendarFile: commit } },
    };
    render(<CalendarImportPanel available onChanged={onChanged} />);

    const selected = {
      name: "commitments.ics",
      size: 128,
      text: async () => "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
    };
    fireEvent.change(screen.getByLabelText("Calendar file (.ics)"), {
      target: { files: [selected] },
    });
    await waitFor(() => expect(screen.getByText(/stays on this device/)).toBeVisible());
    expect(screen.getByRole("button", { name: "Import" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(await screen.findByText(report.message)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(preview).toHaveBeenCalledWith({
      fileName: "commitments.ics",
      contents: "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
    });
    // No zone goes with it: the desktop places floating times in its own zone,
    // the one every refresh uses, so they cannot move when read again.
    const [sent] = preview.mock.calls[0] as unknown[];
    expect(sent).not.toHaveProperty("zoneId");
    expect(commit).toHaveBeenCalledOnce();
  });

  it("clears a CalDAV password after every request and previews before import", async () => {
    const preview = vi.fn(async () => ({ ...report, kind: "caldav" }));
    const commit = vi.fn(async () => ({ ...report, kind: "caldav", imported: true }));
    const onChanged = vi.fn();
    (globalThis as { go?: unknown }).go = {
      main: { App: { PreviewCalDAVCalendar: preview, ImportCalDAVCalendar: commit } },
    };
    render(<CalendarImportPanel available onChanged={onChanged} />);

    fireEvent.click(screen.getByRole("tab", { name: "CalDAV account" }));
    fireEvent.change(screen.getByLabelText("Collection URL"), {
      target: { value: "https://calendar.example.test/dav/" },
    });
    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "owner" } });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "preview-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    await screen.findByText(report.message);
    expect(screen.getByLabelText("Password")).toHaveValue("");
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "commit-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(screen.getByLabelText("Password")).toHaveValue("");
    expect(preview).toHaveBeenCalledWith(expect.objectContaining({ password: "preview-secret" }));
    expect(commit).toHaveBeenCalledWith(expect.objectContaining({ password: "commit-secret" }));
  });

  // A calendar by its private address: what is kept, and why, is said beside
  // the field; a preview comes before subscribing.
  it("subscribes to a calendar by its private address after a preview", async () => {
    const preview = vi.fn(async () => report);
    const subscribe = vi.fn(async () => ({ ...report, imported: true }));
    const onChanged = vi.fn();
    (globalThis as { go?: unknown }).go = {
      main: { App: { PreviewCalendarSubscription: preview, SubscribeCalendar: subscribe } },
    };
    render(<CalendarImportPanel available onChanged={onChanged} />);

    fireEvent.click(screen.getByRole("tab", { name: "Private address" }));
    expect(screen.getByText(/Anyone holding the address can read the calendar/)).toBeVisible();
    const subscribeButton = screen.getByRole("button", { name: "Subscribe" });
    expect(subscribeButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Calendar address"), {
      target: { value: " webcal://calendar.example.test/ical/synthetic/basic.ics " },
    });
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Work" } });
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(subscribeButton).toBeEnabled());
    expect(preview).toHaveBeenCalledWith({
      address: "webcal://calendar.example.test/ical/synthetic/basic.ics",
      label: "Work",
    });

    fireEvent.click(subscribeButton);
    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(subscribe).toHaveBeenCalledOnce();
    expect(screen.getByLabelText("Calendar address")).toHaveValue("");
  });
});
