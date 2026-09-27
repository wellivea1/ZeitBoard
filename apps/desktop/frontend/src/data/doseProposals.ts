// Doses an agent asked to record (ADR-0051). They wait on this computer until
// the owner accepts one, which records it as their own dose, or declines it,
// or it lapses after a day. Without the desktop service there are none.

import { dayInSentence, clockTime } from "../utils/relativeTime";
import { findWailsMethod, hasDesktopBridge, type WailsRoot } from "./wailsBridge";

export const doseProposalsChangedEvent = "zeitboard:dose-proposals-changed";

export type DoseProposalState = "pending" | "recorded" | "discarded" | "expired";

export interface DoseProposal {
  proposalId: string;
  /** "Record dose": the card title, from the action registry. */
  title: string;
  medicationId: string;
  /** Private; shown on this computer only. */
  medicationLabel: string;
  status: "taken" | "skipped";
  /** The instant, ISO 8601. */
  doseAt: string;
  zoneId: string;
  state: DoseProposalState;
  createdAt: string;
  expiresAt: string;
  decidedAt?: string;
}

export interface DoseProposalsData {
  status: "off" | "ok" | "error";
  message?: string;
  pending: DoseProposal[];
  history: DoseProposal[];
  nextExpiryAt: string;
}

export const noDoseProposals: DoseProposalsData = {
  status: "off",
  pending: [],
  history: [],
  nextExpiryAt: "",
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function text(value: unknown) {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function instant(value: unknown) {
  const parsed = text(value);
  return parsed && Number.isFinite(Date.parse(parsed)) ? parsed : undefined;
}

const states: readonly DoseProposalState[] = ["pending", "recorded", "discarded", "expired"];

export function normalizeDoseProposal(value: unknown): DoseProposal | undefined {
  if (!isRecord(value)) return undefined;
  const proposalId = text(value.proposalId);
  const title = text(value.title);
  const medicationId = text(value.medicationId);
  const zoneId = text(value.zoneId);
  const doseAt = instant(value.doseAt);
  const createdAt = instant(value.createdAt);
  const expiresAt = instant(value.expiresAt);
  const status = value.status === "taken" || value.status === "skipped" ? value.status : undefined;
  const state = states.find((candidate) => candidate === value.state);
  const decidedAt = instant(value.decidedAt);
  if (
    !proposalId ||
    !title ||
    !medicationId ||
    !zoneId ||
    !doseAt ||
    !createdAt ||
    !expiresAt ||
    !status ||
    !state ||
    typeof value.medicationLabel !== "string"
  )
    return undefined;
  return {
    proposalId,
    title,
    medicationId,
    medicationLabel: value.medicationLabel,
    status,
    doseAt,
    zoneId,
    state,
    createdAt,
    expiresAt,
    ...(decidedAt ? { decidedAt } : {}),
  };
}

export function normalizeDoseProposals(value: unknown): DoseProposalsData | undefined {
  if (!isRecord(value) || !Array.isArray(value.pending) || !Array.isArray(value.history))
    return undefined;
  const pending = value.pending.map(normalizeDoseProposal);
  const history = value.history.map(normalizeDoseProposal);
  const nextExpiryAt = value.nextExpiryAt === "" ? "" : instant(value.nextExpiryAt);
  if (pending.some((item) => !item) || history.some((item) => !item) || nextExpiryAt === undefined)
    return undefined;
  return {
    status: "ok",
    pending: pending as DoseProposal[],
    history: history as DoseProposal[],
    nextExpiryAt,
  };
}

const unavailable = (message: string): DoseProposalsData => ({
  ...noDoseProposals,
  status: "error",
  message,
});

export async function loadDoseProposals(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<DoseProposalsData> {
  const method = findWailsMethod(root, ["GetDoseProposals"]);
  if (!method)
    return hasDesktopBridge(root)
      ? unavailable(
          "The desktop service for proposed doses is unavailable. Restart the app to retry.",
        )
      : noDoseProposals;
  try {
    const normalized = normalizeDoseProposals(await method());
    if (normalized) return normalized;
  } catch {
    // Reported below.
  }
  return unavailable("Could not read the proposed doses.");
}

/** Accepting records the dose as the owner's own; declining records nothing. */
export async function decideDoseProposal(
  proposalId: string,
  decision: "approved" | "rejected",
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<DoseProposalsData> {
  const method = findWailsMethod(root, ["DecideDoseProposal"]);
  if (!method) throw new Error("Proposed doses need the ZeitBoard desktop service.");
  const normalized = normalizeDoseProposals(
    await method({ proposalId, decision: decision === "approved" ? "record" : "discard" }),
  );
  if (!normalized) throw new Error("The desktop service returned an invalid response.");
  return normalized;
}

export function doseIsWaiting(proposal: DoseProposal, now = Date.now()) {
  return proposal.state === "pending" && Date.parse(proposal.expiresAt) > now;
}

/** "taken today at 9:10 PM", on this computer's clock. */
export function proposedDoseWording(proposal: DoseProposal, now: Date) {
  const at = new Date(proposal.doseAt);
  return `${proposal.status} ${dayInSentence(at, now)} at ${clockTime(at)}`;
}

/** The medication's name, or a stand-in if it has none on this computer. */
export function proposedDoseMedication(proposal: DoseProposal) {
  return proposal.medicationLabel || "A medication";
}
