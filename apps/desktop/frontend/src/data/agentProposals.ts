// What an agent asked for that waits on this computer (ADR-0051): a dose to
// record, a task to add. Accepting one makes the owner's own record of it;
// declining makes nothing, and after a day it lapses. Without the desktop
// service there are none.

import { clockTime, dayInSentence, relativeDay } from "../utils/relativeTime";
import { findWailsMethod, hasDesktopBridge, type WailsRoot } from "./wailsBridge";

export const agentProposalsChangedEvent = "zeitboard:agent-proposals-changed";

export type AgentProposalState = "pending" | "approved" | "rejected" | "expired";

export interface ProposedDose {
  medicationId: string;
  /** Private; shown on this computer only. */
  medicationLabel: string;
  status: "taken" | "skipped";
  /** The instant, ISO 8601. */
  doseAt: string;
  zoneId: string;
}

export interface ProposedTask {
  /** The owner's own words; shown on this computer only. */
  title: string;
  durationMinutes: number;
  earliestStartAt?: string;
  latestFinishAt?: string;
}

interface AgentProposalBase {
  proposalId: string;
  /** "Record dose", "Add task": the card title, from the action registry. */
  title: string;
  state: AgentProposalState;
  createdAt: string;
  expiresAt: string;
  decidedAt?: string;
}

export type AgentProposal =
  | (AgentProposalBase & { kind: "dose"; dose: ProposedDose })
  | (AgentProposalBase & { kind: "task"; task: ProposedTask });

export interface AgentProposalsData {
  status: "off" | "ok" | "error";
  message?: string;
  pending: AgentProposal[];
  history: AgentProposal[];
  nextExpiryAt: string;
}

