import type { OverviewData } from "./overview";

export const overviewFixture: OverviewData = {
  fixtureMode: true,
  state: "Likely awake",
  stateDetail: "Inside the observed waking range",
  timeSinceWake: "6 hr 20 min",
  nextSleepWindow: {
    label: "Today, 3:10 PM to 5:40 PM",
    uncertainty: "Expected range spans 2 hr 30 min",
  },
  drift: {
    label: "+42 min per cycle",
    direction: "Later than the previous cycle",
  },
  confidence: {
    level: "Medium",
    reason: "Based on 9 recent principal sleep episodes with variable timing",
  },
  usefulTaskWindow: {
    label: "Now to 1:45 PM",
    detail: "Two flexible tasks fit before the predicted sleep window",
  },
  sharingStatus: {
    active: true,
    label: "1 trusted view active",
    detail: "Predicted windows and confidence only",
  },
  updatedLabel: "Updated from synthetic observations 12 min ago",
};
