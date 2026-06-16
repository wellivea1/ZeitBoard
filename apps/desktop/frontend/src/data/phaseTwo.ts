import type { ConfidenceLevel } from "./overview";

export type ProposalOrigin = "scheduler" | "assistant" | "sync_conflict";

export interface ChangeProposalFixture {
  id: string;
  origin: ProposalOrigin;
  kind: "Move" | "Place" | "Reminder";
  title: string;
  from?: string;
  to: string;
  rhythmContext: string;
  confidence: ConfidenceLevel;
  explanationCodes: string[];
  reasonLabels: string[];
  createdLabel: string;
  expiresLabel: string;
}

export interface SourceConflictFixture {
  id: string;
  source: string;
  state: "conflicting" | "stale" | "permission-revoked" | "missing";
  title: string;
  detail: string;
  nextAction: string;
}

export const proposalFixtures: ChangeProposalFixture[] = [
  {
    id: "proposal-email-okafor",
    origin: "scheduler",
    kind: "Move",
    title: "Email Dr. Okafor",
    from: "Tue 11:40 AM to 12:10 PM",
    to: "Tue 3:10 PM to 3:40 PM",
    rhythmContext: "about 4 hr after wake",
    confidence: "Medium",
    explanationCodes: ["within_predicted_waking_window", "avoids_fixed_event"],
    reasonLabels: ["In a likely-awake window", "Avoids a fixed appointment"],
    createdLabel: "Proposed by Scheduler",
    expiresLabel: "expires in 18 hr",
  },
  {
    id: "proposal-taxes",
    origin: "assistant",
    kind: "Place",
    title: "Taxes focus block",
    to: "Thu 2:10 PM to 3:40 PM",
    rhythmContext: "not right after wake",
    confidence: "High",
    explanationCodes: ["within_predicted_waking_window", "uncertainty_buffer_applied"],
    reasonLabels: ["In a likely-awake window", "Kept a buffer from window edges"],
    createdLabel: "Proposed by Assistant",
    expiresLabel: "expires in 2 days",
  },
];

export const unplacedTaskFixture = {
  title: "Call service provider",
  reason: "No safe proposal yet: fixed office hours overlap the predicted sleep window.",
  nextAction: "Keep manual until the next estimate refresh.",
};

export const sourceConflictFixtures: SourceConflictFixture[] = [
  {
    id: "wearable-desktop-conflict",
    source: "Wearable sleep",
    state: "conflicting",
    title: "Wearable sleep overlaps desktop activity",
    detail: "Sleep import says 2:20 AM to 10:10 AM; desktop activity shows use at 8:48 AM.",
    nextAction: "Review before this interval affects estimates.",
  },
  {
    id: "phone-permission-missing",
    source: "Phone activity",
    state: "permission-revoked",
    title: "Permission missing",
    detail: "Activity support is off, so this source is excluded rather than inferred.",
    nextAction: "Reconnect only if you want activity as supporting evidence.",
  },
  {
    id: "calendar-stale",
    source: "Local calendar",
    state: "stale",
    title: "Last sync is stale",
    detail: "Calendar availability was last checked 26 hr ago.",
    nextAction: "Refresh before approving proposals near fixed events.",
  },
];

export const correctionPreviewFixture = {
  title: "Correction inspector",
  sourceInterval: "Imported sleep: Jun 13, 2:20 AM to 10:10 AM",
  effectiveInterval: "Corrected sleep: Jun 13, 3:05 AM to 9:35 AM",
  diffLabel: "Forecast moves 28 min later and confidence stays Medium.",
  historyLabel: "Edited 16 min ago from synthetic fixture data",
  undoLabel: "Undo correction",
};

export const refusalFixture = {
  code: "conflicting_observations",
  title: "Refusal rule",
  message:
    "When source conflicts are unresolved, ZeitBoard withholds the forecast band instead of drawing a misleading exact-looking estimate.",
  actions: ["Review source conflict", "Keep manual plan", "Refresh after correction"],
};