export const noAgentProposals: AgentProposalsData = {
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

const states: readonly AgentProposalState[] = ["pending", "approved", "rejected", "expired"];

function normalizeDose(value: unknown): ProposedDose | undefined {
  if (!isRecord(value)) return undefined;
  const medicationId = text(value.medicationId);
  const doseAt = instant(value.doseAt);
  const zoneId = text(value.zoneId);
  const status = value.status === "taken" || value.status === "skipped" ? value.status : undefined;
  if (!medicationId || !doseAt || !zoneId || !status || typeof value.medicationLabel !== "string")
    return undefined;
  return { medicationId, medicationLabel: value.medicationLabel, status, doseAt, zoneId };
}

function normalizeTask(value: unknown): ProposedTask | undefined {
  if (!isRecord(value)) return undefined;
  const title = text(value.title);
  const { durationMinutes } = value;
  const earliestStartAt = value.earliestStartAt === undefined ? "" : instant(value.earliestStartAt);
  const latestFinishAt = value.latestFinishAt === undefined ? "" : instant(value.latestFinishAt);
  if (
    !title ||
    typeof durationMinutes !== "number" ||
    !Number.isSafeInteger(durationMinutes) ||
    durationMinutes <= 0 ||
    earliestStartAt === undefined ||
    latestFinishAt === undefined
  )
    return undefined;
  return {
    title,
    durationMinutes,
    ...(earliestStartAt ? { earliestStartAt } : {}),
    ...(latestFinishAt ? { latestFinishAt } : {}),
  };
}

export function normalizeAgentProposal(value: unknown): AgentProposal | undefined {
  if (!isRecord(value)) return undefined;
  const proposalId = text(value.proposalId);
  const title = text(value.title);
  const createdAt = instant(value.createdAt);
  const expiresAt = instant(value.expiresAt);
  const state = states.find((candidate) => candidate === value.state);
  const decidedAt = instant(value.decidedAt);
  if (!proposalId || !title || !createdAt || !expiresAt || !state) return undefined;
  const base = {
    proposalId,
    title,
    state,
    createdAt,
    expiresAt,
    ...(decidedAt ? { decidedAt } : {}),
  };
  const dose = value.dose === undefined ? undefined : normalizeDose(value.dose);
  const task = value.task === undefined ? undefined : normalizeTask(value.task);
  // Exactly one kind, and it must read.
  if ((value.dose === undefined) === (value.task === undefined)) return undefined;
  if (dose) return { ...base, kind: "dose", dose };
  if (task) return { ...base, kind: "task", task };
  return undefined;
}

export function normalizeAgentProposals(value: unknown): AgentProposalsData | undefined {
  if (!isRecord(value) || !Array.isArray(value.pending) || !Array.isArray(value.history))
    return undefined;
  const pending = value.pending.map(normalizeAgentProposal);
  const history = value.history.map(normalizeAgentProposal);
  const nextExpiryAt = value.nextExpiryAt === "" ? "" : instant(value.nextExpiryAt);
  if (pending.some((item) => !item) || history.some((item) => !item) || nextExpiryAt === undefined)
    return undefined;
  return {
    status: "ok",
    pending: pending as AgentProposal[],
    history: history as AgentProposal[],
    nextExpiryAt,
  };
}

const unavailable = (message: string): AgentProposalsData => ({
  ...noAgentProposals,
  status: "error",
  message,
});

export async function loadAgentProposals(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AgentProposalsData> {
  const method = findWailsMethod(root, ["GetAgentProposals"]);
  if (!method)
    return hasDesktopBridge(root)
      ? unavailable(
          "The desktop service for assistant proposals is unavailable. Restart the app to retry.",
        )
      : noAgentProposals;
  try {
    const normalized = normalizeAgentProposals(await method());
    if (normalized) return normalized;
  } catch {
    // Reported below.
  }
  return unavailable("Could not read the assistant's proposals.");
}

/** Accepting makes the owner's own record; declining makes nothing. */
export async function decideAgentProposal(
  proposalId: string,
  decision: "approved" | "rejected",
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<AgentProposalsData> {
  const method = findWailsMethod(root, ["DecideAgentProposal"]);
  if (!method) throw new Error("Assistant proposals need the ZeitBoard desktop service.");
  const normalized = normalizeAgentProposals(await method({ proposalId, decision }));
  if (!normalized) throw new Error("The desktop service returned an invalid response.");
  return normalized;
}

export function proposalIsWaiting(proposal: AgentProposal, now = Date.now()) {
  return proposal.state === "pending" && Date.parse(proposal.expiresAt) > now;
}

/** "taken today at 9:10 PM", on this computer's clock. */
export function proposedDoseWording(dose: ProposedDose, now: Date) {
  const at = new Date(dose.doseAt);
  return `${dose.status} ${dayInSentence(at, now)} at ${clockTime(at)}`;
}

/** "tomorrow at 5:00 PM", "Saturday at 9:00 AM": a moment after "by" or "from". */
function moment(value: string, now: Date) {
  const at = new Date(value);
  const day = relativeDay(at, now);
  const phrase = ["Today", "Tonight", "Tomorrow", "Yesterday"].includes(day)
    ? day.toLowerCase()
    : day;
  return `${phrase} at ${clockTime(at)}`;
}

/** "15 minutes, by Friday at 5:00 PM". */
export function proposedTaskWording(task: ProposedTask, now: Date) {
  const minutes = task.durationMinutes;
  const length =
    minutes % 60 === 0
      ? `${minutes / 60} hour${minutes === 60 ? "" : "s"}`
      : `${minutes} minute${minutes === 1 ? "" : "s"}`;
  const bounds = [
    task.earliestStartAt && `from ${moment(task.earliestStartAt, now)}`,
    task.latestFinishAt && `by ${moment(task.latestFinishAt, now)}`,
  ].filter(Boolean);
  return [length, ...bounds].join(", ");
}

/** What accepting does, as a verb and once done: "record", "Recorded". */
export function proposalAction(proposal: AgentProposal): { action: string; done: string } {
  return proposal.kind === "dose"
    ? { action: "record", done: "Recorded" }
    : { action: "add", done: "Added" };
}

/** What the proposal is about, by name: "the dose of Melatonin". */
export function proposalSubject(proposal: AgentProposal) {
  return proposal.kind === "dose"
    ? `the dose of ${proposal.dose.medicationLabel || "a medication"}`
    : `the task “${proposal.task.title}”`;
}

/** The heading a proposal goes by in a list: the medication or the task. */
export function proposalName(proposal: AgentProposal) {
  return proposal.kind === "dose"
    ? proposal.dose.medicationLabel || "A medication"
    : proposal.task.title;
}
