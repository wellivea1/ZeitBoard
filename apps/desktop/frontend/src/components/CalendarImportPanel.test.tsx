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
    render(<CalendarImportPanel available zoneId="America/New_York" onChanged={onChanged} />);

    const selected = {
      name: "commitments.ics",
      size: 128,
      text: async () => "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
    };
    fireEvent.change(screen.getByLabelText("iCalendar file"), {
      target: { files: [selected] },
    });
    await waitFor(() => expect(screen.getByText(/contents stay/)).toBeVisible());
    expect(screen.getByRole("button", { name: "Import snapshot" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(await screen.findByText(report.message)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Import snapshot" }));

    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(preview).toHaveBeenCalledWith({
      fileName: "commitments.ics",
      contents: "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
      zoneId: "America/New_York",
    });
    expect(commit).toHaveBeenCalledOnce();
  });

  it("clears a CalDAV password after every request and previews before import", async () => {
    const preview = vi.fn(async () => ({ ...report, kind: "caldav" }));
    const commit = vi.fn(async () => ({ ...report, kind: "caldav", imported: true }));
    const onChanged = vi.fn();
    (globalThis as { go?: unknown }).go = {
      main: { App: { PreviewCalDAVCalendar: preview, ImportCalDAVCalendar: commit } },
    };
    render(<CalendarImportPanel available zoneId="UTC" onChanged={onChanged} />);

    fireEvent.click(screen.getByRole("tab", { name: "CalDAV" }));
    fireEvent.change(screen.getByLabelText("Collection URL"), {
      target: { value: "https://calendar.example.test/dav/" },
    });
    fireEvent.change(screen.getByLabelText("Username"), { target: { value: "owner" } });
    fireEvent.change(screen.getByLabelText("One-shot password"), {
      target: { value: "preview-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Preview REPORT" }));

    await screen.findByText(report.message);
    expect(screen.getByLabelText("One-shot password")).toHaveValue("");
    fireEvent.change(screen.getByLabelText("One-shot password"), {
      target: { value: "commit-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Fetch and import" }));

    await waitFor(() => expect(onChanged).toHaveBeenCalledOnce());
    expect(screen.getByLabelText("One-shot password")).toHaveValue("");
    expect(preview).toHaveBeenCalledWith(expect.objectContaining({ password: "preview-secret" }));
    expect(commit).toHaveBeenCalledWith(expect.objectContaining({ password: "commit-secret" }));
  });
});
