import { normalizeProposal, type BackendProposal } from "./backendProposals";

// Assistant chat surface (spec §4, ADR-0010): every reply is propose-only —
// proposals land in the same approval queue, and the desktop sends only a
// redacted planning context (task ids and bounds; titles never leave the
// device — the Go binding re-attaches them locally for display).

export interface AssistantStatus {
  enabled: boolean;
  configured: boolean;
  provider?: string;
  model?: string;
  message?: string;
}

export type AssistantResult =
  | "answer_only"
  | "proposal_pending"
  | "refused_medical"
  | "unknown"
  | "unavailable";

export interface AssistantReply {
  available: boolean;
  result: AssistantResult;
  answer: string;
  configured: boolean;
  provider?: string;
  model?: string;
  proposals: BackendProposal[];
}

type WailsMethod = (payload?: unknown) => Promise<unknown>;
type UnknownRecord = Record<string, unknown>;

interface WailsRoot {
  go?: Record<string, Record<string, Record<string, unknown>>>;
}

export const assistantUnavailableMessage =
  "The assistant needs the ZeitBoard desktop app with your self-hosted backend connected.";

const browserStatus: AssistantStatus = {
  enabled: false,
  configured: false,
  message: assistantUnavailableMessage,
};

function findMethod(root: WailsRoot, names: readonly string[]): WailsMethod | undefined {
  const packages = root.go;
  if (!packages) return undefined;
  for (const packageValue of Object.values(packages)) {
    for (const serviceValue of Object.values(packageValue)) {
      for (const name of names) {
        const candidate = serviceValue[name];
        if (typeof candidate === "function") {
          return (candidate as WailsMethod).bind(serviceValue);
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

function result(value: unknown): AssistantResult {
  if (
    value === "answer_only" ||
    value === "proposal_pending" ||
    value === "refused_medical" ||
    value === "unavailable"
  ) {
    return value;
  }
  return "unknown";
}

export function normalizeAssistantStatus(value: unknown): AssistantStatus | undefined {
  if (!isRecord(value) || typeof value.enabled !== "boolean") return undefined;
  const provider = str(value.provider);
  const model = str(value.model);
  const message = str(value.message);
  return {
    enabled: value.enabled,
    configured: value.configured === true,
    ...(provider ? { provider } : {}),
    ...(model ? { model } : {}),
    ...(message ? { message } : {}),
  };
}

export function normalizeAssistantReply(value: unknown): AssistantReply | undefined {
  if (!isRecord(value) || typeof value.available !== "boolean") return undefined;
  const answer = str(value.answer) ?? "";
  const provider = str(value.provider);
  const model = str(value.model);
  const proposals: BackendProposal[] = [];
  if (Array.isArray(value.proposals)) {
    for (const item of value.proposals) {
      const proposal = normalizeProposal(item);
      if (!proposal) return undefined;
      proposals.push(proposal);
    }
  }
  return {
    available: value.available,
    result: result(value.result),
    answer,
    configured: value.configured === true,
    ...(provider ? { provider } : {}),
    ...(model ? { model } : {}),
    proposals,
  };
}

export async function loadAssistantStatus(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AssistantStatus> {
  const method = findMethod(root, ["GetAssistantStatus"]);
  if (!method) return browserStatus;
  try {
    const normalized = normalizeAssistantStatus(await method());
    if (normalized) return normalized;
  } catch {
    // fall through
  }
  return browserStatus;
}

export async function sendAssistantMessage(
  message: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AssistantReply> {
  const method = findMethod(root, ["SendAssistantMessage"]);
  if (!method) {
    return {
      available: false,
      result: "unavailable",
      answer: assistantUnavailableMessage,
      configured: false,
      proposals: [],
    };
  }
  const normalized = normalizeAssistantReply(await method({ message }));
  if (!normalized) throw new Error("The assistant reply could not be read.");
  return normalized;
}
