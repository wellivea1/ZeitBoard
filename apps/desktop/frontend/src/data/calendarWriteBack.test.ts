import { describe, expect, it } from "vitest";
import { loadCalendarWriteBack, normalizeCalendarWriteBack } from "./calendarWriteBack";

describe("calendar write-back status", () => {
  const changed = {
    eventId: "calendar_event_changed",
    title: "Call the pharmacy",
    startAt: "2026-09-28T15:00:00Z",
    detail: "It was changed in your calendar after ZeitBoard wrote it.",
    conflict: true,
    removing: true,
  };
  const on = {
    on: true,
    sourceId: "calendar_source_caldav_work",
    label: "Work",
    username: "owner",
    summary: "1 accepted time is in Work.",
    problems: [changed],
  };

  it("reads what the desktop reports", () => {
    expect(normalizeCalendarWriteBack(on)).toEqual(on);
    expect(
      normalizeCalendarWriteBack({ on: false, summary: "Accepted times stay in ZeitBoard." }),
    ).toEqual({
      on: false,
      summary: "Accepted times stay in ZeitBoard.",
      problems: [],
    });
  });

  it("refuses a status it cannot show truthfully", () => {
    expect(normalizeCalendarWriteBack({ ...on, sourceId: undefined })).toBeUndefined();
    expect(normalizeCalendarWriteBack({ ...on, summary: "" })).toBeUndefined();
    expect(
      normalizeCalendarWriteBack({ ...on, problems: [{ eventId: "x", detail: "y" }] }),
    ).toBeUndefined();
    expect(normalizeCalendarWriteBack(null)).toBeUndefined();
    // An instant that does not parse is left out, not shown as "Invalid Date".
    const garbled = normalizeCalendarWriteBack({
      ...on,
      problems: [{ ...changed, startAt: "soon" }],
    });
    expect(garbled?.problems[0]?.startAt).toBeUndefined();
    expect(garbled?.problems[0]?.detail).toBe(changed.detail);
  });

  it("is absent outside the desktop app", async () => {
    await expect(loadCalendarWriteBack({})).resolves.toBeUndefined();
  });
});
