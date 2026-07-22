import { describe, expect, it, vi } from "vitest";

import {
  addCivilDays,
  exportOwnedCalendar,
  importCalendarFile,
  loadCalendar,
  normalizeCalendar,
  normalizeCalendarImport,
  previewCalendarFile,
  removeCalendarSource,
} from "./calendar";

const calendarResponse = {
  status: "estimated",
  message: "1 real calendar event in this range.",
  fixtureMode: false,
  zoneId: "America/New_York",
  startCivilDate: "2026-07-22",
  endCivilDate: "2026-07-22",
  updatedLabel: "Updated Jul 22, 8:00 AM EDT",
  sources: [
    {
      sourceId: "calendar_source_test",
      label: "Commitments",
      kind: "ics",
      readOnly: true,
      visibleEvents: 1,
      coverageLabel: "Jul 1 to Aug 1",
      coverageStart: "2026-07-01T00:00:00Z",
      coverageEnd: "2026-08-01T00:00:00Z",
    },
  ],
  days: [
    {
      civilDate: "2026-07-22",
      label: "Wed, Jul 22",
      isToday: true,
      events: [
        {
          segmentId: "calendar_event_test_20260722",
          eventId: "calendar_event_test",
          sourceId: "calendar_source_test",
          sourceLabel: "Commitments",
          sourceKind: "ics",
          title: "Private appointment",
          startAt: "2026-07-22T14:00:00Z",
          endAt: "2026-07-22T15:00:00Z",
          startLabel: "Jul 22, 2026, 10:00 AM EDT",
          endLabel: "Jul 22, 2026, 11:00 AM EDT",
          startMinute: 600,
          endMinute: 660,
          allDay: false,
          pointInTime: false,
          busy: true,
          ownership: "imported",
          readOnly: true,
          continuesBefore: false,
          continuesAfter: false,
        },
      ],
      predictions: [
        {
          segmentId: "forecast_1_20260722",
          kind: "predicted_wake",
          title: "Predicted waking window",
          startAt: "2026-07-22T12:00:00Z",
          endAt: "2026-07-22T22:00:00Z",
          startLabel: "Jul 22, 2026, 8:00 AM EDT",
          endLabel: "Jul 22, 2026, 6:00 PM EDT",
          startMinute: 480,
          endMinute: 1080,
          confidence: "medium",
          continuesBefore: false,
          continuesAfter: false,
        },
      ],
    },
  ],
  warnings: [],
};

const importResponse = {
  sourceId: "calendar_source_test",
  label: "commitments.ics",
  kind: "ics",
  readOnly: true,
  imported: false,
  eventCount: 1,
  busyCount: 1,
  allDayCount: 0,
  coverageStartAt: "2025-07-22T00:00:00Z",
  coverageEndAt: "2028-07-22T00:00:00Z",
  coverageLabel: "Jul 22, 2025 to Jul 22, 2028",
  previewTruncated: false,
  events: [
    {
      eventId: "calendar_event_test",
      title: "Private appointment",
      startLabel: "Jul 22, 2026, 10:00 AM EDT",
      endLabel: "Jul 22, 2026, 11:00 AM EDT",
      allDay: false,
      busy: true,
    },
  ],
  message: "Previewed 1 calendar event; 1 blocks scheduling.",
};

