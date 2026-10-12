import { useState } from "react";
import {
  accountActivityChangedEvent,
  loadAccountActivity,
  noAccountActivity,
} from "../data/accountActivity";
import {
  loadComputerNights,
  noComputerNights,
  type NightsAgreement,
  type NightsFigure,
} from "../data/activityNights";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { useLoaded } from "../state/useLoaded";
import { ActivityNightsFigure } from "./ActivityNightsFigure";
import { Icon } from "./Icon";

// Rhythm › Sources: the nights a source suggests beside the recorded ones, and
// how well they agree where both exist. The sources are a Google export
// (ADR-0059) and this computer (ADR-0060). Agreement is measured on recorded
// nights only; a night nobody recorded says nothing about a suggestion.

type SourceId = "google" | "computer";

interface NightsSource {
  id: SourceId;
  /** The switch's label. */
  label: string;
  /** Who suggests, inside a sentence. */
  words: string;
  /** The time the source covers, inside a sentence. */
  coverage: string;
  heading: string;
  link: { href: string; label: string };
  figure: NightsFigure;
}

function Agreement({ source }: { source: NightsSource }) {
  const { figure, coverage } = source;
  if (figure.nights === 0) {
    return (
      <p className="activity-nights-agreement">
        No stretch of three hours or more without use yet, so {source.words} suggests no nights.
      </p>
    );
  }
  const agreement: NightsAgreement | undefined = figure.agreement;
  if (!agreement) {
    return (
      <p className="activity-nights-agreement">
        You recorded no nights in {coverage}, so there is nothing yet to measure the suggestions
        against.
      </p>
    );
  }
  const { recorded, matched, summary } = agreement;
  if (matched === 0) {
    return (
      <p className="activity-nights-agreement">
        {recorded === 1
          ? `The night you recorded in ${coverage} had no suggestion beside it.`
          : `None of the ${recorded} nights you recorded in ${coverage} had a suggestion beside it.`}
      </p>
    );
  }
  return (
    <p className="activity-nights-agreement">
      <span className="count">
        {matched} of {recorded}
      </span>{" "}
      {recorded === 1 ? "night" : "nights"} you recorded in {coverage} had a suggestion beside{" "}
      {recorded === 1 ? "it" : "them"}. {summary}
    </p>
  );
}

function useNightsSources(): NightsSource[] {
  const { data: google } = useLoaded(loadAccountActivity, {
    events: [accountActivityChangedEvent, sleepDataChangedEvent],
    fallback: () => noAccountActivity,
  });
  const { data: computer } = useLoaded(loadComputerNights, {
    events: [sleepDataChangedEvent],
    expires: true,
    fallback: () => noComputerNights,
  });
  const sources: NightsSource[] = [];
  if (google?.figure) {
    sources.push({
      id: "google",
      label: "Google activity",
      words: "your Google activity",
      coverage: "the time the export covers",
      heading: "Nights your Google activity suggests",
      link: { href: "#/data-sources", label: "Imported activity" },
      figure: google.figure,
    });
  }
  if (computer?.figure) {
    sources.push({
      id: "computer",
      label: "This computer",
      words: "this computer",
      coverage: "the time this computer's record covers",
      heading: "Nights this computer suggests",
      link: { href: "#/settings/computer", label: "Desktop activity" },
      figure: computer.figure,
    });
  }
  return sources;
}

export function ActivityNightsPanel() {
  const sources = useNightsSources();
  const [chosen, setChosen] = useState<SourceId | null>(null);
  const source = sources.find((candidate) => candidate.id === chosen) ?? sources[0];
  if (!source) return null;
  return (
    <section className="panel activity-nights-panel" aria-labelledby="activity-nights-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">In the shadow</p>
          <h2 id="activity-nights-title">
            {sources.length > 1 ? "Nights your activity suggests" : source.heading}
          </h2>
        </div>
        <a href={source.link.href}>
          {source.link.label} <Icon name="chevron" />
        </a>
      </div>
      {sources.length > 1 && (
        <div className="activity-nights-sources" role="group" aria-label="Suggested by">
          {sources.map((candidate) => (
            <button
              className={`filter${candidate.id === source.id ? " active" : ""}`}
              type="button"
              aria-pressed={candidate.id === source.id}
              onClick={() => setChosen(candidate.id)}
              key={candidate.id}
            >
              {candidate.label}
            </button>
          ))}
        </div>
      )}
      <ActivityNightsFigure figure={source.figure} source={source.words} key={source.id} />
      <Agreement source={source} />
      <p className="diff-note">
        Suggestions stay on this page. They are not in your forecast or plans, and no link or
        assistant sees them.
      </p>
    </section>
  );
}
