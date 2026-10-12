import {
  accountActivityChangedEvent,
  loadAccountActivity,
  noAccountActivity,
  type ActivityAgreement,
} from "../data/accountActivity";
import { sleepDataChangedEvent } from "../data/sleepDataEvents";
import { useLoaded } from "../state/useLoaded";
import { ActivityNightsFigure } from "./ActivityNightsFigure";
import { Icon } from "./Icon";

// Rhythm › Sources, once a Google export has been imported: the nights its
// gaps suggest beside the recorded ones, and how well they agree where both
// exist. Measured only on recorded nights; a night nobody recorded says nothing
// about whether a suggestion was right.

function Agreement({ agreement }: { agreement?: ActivityAgreement }) {
  if (!agreement) {
    return (
      <p className="activity-nights-agreement">
        You recorded no nights while this activity was kept, so there is nothing yet to measure the
        suggestions against.
      </p>
    );
  }
  const { recorded, matched, summary } = agreement;
  const nights = recorded === 1 ? "night" : "nights";
  if (matched === 0) {
    return (
      <p className="activity-nights-agreement">
        {recorded === 1
          ? "The night you recorded while this activity was kept had no suggestion beside it."
          : `None of the ${recorded} nights you recorded while this activity was kept had a suggestion beside it.`}
      </p>
    );
  }
  return (
    <p className="activity-nights-agreement">
      <span className="count">
        {matched} of {recorded}
      </span>{" "}
      {nights} you recorded while this activity was kept had a suggestion beside{" "}
      {recorded === 1 ? "it" : "them"}. {summary}
    </p>
  );
}

export function ActivityNightsPanel() {
  const { data } = useLoaded(loadAccountActivity, {
    events: [accountActivityChangedEvent, sleepDataChangedEvent],
    fallback: () => noAccountActivity,
  });
  if (!data?.imported) return null;
  return (
    <section className="panel activity-nights-panel" aria-labelledby="activity-nights-title">
      <div className="panel-heading">
        <div>
          <p className="section-kicker">In the shadow</p>
          <h2 id="activity-nights-title">Nights your Google activity suggests</h2>
        </div>
        <a href="#/data-sources">
          Imported activity <Icon name="chevron" />
        </a>
      </div>
      <ActivityNightsFigure activity={data} />
      <Agreement agreement={data.agreement} />
      <p className="diff-note">
        Suggestions stay on this page. They are not in your forecast or plans, and no link or
        assistant sees them.
      </p>
    </section>
  );
}
