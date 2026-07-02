// Synced backend proposals (approvals unification, ADR-0016): the desktop
// lists the self-hosted backend's assistant/agent proposals and decides them
// with the one-use token. With sync off this whole surface is absent.

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

export interface BackendProposalsData {
  status: "off" | "ok" | "error";
  message?: string;
  proposals: BackendProposal[];
}

type BackendMethod = (payload?: unknown) => Promise<unknown>;
type UnknownRecord = Record<string, unknown>;

interface WailsRoot {
  go?: Record<string, Record<string, Record<string, unknown>>>;
}

function findMethod(root: WailsRoot, names: readonly string[]): BackendMethod | undefined {
  const packages = root.go;
  if (!packages) return undefined;
  for (const packageValue of Object.values(packages)) {
    for (const serviceValue of Object.values(packageValue)) {
      for (const name of names) {
        const candidate = serviceValue[name];
        if (typeof candidate === "function") {
          return (candidate as BackendMethod).bind(serviceValue);
        }
      }
    }
  }
  return undefined;
}

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

function normalizeProposal(value: unknown): BackendProposal | undefined {
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

export function normalizeBackendProposals(value: unknown): BackendProposalsData | undefined {
  if (!isRecord(value)) return undefined;
  const status = value.status;
  if (status !== "off" && status !== "ok" && status !== "error") return undefined;
  const proposals: BackendProposal[] = [];
  if (Array.isArray(value.proposals)) {
    for (const item of value.proposals) {
      const proposal = normalizeProposal(item);
      if (!proposal) return undefined;
      proposals.push(proposal);
    }
  }
  const message = str(value.message);
  return { status, ...(message ? { message } : {}), proposals };
}

const offline: BackendProposalsData = { status: "off", proposals: [] };

export async function loadBackendProposals(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendProposalsData> {
  const method = findMethod(root, ["GetBackendProposals"]);
  if (!method) return offline;
  try {
    const normalized = normalizeBackendProposals(await method());
    if (normalized) return normalized;
  } catch {
    // Treat a failing bridge like an unreachable backend below.
  }
  return { status: "error", message: "Could not reach the synced backend.", proposals: [] };
}

export async function decideBackendProposal(
  input: { proposalId: string; decision: "approved" | "rejected"; token: string },
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<BackendProposalsData> {
  const method = findMethod(root, ["DecideBackendProposal"]);
  if (!method) return offline;
  try {
    const normalized = normalizeBackendProposals(await method(input));
    if (normalized) return normalized;
  } catch {
    // Fall through to the error state; the one-use token stays valid until the
    // backend actually consumes it.
  }
  return { status: "error", message: "The decision could not be recorded.", proposals: [] };
}