describe("calendar data adapter", () => {
  it("normalizes local sources, exact events, and separate prediction bands", () => {
    expect(normalizeCalendar(calendarResponse)).toMatchObject({
      fixtureMode: false,
      sources: [{ kind: "ics", readOnly: true }],
      days: [
        {
          events: [{ ownership: "imported", title: "Private appointment" }],
          predictions: [{ kind: "predicted_wake", confidence: "medium" }],
        },
      ],
    });
  });

  it("rejects ownership contradictions and unknown source links", () => {
    const writableImport = structuredClone(calendarResponse);
    writableImport.days[0]!.events[0]!.readOnly = false;
    expect(normalizeCalendar(writableImport)).toBeUndefined();

    const unknownSource = structuredClone(calendarResponse);
    unknownSource.days[0]!.events[0]!.sourceId = "calendar_source_missing";
    expect(normalizeCalendar(unknownSource)).toBeUndefined();

    const mismatchedSource = structuredClone(calendarResponse);
    mismatchedSource.days[0]!.events[0]!.sourceLabel = "Wrong source";
    expect(normalizeCalendar(mismatchedSource)).toBeUndefined();

    const incorrectCount = structuredClone(calendarResponse);
    incorrectCount.sources[0]!.visibleEvents = 2;
    expect(normalizeCalendar(incorrectCount)).toBeUndefined();
  });

  it("rejects impossible intervals, duplicate ids, and nonconsecutive civil days", () => {
    const reversed = structuredClone(calendarResponse);
    reversed.days[0]!.events[0]!.endAt = "2026-07-22T13:00:00Z";
    expect(normalizeCalendar(reversed)).toBeUndefined();

    const pointMismatch = structuredClone(calendarResponse);
    pointMismatch.days[0]!.events[0]!.pointInTime = true;
    expect(normalizeCalendar(pointMismatch)).toBeUndefined();

    const localTimestamp = structuredClone(calendarResponse);
    localTimestamp.days[0]!.events[0]!.startAt = "2026-07-22T14:00:00";
    expect(normalizeCalendar(localTimestamp)).toBeUndefined();

    const duplicateSource = structuredClone(calendarResponse);
    duplicateSource.sources.push(structuredClone(duplicateSource.sources[0]!));
    expect(normalizeCalendar(duplicateSource)).toBeUndefined();

    const skippedDay = structuredClone(calendarResponse);
    skippedDay.endCivilDate = "2026-07-24";
    skippedDay.days.push({
      civilDate: "2026-07-24",
      label: "Fri, Jul 24",
      isToday: false,
      events: [],
      predictions: [],
    });
    expect(normalizeCalendar(skippedDay)).toBeUndefined();

    const foldedHour = structuredClone(calendarResponse);
    foldedHour.days[0]!.events[0]!.startMinute = 105;
    foldedHour.days[0]!.events[0]!.endMinute = 75;
    expect(normalizeCalendar(foldedHour)).toBeDefined();
  });

  it("accepts only sanitized CalDAV endpoints", () => {
    const caldav = structuredClone(calendarResponse);
    caldav.sources[0]!.kind = "caldav";
    Object.assign(caldav.sources[0]!, { endpoint: "https://calendar.example.test/dav/" });
    caldav.days[0]!.events[0]!.sourceKind = "caldav";
    expect(normalizeCalendar(caldav)).toBeDefined();

    Object.assign(caldav.sources[0]!, {
      endpoint: "https://owner:secret@calendar.example.test/dav/?token=x",
    });
    expect(normalizeCalendar(caldav)).toBeUndefined();
  });

  it("loads live data when the desktop method exists and labels fallback fixtures", async () => {
    await expect(
      loadCalendar(
        { startCivilDate: "2026-07-22", days: 1, zoneId: "America/New_York" },
        { go: { main: { App: { GetCalendar: async () => calendarResponse } } } },
      ),
    ).resolves.toMatchObject({ source: "local", data: { fixtureMode: false } });

    const fixture = await loadCalendar(
      { startCivilDate: "2026-07-22", days: 2, zoneId: "America/New_York" },
      {},
    );
    expect(fixture.source).toBe("fixture");
    expect(fixture.data.fixtureMode).toBe(true);
    expect(fixture.data.days).toHaveLength(2);

    await expect(
      loadCalendar({ startCivilDate: "2026-02-30", days: 2, zoneId: "UTC" }, {}),
    ).rejects.toThrow(/valid start date/);
  });

  it("normalizes preview and commit calls without weakening the report", async () => {
    const preview = vi.fn(async () => importResponse);
    const commit = vi.fn(async () => ({ ...importResponse, imported: true }));
    const root = {
      go: { main: { App: { PreviewCalendarFile: preview, ImportCalendarFile: commit } } },
    };
    const input = { fileName: "commitments.ics", contents: "BEGIN:VCALENDAR", zoneId: "UTC" };
    await expect(previewCalendarFile(input, root)).resolves.toMatchObject({ imported: false });
    await expect(importCalendarFile(input, root)).resolves.toMatchObject({ imported: true });
    expect(preview).toHaveBeenCalledWith(input);
    expect(commit).toHaveBeenCalledWith(input);
    expect(normalizeCalendarImport({ ...importResponse, busyCount: 2 })).toBeUndefined();
  });

  it("uses explicit source erasure and validates owned export", async () => {
    const remove = vi.fn(async () => undefined);
    const exportMethod = vi.fn(async () => ({
      fileName: "zeitboard-placements.ics",
      ics: "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
      generatedAt: "2026-07-22T12:00:00Z",
      generatedLabel: "Jul 22, 2026, 8:00 AM",
      eventCount: 0,
    }));
    const root = {
      go: { main: { App: { RemoveCalendarSource: remove, ExportOwnedCalendar: exportMethod } } },
    };
    await removeCalendarSource("calendar_source_test", root);
    expect(remove).toHaveBeenCalledWith({
      sourceId: "calendar_source_test",
      confirmation: "REMOVE",
    });
    await expect(exportOwnedCalendar(root)).resolves.toMatchObject({ eventCount: 0 });

    const invalidRoot = {
      go: {
        main: {
          App: {
            ExportOwnedCalendar: async () => ({
              fileName: "zeitboard-placements.ics",
              ics: "BEGIN:VCALENDAR\nEND:VCALENDAR\n",
              generatedAt: "2026-07-22T12:00:00Z",
              generatedLabel: "Jul 22, 2026, 8:00 AM",
              eventCount: 0,
            }),
          },
        },
      },
    };
    await expect(exportOwnedCalendar(invalidRoot)).rejects.toThrow(/invalid response/);
  });

  it("adds civil days without crossing through the host local time zone", () => {
    expect(addCivilDays("2026-03-08", 1)).toBe("2026-03-09");
    expect(addCivilDays("2026-12-31", 1)).toBe("2027-01-01");
    expect(() => addCivilDays("2026-02-30", 1)).toThrow(/valid YYYY-MM-DD/);
    expect(() => addCivilDays("2026-03-08", 0.5)).toThrow(/integer/);
  });
});
