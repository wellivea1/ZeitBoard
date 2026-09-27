// The words for deciding on a suggested or requested time, everywhere one is
// decided: Home, Plan, the Week board and visitor requests. The same act
// should never be "Reject" in one place and "Decline" in another.

export type Decision = "approved" | "rejected";

export function decisionButton(decision: Decision): string {
  return decision === "approved" ? "Accept" : "Decline";
}

/** The accessible name, which says what is being decided. */
export function decisionLabel(decision: Decision, title: string): string {
  return `${decisionButton(decision)} the suggested time for ${title}`;
}

/** The accessible name for deciding on a visitor's request. */
export function requestDecisionLabel(decision: Decision, who: string | undefined): string {
  const whose = who ? `${who}'s request` : "this request";
  return decision === "approved" ? `Accept the chosen time for ${whose}` : `Decline ${whose}`;
}

/** What is announced once the decision is recorded. */
export function decisionDone(decision: Decision, title: string): string {
  return `${decision === "approved" ? "Accepted" : "Declined"} ${title}.`;
}

/**
 * The accessible name for deciding on what an assistant asked for:
 * "Accept and record the dose of Melatonin", "Decline the task “Taxes”".
 */
export function proposalDecisionLabel(decision: Decision, action: string, subject: string): string {
  return decision === "approved"
    ? `${decisionButton("approved")} and ${action} ${subject}`
    : `${decisionButton("rejected")} ${subject}`;
}

/** What is announced once it is decided: "Recorded the dose of Melatonin." */
export function proposalDecisionDone(decision: Decision, done: string, subject: string): string {
  return decision === "approved" ? `${done} ${subject}.` : `Declined ${subject}.`;
}
