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
  // The estimator's internal confidence. ADR-0022 measured these buckets
  // inverted on real history (High 0.61 hit rate against Medium 0.81), so the
  // UI must not present the label as decision-relevant. It is kept because the
  // reasons behind it are still useful, and surfaced under Details.
  confidence: {
    level: ConfidenceLevel;
    reason: string;
  };

  // Freshness is the shared core/freshness verdict on whether `state` may be
  // trusted at all. It is what the user should read instead of a confidence
  // bucket: how recent the evidence is, and whether a claim is being withheld.
  freshness: {
    state: "current" | "stale" | "withheld";
    reason: string;
    explanation: string;
    ageLabel: string;
    trusted: boolean;
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

export const overviewUnavailable: OverviewData = {
  fixtureMode: false,
  status: "unavailable",
  empty: false,
  state: "Estimate unavailable",
  stateDetail: "Your records could not be read",
  timeSinceWake: "Unavailable",
  nextSleepWindow: { label: "Unavailable", uncertainty: "" },
  drift: { label: "Unavailable", direction: "" },
  confidence: { level: "Low", reason: "The desktop service could not load this view." },
  freshness: {
    state: "withheld",
    reason: "service_unavailable",
    explanation: "No current state is claimed until your records can be read again.",
    ageLabel: "",
    trusted: false,
  },
  usefulTaskWindow: { label: "Unavailable", detail: "" },
  sharingStatus: { active: false, label: "Sharing status unavailable", detail: "" },
  updatedLabel: "Refresh to try again",
};
