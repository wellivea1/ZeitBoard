// Visitor time requests from share links (ADR-0030). These are deliberately
// separate from the generic backend proposals surface: approving one means
// choosing an exact block inside the window the visitor asked for, and the
// generic decision route refuses them for exactly that reason.
//
// The handle and message here are the visitor's own words. They stay inside
// the owner's trust zone and are never sent onward to a provider, a projection,
// or an agent surface.

import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export interface VisitorRequest {
  proposalId: string;
  linkLabel: string;
  handle?: string;
  message?: string;
  windowLabel: string;
  durationLabel?: string;
  windowStartLocal: string;
  windowEndLocal: string;
  durationMinutes: number;
  beyondHorizon: boolean;
  beyondHorizonNote?: string;
  createdLabel: string;
  expiresLabel: string;
  approvalDisclosure: string;
  decisionToken?: string;
}

export interface VisitorRequestsData {
  status: "off" | "ok" | "error";
  message?: string;
  requests: VisitorRequest[];
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

export function normalizeVisitorRequest(value: unknown): VisitorRequest | undefined {
  if (!isRecord(value)) return undefined;
  const proposalId = str(value.proposalId);
  const linkLabel = str(value.linkLabel);
  const windowLabel = str(value.windowLabel);
  const windowStartLocal = str(value.windowStartLocal);
  const windowEndLocal = str(value.windowEndLocal);
  const createdLabel = str(value.createdLabel);
  const expiresLabel = str(value.expiresLabel);
  const approvalDisclosure = str(value.approvalDisclosure);
  if (
    !proposalId ||
    !linkLabel ||
    !windowLabel ||
    !windowStartLocal ||
    !windowEndLocal ||
    !createdLabel ||
    !expiresLabel ||
    !approvalDisclosure
  ) {
    return undefined;
  }
  const handle = str(value.handle);
  const message = str(value.message);
  const durationLabel = str(value.durationLabel);
  const beyondHorizonNote = str(value.beyondHorizonNote);
  const decisionToken = str(value.decisionToken);
  return {
    proposalId,
    linkLabel,
    ...(handle ? { handle } : {}),
    ...(message ? { message } : {}),
    windowLabel,
    ...(durationLabel ? { durationLabel } : {}),
    windowStartLocal,
    windowEndLocal,
    durationMinutes: typeof value.durationMinutes === "number" ? value.durationMinutes : 0,
    beyondHorizon: value.beyondHorizon === true,
    ...(beyondHorizonNote ? { beyondHorizonNote } : {}),
    createdLabel,
    expiresLabel,
    approvalDisclosure,
    ...(decisionToken ? { decisionToken } : {}),
  };
}

export function normalizeVisitorRequests(value: unknown): VisitorRequestsData | undefined {
  if (!isRecord(value)) return undefined;
  const status = value.status;
  if (status !== "off" && status !== "ok" && status !== "error") return undefined;
  const requests: VisitorRequest[] = [];
  if (Array.isArray(value.requests)) {
    for (const item of value.requests) {
      const request = normalizeVisitorRequest(item);
      if (!request) return undefined;
      requests.push(request);
    }
  }
  const message = str(value.message);
  return { status, ...(message ? { message } : {}), requests };
}

const offline: VisitorRequestsData = { status: "off", requests: [] };

export async function loadVisitorRequests(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<VisitorRequestsData> {
  const method = findWailsMethod(root, ["GetBackendVisitorRequests"]);
  if (!method) return offline;
  try {
    const normalized = normalizeVisitorRequests(await method());
    if (normalized) return normalized;
  } catch {
    // Treat a failing bridge like an unreachable backend below.
  }
  return { status: "error", message: "Could not reach the synced backend.", requests: [] };
}

export interface VisitorDecisionInput {
  proposalId: string;
  decision: "approved" | "rejected";
  token: string;
  startLocal?: string;
  endLocal?: string;
}

export async function decideVisitorRequest(
  input: VisitorDecisionInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<VisitorRequestsData> {
  const method = findWailsMethod(root, ["DecideBackendVisitorRequest"]);
  if (!method) return offline;
  try {
    const normalized = normalizeVisitorRequests(
      await method({
        proposalId: input.proposalId,
        decision: input.decision,
        token: input.token,
        startLocal: input.startLocal ?? "",
        endLocal: input.endLocal ?? "",
      }),
    );
    if (normalized) return normalized;
  } catch {
    // The one-use token stays valid until the backend actually consumes it,
    // so a transport failure is safe to report and retry.
  }
  return { status: "error", message: "The decision could not be recorded.", requests: [] };
}

// defaultSlot proposes a starting block: the requested length from the window
// start, or the whole window when the visitor named no length. The owner can
// change it, and the backend re-checks whatever they send.
export function defaultSlot(request: VisitorRequest): { start: string; end: string } {
  if (request.durationMinutes <= 0) {
    return { start: request.windowStartLocal, end: request.windowEndLocal };
  }
  const start = new Date(request.windowStartLocal);
  if (Number.isNaN(start.getTime())) {
    return { start: request.windowStartLocal, end: request.windowEndLocal };
  }
  const end = new Date(start.getTime() + request.durationMinutes * 60_000);
  const pad = (value: number) => String(value).padStart(2, "0");
  const format = (value: Date) =>
    `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}T${pad(value.getHours())}:${pad(value.getMinutes())}`;
  return { start: request.windowStartLocal, end: format(end) };
}
