// Words for the state of an estimate, shared by every view that shows one, so
// Home and Rhythm cannot describe the same estimate or the same refusal
// differently.

export type EstimateSource = "local" | "synced" | "fixture";

export interface Refusal {
  code: string;
  message: string;
}

export interface NextStep {
  title: string;
  body: string;
  link?: { href: string; label: string };
}

export interface RefusalExplanation {
  /** What went wrong, in plain words, never the estimator's code. */
  sentence: string;
  /** The one step most likely to resolve it. */
  next: NextStep;
}

export function estimateSourceLabel(source: EstimateSource, hasEstimate: boolean): string {
  if (source === "synced") return hasEstimate ? "Synced estimate" : "Synced, awaiting estimate";
  if (source === "local") return hasEstimate ? "Local estimate" : "Local data";
  return "Sample data";
}

/** The estimator's own message, quoted once and ending with one full stop. */
export function inEstimatorWords(message: string): string {
  return `In the estimator's words: ${message.trim().replace(/[.\s]+$/, "")}.`;
}

// Each refusal the estimator can give, with the step most likely to resolve
// it. A shortage of nights is resolved by more nights; the rest say what went
// wrong.
export function explainRefusal(refusal: Refusal): RefusalExplanation {
  switch (refusal.code) {
    case "insufficient_data":
      return {
        sentence: "There are not enough nights yet to draw a forecast.",
        next: {
          title: "Add more nights",
          body: "Record tonight, or enter nights you remember.",
          link: { href: "#/log/sleep/add", label: "Add a past night" },
        },
      };
    case "ambiguous_cycle_index":
      return {
        sentence: "Two of your recorded sleeps are too far apart to count the cycles between them.",
        next: {
          title: "Fill the gap",
          body: "Add the nights in between, if you remember them.",
          link: { href: "#/log/sleep/add", label: "Add a past night" },
        },
      };
    case "unsupported_input":
      return {
        sentence: "Your records give a pattern this estimator has not been checked on.",
        next: {
          title: "Look over recent nights",
          body: "A date or time typed wrongly can cause this.",
          link: { href: "#/log/sleep", label: "Open the sleep log" },
        },
      };
    case "conflicting_observations":
      return {
        sentence: "Some of your records disagree with each other.",
        next: {
          title: "Settle which is right",
          body: "Review the nights that overlap in the sleep log.",
          link: { href: "#/log/sleep", label: "Open the sleep log" },
        },
      };
    default:
      return {
        sentence: refusal.message,
        next: {
          title: "Look over your records",
          body: "Nothing saved has changed.",
          link: { href: "#/log/sleep", label: "Open the sleep log" },
        },
      };
  }
}
