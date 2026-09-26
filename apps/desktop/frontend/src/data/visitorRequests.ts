// Visitor time requests from share links (ADR-0030). These are deliberately
// separate from generic backend proposal decisions: approving one means
// choosing an exact block inside the window the visitor asked for, and the
// generic decision route refuses them for exactly that reason.
//
// The handle and message here are the visitor's own words. They stay inside
// the owner's trust zone and are never sent onward to a provider, a projection,
// or an agent surface.

import {
  emptyReviewSummary,
  terminalReviewPage,
  normalizeReviewSummary,
  normalizeReviewPagination,
  type ReviewQueueSummary,
  type ReviewQueuePagination,
} from "./reviewQueue";

import { civilMinute } from "../utils/civilTime";

import { findWailsMethod, hasDesktopBridge, type WailsRoot } from "./wailsBridge";

export interface VisitorRequest {
  status: "pending" | "approved" | "rejected";
  expiresAt: string;
  proposalId: string;
  linkLabel: string;
  handle?: string;
  message?: string;
  windowLabel: string;
  durationLabel?: string;
  windowStartAt: string;
  windowEndAt: string;
  durationMinutes: number;
  beyondHorizon: boolean;
  beyondHorizonNote?: string;
  createdLabel: string;
  expiresLabel: string;
  approvalDisclosure: string;
  decisionToken?: string;
  /** The request's thread, oldest first, and whether a reply can be added. */
  messages: VisitorMessage[];
  canMessage: boolean;
}

export interface VisitorMessage {
  author: "visitor" | "owner";
  authorLabel: string;
  body: string;
  createdLabel: string;
}

