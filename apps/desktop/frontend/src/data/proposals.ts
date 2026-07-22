import {
  proposalFixtures,
  unplacedTaskFixture,
  type ChangeProposalFixture,
  type ProposalOrigin,
} from "./phaseTwo";
import type { ConfidenceLevel } from "./overview";
import { findWailsMethod, type WailsRoot } from "./wailsBridge";

export interface UnplacedProposal {
  title: string;
  reason: string;
  nextAction: string;
  reasonCode?: string;
}

export type ProposalsSource = "local" | "fixture";
export type ProposalDecisionState = "pending" | "approved" | "rejected";

export interface ProposalRecord extends ChangeProposalFixture {
  decision: ProposalDecisionState;
  canUndo: boolean;
}

export interface ProposalsData {
  fixtureMode: boolean;
  status: "estimated" | "empty" | "refused" | "unavailable";
  refusal?: {
    code: string;
    message: string;
  };
  proposals: ProposalRecord[];
  unplaced: UnplacedProposal[];
}

export interface ProposalsResult {
  data: ProposalsData;
  source: ProposalsSource;
}

// Repackaged from the shared phaseTwo data so the offline shell renders the same
// shape the scheduler supplies.
export const proposalsFixture: ProposalsData = {
  fixtureMode: true,
  status: "estimated",
  proposals: proposalFixtures.map((proposal) => ({
    ...proposal,
    decision: "pending",
    canUndo: false,
  })),
  unplaced: [
    {
      title: unplacedTaskFixture.title,
      reason: unplacedTaskFixture.reason,
      nextAction: unplacedTaskFixture.nextAction,
    },
  ],
};

type UnknownRecord = Record<string, unknown>;

const methodNames = ["GetProposals", "Proposals"] as const;

