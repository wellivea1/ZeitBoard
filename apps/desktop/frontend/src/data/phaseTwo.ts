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

export type RhythmBandKind = "observed" | "corrected" | "inferred" | "nap" | "forecast";

export interface RhythmSleepBandFixture {
  id: string;
  day: string;
  startHour: number;
  durationHours: number;
  kind: RhythmBandKind;
  startLabel: string;
  wakeLabel: string;
  durationLabel: string;
  source: string;
  confidence: ConfidenceLevel;
  originalStartHour?: number;
  originalDurationHours?: number;
  originalLabel?: string;
}

export interface RhythmDriftPointFixture {
  id: string;
  day: string;
  onsetHour: number;
  fitHour: number;
  bandLowHour: number;
  bandHighHour: number;
  onsetLabel: string;
  source: string;
  confidence: ConfidenceLevel;
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

export const rhythmActogramFixture = {
  summary:
    "Double-plotted actogram showing observed sleep drifting later with widening predicted sleep windows.",
  observedRows: [
    {
      id: "sleep-jun-15",
      day: "Jun 15",
      startHour: 21.7,
      durationHours: 7.4,
      kind: "observed",
      startLabel: "Jun 15, 9:42 PM",
      wakeLabel: "Jun 16, 5:06 AM",
      durationLabel: "7 hr 24 min",
      source: "Manual sleep log",
      confidence: "High",
    },
    {
      id: "sleep-jun-14",
      day: "Jun 14",
      startHour: 20.9,
      durationHours: 7.5,
      kind: "observed",
      startLabel: "Jun 14, 8:54 PM",
      wakeLabel: "Jun 15, 4:24 AM",
      durationLabel: "7 hr 30 min",
      source: "Wearable sleep",
      confidence: "High",
    },
    {
      id: "sleep-jun-13",
      day: "Jun 13",
      startHour: 20.1,
      durationHours: 6.5,
      kind: "corrected",
      startLabel: "Jun 13, 8:06 PM",
      wakeLabel: "Jun 14, 2:36 AM",
      durationLabel: "6 hr 30 min",
      source: "Manual correction",
      confidence: "Medium",
      originalStartHour: 19.35,
      originalDurationHours: 7.8,
      originalLabel: "Original import: Jun 13, 7:21 PM to Jun 14, 3:09 AM",
    },
    {
      id: "sleep-jun-12",
      day: "Jun 12",
      startHour: 19.35,
      durationHours: 7.2,
      kind: "observed",
      startLabel: "Jun 12, 7:21 PM",
      wakeLabel: "Jun 13, 2:33 AM",
      durationLabel: "7 hr 12 min",
      source: "Wearable sleep",
      confidence: "High",
    },
    {
      id: "sleep-jun-11",
      day: "Jun 11",
      startHour: 18.6,
      durationHours: 6.9,
      kind: "inferred",
      startLabel: "Jun 11, 6:36 PM",
      wakeLabel: "Jun 12, 1:30 AM",
      durationLabel: "6 hr 54 min",
      source: "Desktop inactivity",
      confidence: "Low",
    },
    {
      id: "sleep-jun-10",
      day: "Jun 10",
      startHour: 17.8,
      durationHours: 7.1,
      kind: "observed",
      startLabel: "Jun 10, 5:48 PM",
      wakeLabel: "Jun 11, 12:54 AM",
      durationLabel: "7 hr 6 min",
      source: "Manual sleep log",
      confidence: "High",
    },
  ] satisfies RhythmSleepBandFixture[],
  forecastRows: [
    {
      id: "forecast-jun-16",
      day: "Jun 16",
      startHour: 22.25,
      durationHours: 3.2,
      kind: "forecast",
      startLabel: "Jun 16, 10:15 PM earliest",
      wakeLabel: "Jun 17, 1:27 AM latest",
      durationLabel: "3 hr 12 min window",
      source: "Forecast cycle 1",
      confidence: "Medium",
    },
    {
      id: "forecast-jun-17",
      day: "Jun 17",
      startHour: 22.85,
      durationHours: 4.5,
      kind: "forecast",
      startLabel: "Jun 17, 10:51 PM earliest",
      wakeLabel: "Jun 18, 3:21 AM latest",
      durationLabel: "4 hr 30 min window",
      source: "Forecast cycle 2",
      confidence: "Medium",
    },
    {
      id: "forecast-jun-18",
      day: "Jun 18",
      startHour: 23.35,
      durationHours: 6.1,
      kind: "forecast",
      startLabel: "Jun 18, 11:21 PM earliest",
      wakeLabel: "Jun 19, 5:27 AM latest",
      durationLabel: "6 hr 6 min window",
      source: "Forecast cycle 3",
      confidence: "Low",
    },
  ] satisfies RhythmSleepBandFixture[],
  now: {
    label: "now",
    day: "Jun 15",
    hour: 13.5,
  },
};

export const rhythmDriftFixture = {
  title: "Sleep-onset drift",
  slopeLabel: "+48 min per cycle",
  confidence: "Medium" as ConfidenceLevel,
  yMinHour: 17,
  yMaxHour: 24,
  summary:
    "Sleep onset drifts later by about 48 minutes per observed sleep cycle with medium confidence.",
  points: [
    {
      id: "drift-jun-10",
      day: "Jun 10",
      onsetHour: 17.8,
      fitHour: 17.75,
      bandLowHour: 17.25,
      bandHighHour: 18.15,
      onsetLabel: "5:48 PM",
      source: "Manual sleep log",
      confidence: "High",
    },
    {
      id: "drift-jun-11",
      day: "Jun 11",
      onsetHour: 18.6,
      fitHour: 18.55,
      bandLowHour: 17.95,
      bandHighHour: 19.15,
      onsetLabel: "6:36 PM",
      source: "Desktop inactivity",
      confidence: "Low",
    },
    {
      id: "drift-jun-12",
      day: "Jun 12",
      onsetHour: 19.35,
      fitHour: 19.35,
      bandLowHour: 18.75,
      bandHighHour: 19.95,
      onsetLabel: "7:21 PM",
      source: "Wearable sleep",
      confidence: "High",
    },
    {
      id: "drift-jun-13",
      day: "Jun 13",
      onsetHour: 20.1,
      fitHour: 20.15,
      bandLowHour: 19.45,
      bandHighHour: 20.9,
      onsetLabel: "8:06 PM",
      source: "Manual correction",
      confidence: "Medium",
    },
    {
      id: "drift-jun-14",
      day: "Jun 14",
      onsetHour: 20.9,
      fitHour: 20.95,
      bandLowHour: 20.2,
      bandHighHour: 21.75,
      onsetLabel: "8:54 PM",
      source: "Wearable sleep",
      confidence: "High",
    },
    {
      id: "drift-jun-15",
      day: "Jun 15",
      onsetHour: 21.7,
      fitHour: 21.75,
      bandLowHour: 20.9,
      bandHighHour: 22.65,
      onsetLabel: "9:42 PM",
      source: "Manual sleep log",
      confidence: "High",
    },
  ] satisfies RhythmDriftPointFixture[],
};

export const correctionPreviewFixture = {
  title: "Correction inspector",
  sourceInterval: "Imported sleep: Jun 13, 2:20 AM to 10:10 AM",
  effectiveInterval: "Corrected sleep: Jun 13, 3:05 AM to 9:35 AM",
  diffLabel: "Forecast moves 28 min later and confidence stays Medium.",
  historyLabel: "Edited 16 min ago from synthetic fixture data",
};

export const refusalFixture = {
  code: "conflicting_observations",
  title: "Refusal rule",
  message:
    "When source conflicts are unresolved, ZeitBoard withholds the forecast band instead of drawing a misleading exact-looking estimate.",
  actions: ["Review source conflict", "Keep manual plan", "Refresh after correction"],
};