export interface VisitorRequestsData extends ReviewQueueSummary {
  decisionRecorded?: boolean;
  pagination: ReviewQueuePagination;
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
  const status = value.status;
  const expiresAt = str(value.expiresAt);
  if (
    (status !== "pending" && status !== "approved" && status !== "rejected") ||
    !expiresAt ||
    !Number.isFinite(Date.parse(expiresAt))
  )
    return undefined;
  const proposalId = str(value.proposalId);
  const linkLabel = str(value.linkLabel);
  const windowLabel = str(value.windowLabel);
  const windowStartAt = str(value.windowStartAt);
  const windowEndAt = str(value.windowEndAt);
  if (
    !windowStartAt ||
    !windowEndAt ||
    !Number.isFinite(Date.parse(windowStartAt)) ||
    !Number.isFinite(Date.parse(windowEndAt)) ||
    Date.parse(windowEndAt) <= Date.parse(windowStartAt)
  )
    return undefined;
  const createdLabel = str(value.createdLabel);
  const expiresLabel = str(value.expiresLabel);
  const approvalDisclosure = str(value.approvalDisclosure);
  if (
    !proposalId ||
    !linkLabel ||
    !windowLabel ||
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
  const messages: VisitorMessage[] = [];
  for (const item of Array.isArray(value.messages) ? value.messages : []) {
    if (!isRecord(item)) return undefined;
    const author = item.author;
    const authorLabel = str(item.authorLabel);
    const body = str(item.body);
    const createdLabel = str(item.createdLabel);
    if ((author !== "visitor" && author !== "owner") || !authorLabel || !body || !createdLabel) {
      return undefined;
    }
    messages.push({ author, authorLabel, body, createdLabel });
  }
  return {
    proposalId,
    status,
    expiresAt,
    linkLabel,
    ...(handle ? { handle } : {}),
    ...(message ? { message } : {}),
    windowLabel,
    ...(durationLabel ? { durationLabel } : {}),
    windowStartAt,
    windowEndAt,
    durationMinutes: typeof value.durationMinutes === "number" ? value.durationMinutes : 0,
    beyondHorizon: value.beyondHorizon === true,
    ...(beyondHorizonNote ? { beyondHorizonNote } : {}),
    createdLabel,
    expiresLabel,
    approvalDisclosure,
    ...(decisionToken ? { decisionToken } : {}),
    messages,
    canMessage: value.canMessage === true,
  };
}

export function normalizeVisitorRequests(value: unknown): VisitorRequestsData | undefined {
  if (!isRecord(value)) return undefined;
  const status = value.status;
  if (status !== "off" && status !== "ok" && status !== "error") return undefined;
  const summary = normalizeReviewSummary(value);
  const pagination = normalizeReviewPagination(value.pagination);
  if (!summary || !pagination) return undefined;
  if (!Array.isArray(value.requests)) return undefined;
  const requests: VisitorRequest[] = [];
  {
    for (const item of value.requests) {
      const request = normalizeVisitorRequest(item);
      if (!request) return undefined;
      requests.push(request);
    }
  }
  const message = str(value.message);
  return {
    ...(value.decisionRecorded === true ? { decisionRecorded: true } : {}),
    ...summary,
    pagination,
    status,
    ...(message ? { message } : {}),
    requests,
  };
}

export const emptyVisitorRequests: VisitorRequestsData = {
  ...emptyReviewSummary,
  pagination: terminalReviewPage(),
  status: "off",
  requests: [],
};

export async function loadVisitorRequests(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<VisitorRequestsData> {
  const method = findWailsMethod(root, ["GetBackendVisitorRequests"]);
  if (!method)
    return hasDesktopBridge(root)
      ? {
          ...emptyVisitorRequests,
          status: "error",
          message: "The desktop review service is unavailable. Restart the app to retry.",
        }
      : emptyVisitorRequests;
  try {
    const normalized = normalizeVisitorRequests(await method());
    if (normalized) return normalized;
  } catch {
    // Treat a failing bridge like an unreachable backend below.
  }
  return {
    ...emptyVisitorRequests,
    status: "error",
    message: "Could not reach the synced backend.",
    requests: [],
  };
}

export async function loadVisitorRequestPage(
  cursor: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<VisitorRequestsData> {
  const method = findWailsMethod(root, ["GetBackendVisitorRequestPage"]);
  if (method && cursor) {
    try {
      const result = normalizeVisitorRequests(await method({ cursor }));
      if (result) return result;
    } catch {
      /* handled below */
    }
  }
  return {
    ...emptyVisitorRequests,
    status: "error",
    message: "Could not load older time requests.",
  };
}

export interface VisitorDecisionInput {
  proposalId: string;
  decision: "approved" | "rejected";
  token: string;
  startAt?: string;
  endAt?: string;
}

export async function decideVisitorRequest(
  input: VisitorDecisionInput,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<VisitorRequestsData> {
  const method = findWailsMethod(root, ["DecideBackendVisitorRequest"]);
  if (!method)
    return hasDesktopBridge(root)
      ? {
          ...emptyVisitorRequests,
          status: "error",
          message: "The desktop review service is unavailable. Restart the app to retry.",
        }
      : emptyVisitorRequests;
  try {
    const normalized = normalizeVisitorRequests(
      await method({
        proposalId: input.proposalId,
        decision: input.decision,
        token: input.token,
        startAt: input.startAt ?? "",
        endAt: input.endAt ?? "",
      }),
    );
    if (normalized) return normalized;
  } catch {
    // The one-use token stays valid until the backend actually consumes it,
    // so a transport failure is safe to report and retry.
  }
  return {
    ...emptyVisitorRequests,
    status: "error",
    message: "Could not confirm the decision. Refresh the queue before retrying.",
    requests: [],
  };
}

// A reply to a request's thread, or erasing the thread, returns the refreshed
// queue like a decision does, so the result appears where it was typed.
async function threadAction(
  name: "ReplyToBackendVisitorRequest" | "EraseBackendVisitorThread",
  input: { proposalId: string; message?: string },
  root: WailsRoot,
): Promise<VisitorRequestsData> {
  const method = findWailsMethod(root, [name]);
  if (!method) {
    return {
      ...emptyVisitorRequests,
      status: "error",
      message: "Messages need the ZeitBoard desktop app.",
    };
  }
  try {
    const normalized = normalizeVisitorRequests(
      await method({ proposalId: input.proposalId, message: input.message ?? "" }),
    );
    if (normalized) return normalized;
  } catch {
    // Reported below; nothing was lost on this side.
  }
  return {
    ...emptyVisitorRequests,
    status: "error",
    message: "Could not reach your server. Nothing was sent.",
  };
}

export function replyToVisitorRequest(
  proposalId: string,
  message: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<VisitorRequestsData> {
  return threadAction("ReplyToBackendVisitorRequest", { proposalId, message }, root);
}

export function eraseVisitorThread(
  proposalId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<VisitorRequestsData> {
  return threadAction("EraseBackendVisitorThread", { proposalId }, root);
}

// defaultSlot proposes a starting block: the requested length from the window
// start, or the whole window when the visitor named no length. The owner can
// change it, and the backend re-checks whatever they send.
export function defaultSlot(request: VisitorRequest): {
  start: string;
  end: string;
  startAt: string;
  endAt: string;
} {
  const start = Date.parse(request.windowStartAt);
  const end =
    request.durationMinutes > 0
      ? Math.min(start + request.durationMinutes * 60_000, Date.parse(request.windowEndAt))
      : Date.parse(request.windowEndAt);
  return {
    start: civilMinute(start),
    end: civilMinute(end),
    startAt: new Date(start).toISOString(),
    endAt: new Date(end).toISOString(),
  };
}