export function hasLocalProposalService(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): boolean {
  return Boolean(findWailsMethod(root, methodNames));
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === "object" && value !== null;
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function strList(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const out: string[] = [];
  for (const item of value) {
    if (typeof item !== "string" || item.length === 0) return undefined;
    out.push(item);
  }
  return out;
}

const origins: ProposalOrigin[] = ["scheduler", "assistant", "sync_conflict"];
const kinds: ChangeProposalFixture["kind"][] = ["Move", "Place", "Reminder"];

function confidence(value: unknown): ConfidenceLevel | undefined {
  const normalized = str(value)?.toLowerCase();
  if (normalized === "low") return "Low";
  if (normalized === "medium" || normalized === "moderate") return "Medium";
  if (normalized === "high") return "High";
  return undefined;
}

function status(value: unknown): ProposalsData["status"] {
  if (value === "empty" || value === "refused" || value === "unavailable") return value;
  return "estimated";
}

function refusal(value: unknown): ProposalsData["refusal"] | undefined {
  if (!isRecord(value)) return undefined;
  const code = str(value.code);
  const message = str(value.message);
  return code && message ? { code, message } : undefined;
}

function proposal(value: unknown): ProposalRecord | undefined {
  if (!isRecord(value)) return undefined;
  const id = str(value.id);
  const origin = str(value.origin) as ProposalOrigin | undefined;
  const kind = str(value.kind) as ChangeProposalFixture["kind"] | undefined;
  const title = str(value.title);
  const to = str(value.to);
  const rhythmContext = str(value.rhythmContext);
  const level = confidence(value.confidence);
  const explanationCodes = strList(value.explanationCodes);
  const reasonLabels = strList(value.reasonLabels);
  const createdLabel = str(value.createdLabel);
  const expiresLabel = str(value.expiresLabel);
  const decision =
    value.decision === "approved" || value.decision === "rejected" || value.decision === "pending"
      ? value.decision
      : "pending";
  const canUndo = value.canUndo === true;
  if (
    !id ||
    !origin ||
    !origins.includes(origin) ||
    !kind ||
    !kinds.includes(kind) ||
    !title ||
    !to ||
    !rhythmContext ||
    !level ||
    !explanationCodes ||
    explanationCodes.length === 0 ||
    !reasonLabels ||
    !createdLabel ||
    !expiresLabel ||
    (decision === "pending" && canUndo) ||
    (decision !== "pending" && !canUndo)
  ) {
    return undefined;
  }
  const from = str(value.from);
  return {
    id,
    origin,
    kind,
    title,
    ...(from ? { from } : {}),
    to,
    rhythmContext,
    confidence: level,
    explanationCodes,
    reasonLabels,
    createdLabel,
    expiresLabel,
    decision,
    canUndo,
  };
}

function unplaced(value: unknown): UnplacedProposal | undefined {
  if (!isRecord(value)) return undefined;
  const title = str(value.title);
  const reason = str(value.reason);
  const nextAction = str(value.nextAction);
  if (!title || !reason || !nextAction) return undefined;
  const reasonCode = str(value.reasonCode);
  return { title, reason, nextAction, ...(reasonCode ? { reasonCode } : {}) };
}

function mapAll<T>(value: unknown, map: (item: unknown) => T | undefined): T[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const out: T[] = [];
  for (const item of value) {
    const next = map(item);
    if (!next) return undefined;
    out.push(next);
  }
  return out;
}

export function normalizeProposals(value: unknown): ProposalsData | undefined {
  if (!isRecord(value)) return undefined;
  const proposals = mapAll(value.proposals, proposal);
  const unplacedList = mapAll(value.unplaced, unplaced);
  if (!proposals || !unplacedList) return undefined;
  return {
    fixtureMode: value.fixtureMode === true,
    status: status(value.status),
    ...(refusal(value.refusal) ? { refusal: refusal(value.refusal) } : {}),
    proposals,
    unplaced: unplacedList,
  };
}

export async function loadProposals(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ProposalsResult> {
  const method = findWailsMethod(root, methodNames);
  if (!method) return { data: proposalsFixture, source: "fixture" };
  const result = await method();
  const proposals = normalizeProposals(result);
  if (!proposals) throw new Error("Proposal service returned an invalid response.");
  return { data: proposals, source: proposals.fixtureMode ? "fixture" : "local" };
}

export interface LocalProposalDecisionResult {
  proposalId: string;
  decision: "approved" | "rejected" | "undone";
  eventId?: string;
  message: string;
}

function normalizeDecisionResult(value: unknown): LocalProposalDecisionResult | undefined {
  if (!isRecord(value)) return undefined;
  const proposalId = str(value.proposalId);
  const decision =
    value.decision === "approved" || value.decision === "rejected" || value.decision === "undone"
      ? value.decision
      : undefined;
  const message = str(value.message);
  const eventId = str(value.eventId);
  if (!proposalId || !decision || !message) return undefined;
  return { proposalId, decision, message, ...(eventId ? { eventId } : {}) };
}

export async function decideLocalProposal(
  proposalId: string,
  decision: "approved" | "rejected",
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<LocalProposalDecisionResult> {
  const method = findWailsMethod(root, ["DecideLocalProposal"]);
  if (!method) throw new Error("Local proposal decisions require the ZeitBoard desktop service.");
  const result = normalizeDecisionResult(await method({ proposalId, decision }));
  if (!result || result.decision !== decision) {
    throw new Error("Local proposal decision returned an invalid response.");
  }
  return result;
}

export async function undoLocalProposalDecision(
  proposalId: string,
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<LocalProposalDecisionResult> {
  const method = findWailsMethod(root, ["UndoLocalProposalDecision"]);
  if (!method) throw new Error("Local proposal undo requires the ZeitBoard desktop service.");
  const result = normalizeDecisionResult(await method({ proposalId }));
  if (!result || result.decision !== "undone") {
    throw new Error("Local proposal undo returned an invalid response.");
  }
  return result;
}
