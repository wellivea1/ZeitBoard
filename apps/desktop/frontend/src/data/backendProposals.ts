// Synced backend proposals (approvals unification, ADR-0016): the desktop
// lists the self-hosted backend's assistant/agent proposals and decides them
// with the one-use token. With sync off this whole surface is absent.

import {
  emptyReviewSummary,
  terminalReviewPage,
  normalizeReviewSummary,
  normalizeReviewPagination,
  type ReviewQueueSummary,
} from "./reviewQueue";

import { findWailsMethod, hasDesktopBridge, type WailsRoot } from "./wailsBridge";

export type BackendProposalStatus = "pending" | "approved" | "rejected";

export interface BackendProposal {
  proposalId: string;
  action: string;
  status: BackendProposalStatus;
  title: string;
  window: string;
  confidence: "Low" | "Medium" | "High";
  reasonLabels: string[];
  answer?: string;
  createdLabel: string;
  expiresLabel: string;
  expiresAt: string;
  decisionToken?: string;
}

export interface BackendProposalPagination {
  nextCursor: string;
  hasMore: boolean;
}

export interface BackendProposalsData extends ReviewQueueSummary {
  decisionRecorded?: boolean;
  status: "off" | "ok" | "error";
  message?: string;
  proposals: BackendProposal[];
  pagination: BackendProposalPagination;
}

type UnknownRecord = Record<string, unknown>;

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function confidence(value: unknown): BackendProposal["confidence"] {
  const normalized = str(value)?.toLowerCase();
  if (normalized === "high") return "High";
  if (normalized === "medium" || normalized === "moderate") return "Medium";
  return "Low";
}

function proposalStatus(value: unknown): BackendProposalStatus | undefined {
  if (value === "pending" || value === "approved" || value === "rejected") return value;
  return undefined;
}

export function normalizeProposal(value: unknown): BackendProposal | undefined {
  if (!isRecord(value)) return undefined;
  const proposalId = str(value.proposalId);
  const action = str(value.action);
  const status = proposalStatus(value.status);
  const title = str(value.title);
  const window = str(value.window);
  const createdLabel = str(value.createdLabel);
  const expiresLabel = str(value.expiresLabel);
  const expiresAt = str(value.expiresAt);
  if (
    !proposalId ||
    !action ||
    !status ||
    !title ||
    !window ||
    !createdLabel ||
    !expiresLabel ||
    !expiresAt ||
    !Number.isFinite(Date.parse(expiresAt))
  ) {
    return undefined;
  }
  const reasonLabels = Array.isArray(value.reasonLabels)
    ? value.reasonLabels.filter((item): item is string => typeof item === "string")
    : [];
  const answer = str(value.answer);
  const decisionToken = str(value.decisionToken);
  return {
    proposalId,
    action,
    status,
    title,
    window,
    confidence: confidence(value.confidence),
    reasonLabels,
    ...(answer ? { answer } : {}),
    createdLabel,
    expiresLabel,
    expiresAt,
    ...(decisionToken ? { decisionToken } : {}),
  };
}

export function normalizeBackendProposals(value: unknown): BackendProposalsData | undefined {
  if (!isRecord(value)) return undefined;
  const status = value.status;
  if (status !== "off" && status !== "ok" && status !== "error") return undefined;
  const pagination = normalizeReviewPagination(value.pagination);
  const summary = normalizeReviewSummary(value);
  if (!pagination || !summary) return undefined;
  if (!Array.isArray(value.proposals)) return undefined;
  const proposals: BackendProposal[] = [];
  {
    for (const item of value.proposals) {
      const proposal = normalizeProposal(item);
      if (!proposal) return undefined;
      proposals.push(proposal);
    }
  }
  const message = str(value.message);
  return {
    ...(value.decisionRecorded === true ? { decisionRecorded: true } : {}),
    ...summary,
    status,
    ...(message ? { message } : {}),
    proposals,
    pagination,
  };
}

const offline: BackendProposalsData = {
  ...emptyReviewSummary,
  status: "off",
  proposals: [],
  pagination: terminalReviewPage(),
};

export async function loadBackendProposals(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendProposalsData> {
  const method = findWailsMethod(root, ["GetBackendProposals"]);
  if (!method)
    return hasDesktopBridge(root)
      ? {
          ...offline,
          status: "error",
          message: "The desktop review service is unavailable. Restart the app to retry.",
        }
      : offline;
  try {
    const normalized = normalizeBackendProposals(await method());
    if (normalized) return normalized;
  } catch {
    // Treat a failing bridge like an unreachable backend below.
  }
  return {
    ...emptyReviewSummary,
    status: "error",
    message: "Could not reach the synced backend.",
    proposals: [],
    pagination: terminalReviewPage(),
  };
}

export async function loadBackendProposalPage(
  cursor: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendProposalsData> {
  const unavailable: BackendProposalsData = {
    ...emptyReviewSummary,
    status: "error",
    message: "Could not load older synced proposals.",
    proposals: [],
    pagination: terminalReviewPage(),
  };
  if (cursor.length === 0) return unavailable;
  const method = findWailsMethod(root, ["GetBackendProposalPage"]);
  if (!method) return unavailable;
  try {
    const normalized = normalizeBackendProposals(await method({ cursor }));
    if (normalized) return normalized;
  } catch {
    // Fall through to the non-destructive page error below.
  }
  return unavailable;
}
export async function decideBackendProposal(
  input: { proposalId: string; decision: "approved" | "rejected"; token: string },
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendProposalsData> {
  const method = findWailsMethod(root, ["DecideBackendProposal"]);
  if (!method)
    return hasDesktopBridge(root)
      ? {
          ...offline,
          status: "error",
          message: "The desktop review service is unavailable. Restart the app to retry.",
        }
      : offline;
  try {
    const normalized = normalizeBackendProposals(await method(input));
    if (normalized) return normalized;
  } catch {
    // Fall through to the error state; the one-use token stays valid until the
    // backend actually consumes it.
  }
  return {
    ...emptyReviewSummary,
    status: "error",
    message: "Could not confirm the decision. Refresh the queue before retrying.",
    proposals: [],
    pagination: terminalReviewPage(),
  };
}
