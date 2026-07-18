import type { OverviewData } from "./overview";

export const overviewFixture: OverviewData = {
  fixtureMode: true,
  status: "estimated",
  empty: false,
  state: "Likely awake",
  stateDetail: "Inside the observed waking range",
  timeSinceWake: "8 hr 24 min",
  nextSleepWindow: {
    label: "Today, 10:15 PM to 1:27 AM",
    uncertainty: "Expected range spans 3 hr 12 min",
  },
  drift: {
    label: "+48 min per cycle",
    direction: "Later than the previous cycle",
  },
  confidence: {
    level: "Medium",
    reason: "Based on 9 recent principal sleep episodes with variable timing",
  },
  usefulTaskWindow: {
    label: "Now to 9:40 PM",
    detail: "Two flexible tasks fit before the earliest predicted sleep time",
  },
  sharingStatus: {
    active: true,
    label: "1 trusted view active",
    detail: "Predicted windows and confidence only",
  },
  updatedLabel: "Updated from synthetic observations 12 min ago",
};
