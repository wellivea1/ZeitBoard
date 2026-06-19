export type ConfidenceLevel = "Low" | "Medium" | "High";

export interface OverviewData {
  fixtureMode: boolean;
  status: "estimated" | "empty" | "refused" | "unavailable";
  empty: boolean;
  refusal?: {
    code: string;
    message: string;
  };
  state: string;
  stateDetail: string;
  timeSinceWake: string;
  nextSleepWindow: {
    label: string;
    uncertainty: string;
  };
  drift: {
    label: string;
    direction: string;
  };
  confidence: {
    level: ConfidenceLevel;
    reason: string;
  };
  usefulTaskWindow: {
    label: string;
    detail: string;
  };
  sharingStatus: {
    active: boolean;
    label: string;
    detail: string;
  };
  updatedLabel: string;
}

export type OverviewSource = "local" | "synced" | "fixture";

export interface OverviewResult {
  data: OverviewData;
  source: OverviewSource;
}
