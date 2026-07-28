// Synced backend proposals (approvals unification, ADR-0016): the desktop
// lists the self-hosted backend's assistant/agent proposals and decides them
// with the one-use token. With sync off this whole surface is absent.

import { findWailsMethod, type WailsRoot } from "./wailsBridge";

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
  decisionToken?: string;
}

export interface BackendProposalPagination {
  nextCursor: string;
  hasMore: boolean;
}

export interface BackendProposalsData {
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

function terminalPagination(): BackendProposalPagination {
  return { nextCursor: "", hasMore: false };
}

function normalizePagination(
  value: unknown,
  required: boolean,
): BackendProposalPagination | undefined {
  if (value === undefined && !required) return terminalPagination();
  if (!isRecord(value)) return undefined;
  const { nextCursor, hasMore } = value;
  if (typeof nextCursor !== "string" || typeof hasMore !== "boolean") return undefined;
  if (hasMore !== nextCursor.length > 0) return undefined;
  return { nextCursor, hasMore };
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
  if (!proposalId || !action || !status || !title || !window || !createdLabel || !expiresLabel) {
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
    ...(decisionToken ? { decisionToken } : {}),
  };
}

function normalizeBackendProposalData(
  value: unknown,
  paginationRequired: boolean,
): BackendProposalsData | undefined {
  if (!isRecord(value)) return undefined;
  const status = value.status;
  if (status !== "off" && status !== "ok" && status !== "error") return undefined;
  const pagination = normalizePagination(value.pagination, paginationRequired);
  if (!pagination) return undefined;
  const proposals: BackendProposal[] = [];
  if (Array.isArray(value.proposals)) {
    for (const item of value.proposals) {
      const proposal = normalizeProposal(item);
      if (!proposal) return undefined;
      proposals.push(proposal);
    }
  }
  const message = str(value.message);
  return { status, ...(message ? { message } : {}), proposals, pagination };
}

export function normalizeBackendProposals(value: unknown): BackendProposalsData | undefined {
  return normalizeBackendProposalData(value, false);
}

export function normalizeBackendProposalPage(value: unknown): BackendProposalsData | undefined {
  return normalizeBackendProposalData(value, true);
}

const offline: BackendProposalsData = {
  status: "off",
  proposals: [],
  pagination: terminalPagination(),
};

export async function loadBackendProposals(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendProposalsData> {
  const method = findWailsMethod(root, ["GetBackendProposals"]);
  if (!method) return offline;
  try {
    const normalized = normalizeBackendProposals(await method());
    if (normalized) return normalized;
  } catch {
    // Treat a failing bridge like an unreachable backend below.
  }
  return {
    status: "error",
    message: "Could not reach the synced backend.",
    proposals: [],
    pagination: terminalPagination(),
  };
}

export async function loadBackendProposalPage(
  cursor: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendProposalsData> {
  const unavailable: BackendProposalsData = {
    status: "error",
    message: "Could not load older synced proposals.",
    proposals: [],
    pagination: terminalPagination(),
  };
  if (cursor.length === 0) return unavailable;
  const method = findWailsMethod(root, ["GetBackendProposalPage"]);
  if (!method) return unavailable;
  try {
    const normalized = normalizeBackendProposalPage(await method({ cursor }));
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
  if (!method) return offline;
  try {
    const normalized = normalizeBackendProposals(await method(input));
    if (normalized) return normalized;
  } catch {
    // Fall through to the error state; the one-use token stays valid until the
    // backend actually consumes it.
  }
  return {
    status: "error",
    message: "The decision could not be recorded.",
    proposals: [],
    pagination: terminalPagination(),
  };
}
