import { notifyCalendarDataChanged } from "./calendar";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Events the owner adds (ADR-0068): an appointment entered in ZeitBoard, kept
// as a fixed commitment the planner works around. Titles are private text and
// stay with the owner's own devices and server.

export interface OwnerEvent {
  eventId: string;
  revision: number;
  title: string;
  /** RFC 3339 in the event's own zone. */
  startLocal: string;
  endLocal: string;
  zoneId: string;
  startLabel: string;
  endLabel: string;
}

export interface OwnerEventInput {
  /** Both absent for a new event; a change names the revision it was read at. */
  eventId?: string;
  revision?: number;
  title: string;
  /** RFC 3339 with the offset of the occurrence meant. */
  startLocal: string;
  endLocal: string;
  zoneId: string;
}

export const ownerEventTitleLimit = 120;

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function text(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function offsetTime(value: unknown): string | undefined {
  const candidate = text(value);
  return candidate &&
    /(?:[zZ]|[+-]\d\d:\d\d)$/.test(candidate) &&
    Number.isFinite(Date.parse(candidate))
    ? candidate
    : undefined;
}

export function normalizeOwnerEvent(value: unknown): OwnerEvent | undefined {
  if (!isRecord(value)) return undefined;
  const eventId = text(value.eventId);
  const title = text(value.title);
  const startLocal = offsetTime(value.startLocal);
  const endLocal = offsetTime(value.endLocal);
  const zoneId = text(value.zoneId);
  const startLabel = text(value.startLabel);
  const endLabel = text(value.endLabel);
  const revision = value.revision;
  if (
    !eventId ||
    !title ||
    !startLocal ||
    !endLocal ||
    !zoneId ||
    !startLabel ||
    !endLabel ||
    typeof revision !== "number" ||
    !Number.isSafeInteger(revision) ||
    revision < 1 ||
    Date.parse(endLocal) <= Date.parse(startLocal)
  ) {
    return undefined;
  }
  return { eventId, revision, title, startLocal, endLocal, zoneId, startLabel, endLabel };
}

/** Whether events can be added here: only in the desktop app. */
export function hasOwnerEventService(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return Boolean(findWailsMethod(root, ["AddOwnerEvent"]));
}

const notHere = "Events are added in the ZeitBoard desktop app.";

async function keep(
  name: "AddOwnerEvent" | "ChangeOwnerEvent",
  input: OwnerEventInput,
  root: WailsRoot,
): Promise<OwnerEvent> {
  const method = findWailsMethod(root, [name]);
  if (!method) throw new Error(notHere);
  const saved = normalizeOwnerEvent(
    await method({
      eventId: input.eventId ?? "",
      revision: input.revision ?? 0,
      title: input.title,
      startLocal: input.startLocal,
      endLocal: input.endLocal,
      zoneId: input.zoneId,
    }),
  );
  if (!saved) throw new Error("The event was saved, but ZeitBoard could not read it back.");
  notifyCalendarDataChanged();
  return saved;
}

export function addOwnerEvent(
  input: OwnerEventInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<OwnerEvent> {
  return keep("AddOwnerEvent", input, root);
}

export function changeOwnerEvent(
  input: OwnerEventInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<OwnerEvent> {
  return keep("ChangeOwnerEvent", input, root);
}

/** The event as it stands now, with the revision a change or deletion names. */
export async function loadOwnerEvent(
  eventId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<OwnerEvent> {
  const method = findWailsMethod(root, ["GetOwnerEvent"]);
  if (!method) throw new Error(notHere);
  const event = normalizeOwnerEvent(await method({ eventId, revision: 0 }));
  if (!event) throw new Error("That event could not be read.");
  return event;
}

export async function deleteOwnerEvent(
  event: Pick<OwnerEvent, "eventId" | "revision">,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<void> {
  const method = findWailsMethod(root, ["DeleteOwnerEvent"]);
  if (!method) throw new Error(notHere);
  await method({ eventId: event.eventId, revision: event.revision });
  notifyCalendarDataChanged();
}
