import {
  proposalFixtures,
  unplacedTaskFixture,
  type ChangeProposalFixture,
  type ProposalOrigin,
} from "./phaseTwo";
import type { ConfidenceLevel } from "./overview";

export interface UnplacedProposal {
  title: string;
  reason: string;
  nextAction: string;
  reasonCode?: string;
}

export type ProposalsSource = "backend" | "fixture";

export interface ProposalsData {
  fixtureMode: boolean;
  proposals: ChangeProposalFixture[];
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
  proposals: proposalFixtures,
  unplaced: [
    {
      title: unplacedTaskFixture.title,
      reason: unplacedTaskFixture.reason,
      nextAction: unplacedTaskFixture.nextAction,
    },
  ],
};

type ProposalsMethod = () => Promise<unknown>;
type UnknownRecord = Record<string, unknown>;

interface WailsRoot {
  go?: Record<string, Record<string, Record<string, unknown>>>;
}

const methodNames = ["GetProposals", "Proposals"] as const;

function findProposalsMethod(root: WailsRoot): ProposalsMethod | undefined {
  const packages = root.go;
  if (!packages) return undefined;

  for (const packageValue of Object.values(packages)) {
    for (const serviceValue of Object.values(packageValue)) {
      for (const methodName of methodNames) {
        const candidate = serviceValue[methodName];
        if (typeof candidate === "function") {
          return (candidate as ProposalsMethod).bind(serviceValue);
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

function proposal(value: unknown): ChangeProposalFixture | undefined {
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
    !expiresLabel
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
    proposals,
    unplaced: unplacedList,
  };
}

export async function loadProposals(
  root: WailsRoot = globalThis as unknown as WailsRoot,
): Promise<ProposalsResult> {
  const method = findProposalsMethod(root);
  if (!method) return { data: proposalsFixture, source: "fixture" };

  try {
    const result = await method();
    const proposals = normalizeProposals(result);
    if (proposals) return { data: proposals, source: "backend" };
  } catch {
    // Fixture mode keeps the Approvals gate usable before the Wails service is ready.
  }

  return { data: proposalsFixture, source: "fixture" };
}
