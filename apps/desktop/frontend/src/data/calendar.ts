import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export const calendarDataChangedEvent = "zeitboard:calendar-data-changed";

export type CalendarSourceKind = "ics" | "caldav" | "zeitboard";
export type CalendarOwnership = "imported" | "app_owned";

export interface CalendarSource {
  sourceId: string;
  label: string;
  kind: CalendarSourceKind;
  readOnly: boolean;
  endpoint?: string;
  visibleEvents: number;
  coverageLabel: string;
  coverageStart: string;
  coverageEnd: string;
}

export interface CalendarEventSegment {
  segmentId: string;
  eventId: string;
  sourceId: string;
  sourceLabel: string;
  sourceKind: CalendarSourceKind;
  title: string;
  startAt: string;
  endAt: string;
  startLabel: string;
  endLabel: string;
  startMinute: number;
  endMinute: number;
  allDay: boolean;
  pointInTime: boolean;
  busy: boolean;
  ownership: CalendarOwnership;
  readOnly: boolean;
  continuesBefore: boolean;
  continuesAfter: boolean;
  location?: string;
  notes?: string;
}

export interface CalendarBandSegment {
  segmentId: string;
  kind: "predicted_sleep" | "predicted_wake";
  title: string;
  startAt: string;
  endAt: string;
  startLabel: string;
  endLabel: string;
  startMinute: number;
  endMinute: number;
  confidence: "low" | "medium" | "high";
  continuesBefore: boolean;
  continuesAfter: boolean;
}

export interface CalendarDay {
  civilDate: string;
  label: string;
  isToday: boolean;
  events: CalendarEventSegment[];
  predictions: CalendarBandSegment[];
}

export interface CalendarData {
  status: "estimated" | "empty" | "refused" | "unavailable";
  message: string;
  fixtureMode: boolean;
  zoneId: string;
  startCivilDate: string;
  endCivilDate: string;
  updatedLabel: string;
  sources: CalendarSource[];
  days: CalendarDay[];
  warnings: string[];
}

export interface CalendarResult {
  data: CalendarData;
  source: "local" | "fixture";
}

export interface CalendarQuery {
  startCivilDate: string;
  days: number;
  zoneId: string;
}

export function hasLocalCalendarService(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): boolean {
  return Boolean(findWailsMethod(root, ["GetCalendar"]));
}

export interface CalendarFileInput {
  fileName: string;
  contents: string;
  zoneId: string;
}

export interface CalDAVInput {
  endpoint: string;
  label: string;
  username: string;
  password: string;
  zoneId: string;
}

export interface CalendarImportEvent {
  eventId: string;
  title: string;
  startLabel: string;
  endLabel: string;
  allDay: boolean;
  busy: boolean;
}

export interface CalendarImportReport {
  sourceId: string;
  label: string;
  kind: "ics" | "caldav";
  readOnly: true;
  imported: boolean;
  eventCount: number;
  busyCount: number;
  allDayCount: number;
  coverageStartAt: string;
  coverageEndAt: string;
  coverageLabel: string;
  previewTruncated: boolean;
  events: CalendarImportEvent[];
  message: string;
}

