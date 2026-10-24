import { normalizeProposal, type BackendProposal } from "./backendProposals";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

// Assistant chat surface (spec §4, ADR-0010): every reply is propose-only —
// proposals land in the same approval queue, and the desktop sends only a
// redacted planning context (task ids and bounds; titles never leave the
// device — the Go binding re-attaches them locally for display).

export interface AssistantStatus {
  enabled: boolean;
  configured: boolean;
  provider?: string;
  model?: string;
  /** "chatgpt_plan" when the owner's ChatGPT plan pays for the answers. */
  auth?: string;
  /** What a ChatGPT sign-in still needs, while it is not configured. */
  problem?: AssistantReason;
  message?: string;
}

/**
 * Why the provider gave no answer, in terms the owner can act on (ADR-0066).
 * The server says the same in its answer; the rail adds what to do.
 */
export type AssistantReason =
  | "usage_limit"
  | "busy"
  | "sign_in_needed"
  | "sign_in_ended"
  | "plan_not_shared"
  | "not_eligible"
  | "not_enabled"
  | "model_needed"
  | "model_unavailable"
  | "refused";

const reasons: readonly AssistantReason[] = [
  "usage_limit",
  "busy",
  "sign_in_needed",
  "sign_in_ended",
  "plan_not_shared",
  "not_eligible",
  "not_enabled",
  "model_needed",
  "model_unavailable",
  "refused",
];

function reason(value: unknown): AssistantReason | undefined {
  return reasons.find((known) => known === value);
}

/** Whether the owner's ChatGPT plan pays for the assistant. */
export function usesChatGPTPlan(status: Pick<AssistantStatus, "auth"> | undefined): boolean {
  return status?.auth === "chatgpt_plan";
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
  auth?: string;
  reason?: AssistantReason;
  proposals: BackendProposal[];
}

type UnknownRecord = Record<string, unknown>;

export const assistantUnavailableMessage =
  "The assistant needs the ZeitBoard desktop app with your self-hosted backend connected.";

const browserStatus: AssistantStatus = {
  enabled: false,
  configured: false,
  message: assistantUnavailableMessage,
};

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
  const auth = str(value.auth);
  const problem = reason(value.problem);
  const message = str(value.message);
  return {
    enabled: value.enabled,
    configured: value.configured === true,
    ...(provider ? { provider } : {}),
    ...(model ? { model } : {}),
    ...(auth ? { auth } : {}),
    ...(problem ? { problem } : {}),
    ...(message ? { message } : {}),
  };
}

export function normalizeAssistantReply(value: unknown): AssistantReply | undefined {
  if (!isRecord(value) || typeof value.available !== "boolean") return undefined;
  const answer = str(value.answer) ?? "";
  const provider = str(value.provider);
  const model = str(value.model);
  const auth = str(value.auth);
  const why = reason(value.reason);
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
    ...(auth ? { auth } : {}),
    ...(why ? { reason: why } : {}),
    proposals,
  };
}

export async function loadAssistantStatus(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AssistantStatus> {
  const method = findWailsMethod(root, ["GetAssistantStatus"]);
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
  const method = findWailsMethod(root, ["SendAssistantMessage"]);
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

// The assistant on the owner's ChatGPT plan (ADR-0066). The server holds the
// sign-in; the desktop opens it in the browser and relays the browser's
// answer, and this window only ever sees where it stands.

export type AssistantConnectionState = "disconnected" | "connected" | "sign_in_ended";

export interface AssistantConnection {
  /** False when there is no server to ask; message says why. */
  reachable: boolean;
  /** False when the server's configuration names a provider of its own. */
  offered: boolean;
  state?: AssistantConnectionState;
  email?: string;
  model?: string;
  problem?: string;
  message?: string;
  /** Answers a disconnection: false when ChatGPT did not confirm it. */
  revoked?: boolean;
}

export interface AssistantModel {
  slug: string;
  displayName: string;
}

/** Sent when the sign-in or its model changes, so an open rail reads again. */
export const assistantConnectionChangedEvent = "zeitboard:assistant-connection-changed";

function connectionState(value: unknown): AssistantConnectionState | undefined {
  return value === "disconnected" || value === "connected" || value === "sign_in_ended"
    ? value
    : undefined;
}

export function normalizeAssistantConnection(value: unknown): AssistantConnection | undefined {
  if (!isRecord(value) || typeof value.reachable !== "boolean") return undefined;
  const state = connectionState(value.state);
  const email = str(value.email);
  const model = str(value.model);
  const problem = str(value.problem);
  const message = str(value.message);
  return {
    reachable: value.reachable,
    offered: value.offered === true,
    ...(state ? { state } : {}),
    ...(email ? { email } : {}),
    ...(model ? { model } : {}),
    ...(problem ? { problem } : {}),
    ...(message ? { message } : {}),
    ...(typeof value.revoked === "boolean" ? { revoked: value.revoked } : {}),
  };
}

async function callAssistant(name: string, root: WailsRoot, input?: unknown): Promise<unknown> {
  const method = findWailsMethod(root, [name]);
  if (!method) throw new Error(assistantUnavailableMessage);
  return method(input);
}

async function connectionFrom(name: string, root: WailsRoot, input?: unknown) {
  const connection = normalizeAssistantConnection(await callAssistant(name, root, input));
  if (!connection) throw new Error("The assistant's sign-in could not be read.");
  window.dispatchEvent(new Event(assistantConnectionChangedEvent));
  return connection;
}

/** Where the sign-in stands; null in a window without the desktop app. */
export async function loadAssistantConnection(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AssistantConnection | null> {
  const method = findWailsMethod(root, ["GetAssistantConnection"]);
  if (!method) return null;
  const connection = normalizeAssistantConnection(await method());
  if (!connection) throw new Error("The assistant's sign-in could not be read.");
  return connection;
}

/**
 * Opens ChatGPT's sign-in in the browser and waits, for up to ten minutes,
 * for it to come back.
 */
export function connectChatGPT(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return connectionFrom("ConnectChatGPT", root);
}

export async function cancelChatGPTSignIn(root: WailsRoot = globalThis as unknown as WailsRoot) {
  await callAssistant("CancelChatGPTSignIn", root);
}

export function disconnectChatGPT(root: WailsRoot = globalThis as unknown as WailsRoot) {
  return connectionFrom("DisconnectChatGPT", root);
}

export function chooseAssistantModel(
  model: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  return connectionFrom("ChooseAssistantModel", root, { model });
}

export async function listAssistantModels(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AssistantModel[]> {
  const value = await callAssistant("ListAssistantModels", root);
  if (!Array.isArray(value)) throw new Error("The plan's models could not be read.");
  return value.flatMap((item: unknown) => {
    if (!isRecord(item)) return [];
    const slug = str(item.slug);
    const displayName = str(item.displayName);
    return slug && displayName ? [{ slug, displayName }] : [];
  });
}

/** Opens ChatGPT's usage settings, or OpenAI's help, in the browser. */
export async function openAssistantPage(
  page: "usage" | "help",
  root: WailsRoot = globalThis as unknown as WailsRoot,
) {
  await callAssistant("OpenAssistantPage", root, { page });
}
