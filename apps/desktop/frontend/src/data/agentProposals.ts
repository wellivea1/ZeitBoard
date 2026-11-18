// What an agent asked for that waits on this computer (ADR-0051): a dose to
// record, a task to add, a status to report (ADR-0070). Accepting one makes the owner's own record of it;
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

export interface ProposedStatus {
  status: "awake" | "trying_to_sleep" | "do_not_disturb" | "cleared";
  /** "Trying to sleep", in the desktop's words. */
  label: string;
  /** How long it would stand; zero when it takes a status back. */
  minutes: number;
  /** What accepting it now would do, by the owner's own setting. */
  shared: boolean;
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
  | (AgentProposalBase & { kind: "task"; task: ProposedTask })
  | (AgentProposalBase & { kind: "status"; reported: ProposedStatus });

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

const reportable: readonly ProposedStatus["status"][] = [
  "awake",
  "trying_to_sleep",
  "do_not_disturb",
  "cleared",
];

function normalizeStatus(value: unknown): ProposedStatus | undefined {
  if (!isRecord(value)) return undefined;
  const status = reportable.find((candidate) => candidate === value.status);
  const label = text(value.label);
  const { minutes, shared } = value;
  if (
    !status ||
    !label ||
    typeof minutes !== "number" ||
    !Number.isSafeInteger(minutes) ||
    minutes < 0 ||
    minutes > 24 * 60 ||
    typeof shared !== "boolean"
  )
    return undefined;
  // Taking a status back lasts no time and shows nothing; anything else lasts some.
  if ((status === "cleared") !== (minutes === 0) || (status === "cleared" && shared))
    return undefined;
  return { status, label, minutes, shared };
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
  const reported = value.reported === undefined ? undefined : normalizeStatus(value.reported);
  // Exactly one kind, and it must read.
  const kinds = [value.dose, value.task, value.reported].filter((kind) => kind !== undefined);
  if (kinds.length !== 1) return undefined;
  if (dose) return { ...base, kind: "dose", dose };
  if (task) return { ...base, kind: "task", task };
  if (reported) return { ...base, kind: "status", reported };
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

/** "for 1 hour 30 minutes", as a status's length is said. */
function lasting(minutes: number) {
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  const parts = [
    hours > 0 && `${hours} hour${hours === 1 ? "" : "s"}`,
    rest > 0 && `${rest} minute${rest === 1 ? "" : "s"}`,
  ].filter(Boolean);
  return `for ${parts.join(" ")}`;
}

/**
 * "for 2 hours, kept to your own devices": how long a proposed status would
 * stand and who would see it. Who sees it is the owner's own setting; the
 * assistant cannot ask for it, so it is said before they accept.
 */
export function proposedStatusWording(reported: ProposedStatus) {
  if (reported.status === "cleared") return "takes back the status you reported";
  return `${lasting(reported.minutes)}, ${
    reported.shared ? "and share links you allowed may show it" : "kept to your own devices"
  }`;
}

/** What accepting does, as a verb and once done: "record", "Recorded". */
export function proposalAction(proposal: AgentProposal): { action: string; done: string } {
  switch (proposal.kind) {
    case "dose":
      return { action: "record", done: "Recorded" };
    case "task":
      return { action: "add", done: "Added" };
    case "status":
      return proposal.reported.status === "cleared"
        ? { action: "clear", done: "Cleared" }
        : { action: "report", done: "Reported" };
  }
}

/** What the proposal is about, by name: "the dose of Melatonin". */
export function proposalSubject(proposal: AgentProposal) {
  switch (proposal.kind) {
    case "dose":
      return `the dose of ${proposal.dose.medicationLabel || "a medication"}`;
    case "task":
      return `the task “${proposal.task.title}”`;
    case "status":
      return proposal.reported.status === "cleared"
        ? "your status"
        : `the status “${proposal.reported.label}”`;
  }
}

/** The heading a proposal goes by in a list: the medication, the task or the status. */
export function proposalName(proposal: AgentProposal) {
  switch (proposal.kind) {
    case "dose":
      return proposal.dose.medicationLabel || "A medication";
    case "task":
      return proposal.task.title;
    case "status":
      return proposal.reported.status === "cleared" ? "Clear your status" : proposal.reported.label;
  }
}