export interface CalendarExport {
  fileName: string;
  ics: string;
  generatedAt: string;
  generatedLabel: string;
  eventCount: number;
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function text(value: unknown, allowEmpty = false): string | undefined {
  if (typeof value !== "string" || (!allowEmpty && value.trim().length === 0)) return undefined;
  return value;
}

function optionalText(value: unknown): string | undefined {
  return value === undefined || value === "" ? undefined : text(value);
}

function integer(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : undefined;
}

function minute(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 1440
    ? value
    : undefined;
}

function timestamp(value: unknown): string | undefined {
  const candidate = text(value);
  return candidate &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(candidate) &&
    !Number.isNaN(Date.parse(candidate))
    ? candidate
    : undefined;
}

function civilDate(value: unknown): string | undefined {
  const candidate = text(value);
  if (!candidate || !/^\d{4}-\d{2}-\d{2}$/.test(candidate)) return undefined;
  const parsed = new Date(`${candidate}T00:00:00Z`);
  return !Number.isNaN(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === candidate
    ? candidate
    : undefined;
}

function strings(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const result: string[] = [];
  for (const item of value) {
    const candidate = text(item);
    if (!candidate) return undefined;
    result.push(candidate);
  }
  return result;
}

function sourceKind(value: unknown): CalendarSourceKind | undefined {
  return value === "ics" || value === "caldav" || value === "zeitboard" ? value : undefined;
}

function ownership(value: unknown): CalendarOwnership | undefined {
  return value === "imported" || value === "app_owned" ? value : undefined;
}

function calendarEndpoint(value: unknown, kind: CalendarSourceKind): string | undefined {
  const candidate = optionalText(value);
  if (!candidate) return kind === "caldav" ? undefined : "";
  if (kind !== "caldav") return undefined;
  try {
    const parsed = new URL(candidate);
    const loopback =
      parsed.hostname === "localhost" ||
      parsed.hostname === "127.0.0.1" ||
      parsed.hostname === "[::1]";
    if (
      (parsed.protocol !== "https:" && !(parsed.protocol === "http:" && loopback)) ||
      parsed.username ||
      parsed.password ||
      parsed.search ||
      parsed.hash
    ) {
      return undefined;
    }
    return candidate;
  } catch {
    return undefined;
  }
}

function normalizeSource(value: unknown): CalendarSource | undefined {
  if (!isRecord(value)) return undefined;
  const sourceId = text(value.sourceId);
  const label = text(value.label);
  const kind = sourceKind(value.kind);
  const visibleEvents = integer(value.visibleEvents);
  const coverageLabel = text(value.coverageLabel);
  const coverageStart = timestamp(value.coverageStart);
  const coverageEnd = timestamp(value.coverageEnd);
  const endpoint = kind ? calendarEndpoint(value.endpoint, kind) : undefined;
  if (
    !sourceId ||
    !label ||
    !kind ||
    typeof value.readOnly !== "boolean" ||
    visibleEvents === undefined ||
    !coverageLabel ||
    !coverageStart ||
    !coverageEnd ||
    Date.parse(coverageEnd) <= Date.parse(coverageStart) ||
    endpoint === undefined ||
    ((kind === "ics" || kind === "caldav") && !value.readOnly) ||
    (kind === "zeitboard" && value.readOnly)
  ) {
    return undefined;
  }
  return {
    sourceId,
    label,
    kind,
    readOnly: value.readOnly,
    visibleEvents,
    coverageLabel,
    coverageStart,
    coverageEnd,
    ...(endpoint ? { endpoint } : {}),
  };
}

function normalizeEvent(value: unknown): CalendarEventSegment | undefined {
  if (!isRecord(value)) return undefined;
  const segmentId = text(value.segmentId);
  const eventId = text(value.eventId);
  const sourceId = text(value.sourceId);
  const sourceLabel = text(value.sourceLabel);
  const kind = sourceKind(value.sourceKind);
  const title = text(value.title);
  const startAt = timestamp(value.startAt);
  const endAt = timestamp(value.endAt);
  const startLabel = text(value.startLabel);
  const endLabel = text(value.endLabel);
  const startMinute = minute(value.startMinute);
  const endMinute = minute(value.endMinute);
  const eventOwnership = ownership(value.ownership);
  const startMillis = startAt ? Date.parse(startAt) : Number.NaN;
  const endMillis = endAt ? Date.parse(endAt) : Number.NaN;
  const pointInTime = startMillis === endMillis;
  if (
    !segmentId ||
    !eventId ||
    !sourceId ||
    !sourceLabel ||
    !kind ||
    !title ||
    !startAt ||
    !endAt ||
    !startLabel ||
    !endLabel ||
    startMinute === undefined ||
    endMinute === undefined ||
    !eventOwnership ||
    typeof value.allDay !== "boolean" ||
    typeof value.pointInTime !== "boolean" ||
    typeof value.busy !== "boolean" ||
    typeof value.readOnly !== "boolean" ||
    typeof value.continuesBefore !== "boolean" ||
    typeof value.continuesAfter !== "boolean" ||
    endMillis < startMillis ||
    value.pointInTime !== pointInTime ||
    (value.busy && pointInTime) ||
    (value.allDay && pointInTime) ||
    (eventOwnership === "imported" && !value.readOnly) ||
    (eventOwnership === "app_owned" && value.readOnly)
  ) {
    return undefined;
  }
  const location = optionalText(value.location);
  const notes = optionalText(value.notes);
  return {
    segmentId,
    eventId,
    sourceId,
    sourceLabel,
    sourceKind: kind,
    title,
    startAt,
    endAt,
    startLabel,
    endLabel,
    startMinute,
    endMinute,
    allDay: value.allDay,
    pointInTime: value.pointInTime,
    busy: value.busy,
    ownership: eventOwnership,
    readOnly: value.readOnly,
    continuesBefore: value.continuesBefore,
    continuesAfter: value.continuesAfter,
    ...(location ? { location } : {}),
    ...(notes ? { notes } : {}),
  };
}

function normalizeBand(value: unknown): CalendarBandSegment | undefined {
  if (!isRecord(value)) return undefined;
  const segmentId = text(value.segmentId);
  const kind =
    value.kind === "predicted_sleep" || value.kind === "predicted_wake" ? value.kind : undefined;
  const title = text(value.title);
  const startAt = timestamp(value.startAt);
  const endAt = timestamp(value.endAt);
  const startLabel = text(value.startLabel);
  const endLabel = text(value.endLabel);
  const startMinute = minute(value.startMinute);
  const endMinute = minute(value.endMinute);
  const confidence =
    value.confidence === "low" || value.confidence === "medium" || value.confidence === "high"
      ? value.confidence
      : undefined;
  const startMillis = startAt ? Date.parse(startAt) : Number.NaN;
  const endMillis = endAt ? Date.parse(endAt) : Number.NaN;
  if (
    !segmentId ||
    !kind ||
    !title ||
    !startAt ||
    !endAt ||
    !startLabel ||
    !endLabel ||
    startMinute === undefined ||
    endMinute === undefined ||
    !confidence ||
    endMillis <= startMillis ||
    typeof value.continuesBefore !== "boolean" ||
    typeof value.continuesAfter !== "boolean"
  ) {
    return undefined;
  }
  return {
    segmentId,
    kind,
    title,
    startAt,
    endAt,
    startLabel,
    endLabel,
    startMinute,
    endMinute,
    confidence,
    continuesBefore: value.continuesBefore,
    continuesAfter: value.continuesAfter,
  };
}

function normalizeDay(value: unknown): CalendarDay | undefined {
  if (!isRecord(value) || !Array.isArray(value.events) || !Array.isArray(value.predictions)) {
    return undefined;
  }
  const date = civilDate(value.civilDate);
  const label = text(value.label);
  if (!date || !label || typeof value.isToday !== "boolean") return undefined;
  const events: CalendarEventSegment[] = [];
  for (const item of value.events) {
    const event = normalizeEvent(item);
    if (!event) return undefined;
    events.push(event);
  }
  const predictions: CalendarBandSegment[] = [];
  for (const item of value.predictions) {
    const prediction = normalizeBand(item);
    if (!prediction) return undefined;
    predictions.push(prediction);
  }
  return { civilDate: date, label, isToday: value.isToday, events, predictions };
}

export function normalizeCalendar(value: unknown): CalendarData | undefined {
  if (!isRecord(value) || !Array.isArray(value.sources) || !Array.isArray(value.days)) {
    return undefined;
  }
  const status =
    value.status === "estimated" ||
    value.status === "empty" ||
    value.status === "refused" ||
    value.status === "unavailable"
      ? value.status
      : undefined;
  const message = text(value.message);
  const zoneId = text(value.zoneId);
  const startCivilDate = civilDate(value.startCivilDate);
  const endCivilDate = civilDate(value.endCivilDate);
  const updatedLabel = text(value.updatedLabel);
  const warnings = strings(value.warnings);
  if (
    !status ||
    !message ||
    typeof value.fixtureMode !== "boolean" ||
    !zoneId ||
    !startCivilDate ||
    !endCivilDate ||
    !updatedLabel ||
    !warnings
  ) {
    return undefined;
  }
  const sources: CalendarSource[] = [];
  const sourceIds = new Set<string>();
  for (const item of value.sources) {
    const source = normalizeSource(item);
    if (!source || sourceIds.has(source.sourceId)) return undefined;
    sourceIds.add(source.sourceId);
    sources.push(source);
  }
  const sourcesById = new Map(sources.map((source) => [source.sourceId, source]));
  const days: CalendarDay[] = [];
  const civilDates = new Set<string>();
  const segmentIds = new Set<string>();
  const visibleEvents = new Map<string, Set<string>>();
  for (const item of value.days) {
    const day = normalizeDay(item);
    if (!day || civilDates.has(day.civilDate)) return undefined;
    const expectedDate = addCivilDays(startCivilDate, days.length);
    if (day.civilDate !== expectedDate) return undefined;
    for (const event of day.events) {
      const source = sourcesById.get(event.sourceId);
      if (
        !source ||
        segmentIds.has(event.segmentId) ||
        event.sourceLabel !== source.label ||
        event.sourceKind !== source.kind ||
        event.readOnly !== source.readOnly ||
        (event.ownership === "imported" && source.kind === "zeitboard") ||
        (event.ownership === "app_owned" && source.kind !== "zeitboard")
      ) {
        return undefined;
      }
      segmentIds.add(event.segmentId);
      const sourceEvents = visibleEvents.get(event.sourceId) ?? new Set<string>();
      sourceEvents.add(event.eventId);
      visibleEvents.set(event.sourceId, sourceEvents);
    }
    for (const prediction of day.predictions) {
      if (segmentIds.has(prediction.segmentId)) return undefined;
      segmentIds.add(prediction.segmentId);
    }
    civilDates.add(day.civilDate);
    days.push(day);
  }
  if (
    days.length === 0 ||
    days.length > 14 ||
    days[0]?.civilDate !== startCivilDate ||
    days.at(-1)?.civilDate !== endCivilDate
  ) {
    return undefined;
  }
  if (
    sources.some(
      (source) => (visibleEvents.get(source.sourceId)?.size ?? 0) !== source.visibleEvents,
    )
  ) {
    return undefined;
  }
  return {
    status,
    message,
    fixtureMode: value.fixtureMode,
    zoneId,
    startCivilDate,
    endCivilDate,
    updatedLabel,
    sources,
    days,
    warnings,
  };
}

function normalizeImportEvent(value: unknown): CalendarImportEvent | undefined {
  if (!isRecord(value)) return undefined;
  const eventId = text(value.eventId);
  const title = text(value.title);
  const startLabel = text(value.startLabel);
  const endLabel = text(value.endLabel);
  if (
    !eventId ||
    !title ||
    !startLabel ||
    !endLabel ||
    typeof value.allDay !== "boolean" ||
    typeof value.busy !== "boolean"
  ) {
    return undefined;
  }
  return { eventId, title, startLabel, endLabel, allDay: value.allDay, busy: value.busy };
}

export function normalizeCalendarImport(value: unknown): CalendarImportReport | undefined {
  if (!isRecord(value) || !Array.isArray(value.events)) return undefined;
  const sourceId = text(value.sourceId);
  const label = text(value.label);
  const kind = value.kind === "ics" || value.kind === "caldav" ? value.kind : undefined;
  const eventCount = integer(value.eventCount);
  const busyCount = integer(value.busyCount);
  const allDayCount = integer(value.allDayCount);
  const coverageStartAt = timestamp(value.coverageStartAt);
  const coverageEndAt = timestamp(value.coverageEndAt);
  const coverageLabel = text(value.coverageLabel);
  const message = text(value.message);
  if (
    !sourceId ||
    !label ||
    !kind ||
    value.readOnly !== true ||
    typeof value.imported !== "boolean" ||
    eventCount === undefined ||
    busyCount === undefined ||
    allDayCount === undefined ||
    busyCount > eventCount ||
    allDayCount > eventCount ||
    !coverageStartAt ||
    !coverageEndAt ||
    Date.parse(coverageEndAt) <= Date.parse(coverageStartAt) ||
    !coverageLabel ||
    typeof value.previewTruncated !== "boolean" ||
    !message
  ) {
    return undefined;
  }
  const events: CalendarImportEvent[] = [];
  const eventIds = new Set<string>();
  for (const item of value.events) {
    const event = normalizeImportEvent(item);
    if (!event || eventIds.has(event.eventId)) return undefined;
    eventIds.add(event.eventId);
    events.push(event);
  }
  if ((!value.previewTruncated && events.length !== eventCount) || events.length > eventCount)
    return undefined;
  return {
    sourceId,
    label,
    kind,
    readOnly: true,
    imported: value.imported,
    eventCount,
    busyCount,
    allDayCount,
    coverageStartAt,
    coverageEndAt,
    coverageLabel,
    previewTruncated: value.previewTruncated,
    events,
    message,
  };
}

function normalizeExport(value: unknown): CalendarExport | undefined {
  if (!isRecord(value)) return undefined;
  const fileName = text(value.fileName);
  const ics = text(value.ics);
  const generatedAt = timestamp(value.generatedAt);
  const generatedLabel = text(value.generatedLabel);
  const eventCount = integer(value.eventCount);
  if (
    !fileName ||
    !fileName.toLowerCase().endsWith(".ics") ||
    !ics ||
    !ics.startsWith("BEGIN:VCALENDAR\r\n") ||
    !ics.endsWith("END:VCALENDAR\r\n") ||
    !generatedAt ||
    !generatedLabel ||
    eventCount === undefined
  ) {
    return undefined;
  }
  return { fileName, ics, generatedAt, generatedLabel, eventCount };
}

export async function loadCalendar(
  query: CalendarQuery,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<CalendarResult> {
  if (
    !civilDate(query.startCivilDate) ||
    !Number.isInteger(query.days) ||
    query.days < 1 ||
    query.days > 14 ||
    !text(query.zoneId)
  ) {
    throw new Error("Calendar query requires a valid start date, 1 to 14 days, and a zone.");
  }
  const method = findWailsMethod(root, ["GetCalendar"]);
  if (!method) return { data: calendarFixture(query), source: "fixture" };
  const normalized = normalizeCalendar(await method(query));
  if (!normalized) throw new Error("Calendar service returned an invalid response.");
  return { data: normalized, source: normalized.fixtureMode ? "fixture" : "local" };
}

async function importCall(
  methodName: string,
  input: CalendarFileInput | CalDAVInput,
  root: WailsRoot,
): Promise<CalendarImportReport> {
  const method = findWailsMethod(root, [methodName]);
  if (!method) throw new Error("Calendar import is available in the ZeitBoard desktop app.");
  const report = normalizeCalendarImport(await method(input));
  if (!report) throw new Error("Calendar import returned an invalid report.");
  return report;
}

export function previewCalendarFile(
  input: CalendarFileInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return importCall("PreviewCalendarFile", input, root);
}

export function importCalendarFile(
  input: CalendarFileInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return importCall("ImportCalendarFile", input, root);
}

export function previewCalDAVCalendar(
  input: CalDAVInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return importCall("PreviewCalDAVCalendar", input, root);
}

export function importCalDAVCalendar(
  input: CalDAVInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return importCall("ImportCalDAVCalendar", input, root);
}

export async function removeCalendarSource(
  sourceId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<void> {
  const method = findWailsMethod(root, ["RemoveCalendarSource"]);
  if (!method) throw new Error("Calendar source removal is available in the desktop app.");
  await method({ sourceId, confirmation: "REMOVE" });
}

export async function exportOwnedCalendar(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<CalendarExport> {
  const method = findWailsMethod(root, ["ExportOwnedCalendar"]);
  if (!method) throw new Error("Calendar export is available in the desktop app.");
  const result = normalizeExport(await method());
  if (!result) throw new Error("Calendar export returned an invalid response.");
  return result;
}

export function downloadCalendarExport(value: CalendarExport): boolean {
  if (
    (typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom")) ||
    typeof Blob === "undefined" ||
    typeof URL === "undefined" ||
    typeof URL.createObjectURL !== "function" ||
    typeof document === "undefined"
  ) {
    return false;
  }
  const blob = new Blob([value.ics], { type: "text/calendar;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = value.fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  return true;
}

export function notifyCalendarDataChanged() {
  if (typeof window !== "undefined") window.dispatchEvent(new Event(calendarDataChangedEvent));
}

export function addCivilDays(value: string, days: number): string {
  if (!civilDate(value) || !Number.isInteger(days)) {
    throw new Error("Civil date must be valid YYYY-MM-DD and days must be an integer.");
  }
  const date = new Date(`${value}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

export function todayCivilDate(zoneId: string): string {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: zoneId,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date());
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((item) => item.type === type)?.value;
  const year = part("year");
  const month = part("month");
  const day = part("day");
  if (!year || !month || !day) throw new Error("Could not resolve today's civil date.");
  return `${year}-${month}-${day}`;
}

function calendarFixture(query: CalendarQuery): CalendarData {
  const start = query.startCivilDate;
  const count = query.days;
  const source: CalendarSource = {
    sourceId: "calendar_source_fixture",
    label: "Sample commitments",
    kind: "ics",
    readOnly: true,
    visibleEvents: 1,
    coverageLabel: "Sample range only",
    coverageStart: `${start}T00:00:00Z`,
    coverageEnd: `${addCivilDays(start, count)}T00:00:00Z`,
  };
  const days: CalendarDay[] = Array.from({ length: count }, (_, index) => {
    const date = addCivilDays(start, index);
    const label = new Intl.DateTimeFormat("en-US", {
      weekday: "short",
      month: "short",
      day: "numeric",
      timeZone: "UTC",
    }).format(new Date(`${date}T12:00:00Z`));
    return {
      civilDate: date,
      label,
      isToday: false,
      events:
        index === 0
          ? [
              {
                segmentId: `calendar_event_fixture_${date.replaceAll("-", "")}`,
                eventId: "calendar_event_fixture",
                sourceId: source.sourceId,
                sourceLabel: source.label,
                sourceKind: source.kind,
                title: "Sample fixed event",
                startAt: `${date}T18:00:00Z`,
                endAt: `${date}T19:00:00Z`,
                startLabel: `${label}, 6:00 PM UTC`,
                endLabel: `${label}, 7:00 PM UTC`,
                startMinute: 1080,
                endMinute: 1140,
                allDay: false,
                pointInTime: false,
                busy: true,
                ownership: "imported",
                readOnly: true,
                continuesBefore: false,
                continuesAfter: false,
              },
            ]
          : [],
      predictions: [
        {
          segmentId: `sample_sleep_${date}`,
          kind: "predicted_sleep",
          title: "Predicted sleep window",
          startAt: `${date}T01:00:00Z`,
          endAt: `${date}T08:00:00Z`,
          startLabel: `${label}, 1:00 AM UTC`,
          endLabel: `${label}, 8:00 AM UTC`,
          startMinute: 60,
          endMinute: 480,
          confidence: "medium",
          continuesBefore: false,
          continuesAfter: false,
        },
        {
          segmentId: `sample_wake_${date}`,
          kind: "predicted_wake",
          title: "Predicted waking window",
          startAt: `${date}T09:00:00Z`,
          endAt: `${date}T22:00:00Z`,
          startLabel: `${label}, 9:00 AM UTC`,
          endLabel: `${label}, 10:00 PM UTC`,
          startMinute: 540,
          endMinute: 1320,
          confidence: "medium",
          continuesBefore: false,
          continuesAfter: false,
        },
      ],
    };
  });
  return {
    status: "estimated",
    message: "Sample calendar data. Run the desktop service to load local sources.",
    fixtureMode: true,
    zoneId: "UTC",
    startCivilDate: start,
    endCivilDate: addCivilDays(start, count - 1),
    updatedLabel: "Sample preview",
    sources: [source],
    days,
    warnings: ["Sample mode does not read or write calendar files."],
  };
}
