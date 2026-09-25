import type { OverviewData } from "../data/overview";

// Home before the first forecast. The estimator needs a week of main sleep, so
// this page counts the nights so far and lists the ways to add more, each
// going straight to where it is done. When the records exist but still give
// no forecast, it says why in plain words and what would change that.

interface Way {
  title: string;
  body: string;
  link?: { href: string; label: string };
}

const ways: Way[] = [
  {
    title: "Record from tonight",
    body: "Use the two buttons above when you next go to sleep and when you wake.",
  },
  {
    title: "Add nights you remember",
    body: "Enter each with the times you fell asleep and woke.",
    link: { href: "#/log/sleep/add", label: "Add a past night" },
  },
  {
    title: "Import what you have",
    body: "A JSON or CSV file in ZeitBoard's format, or a paper chart copied into its template.",
    link: { href: "#/data-sources", label: "Import a file" },
  },
  {
    title: "Sync from your phone",
    body: "The Android companion reads Health Connect and sends each night through your own ZeitBoard server.",
    link: { href: "#/settings/sync", label: "Set up sync" },
  },
];

interface Blocked {
  sentence: string;
  next: Way;
}

// Refusals that more nights alone would not resolve, each with the one step
// most likely to.
function blockedBy(refusal: NonNullable<OverviewData["refusal"]>): Blocked {
  switch (refusal.code) {
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

function NightsTally({ nights, needed }: { nights: number; needed: number }) {
  return (
    <figure className="nights-tally">
      <ol aria-hidden="true">
        {Array.from({ length: needed }, (_, index) => (
          <li key={index} data-recorded={index < nights || undefined} />
        ))}
      </ol>
      <figcaption>
        <strong>
          {nights} of {needed} nights.
        </strong>{" "}
        A night counts when it is your main sleep and lasts three hours or more.
      </figcaption>
    </figure>
  );
}

function WayEntry({ way, index }: { way: Way; index: number }) {
  return (
    <li className="home-way">
      <span className="home-way-number" aria-hidden="true">
        {index + 1}
      </span>
      <h3>{way.title}</h3>
      <p>{way.body}</p>
      {way.link && (
        <a className="home-more" href={way.link.href}>
          {way.link.label}
        </a>
      )}
    </li>
  );
}

export function HomeSetup({ overview }: { overview: OverviewData }) {
  const { progress, refusal } = overview;
  // More nights resolve a shortage even when the count is not known (a
  // synced estimate does not carry it); anything else says what went wrong.
  const blocked =
    overview.status === "refused" && refusal && refusal.code !== "insufficient_data" && !progress
      ? blockedBy(refusal)
      : undefined;

  return (
    <section className="home-setup" aria-labelledby="home-setup-title">
      <header className="outlook-head">
        <h2 id="home-setup-title">
          <span className="figure-number">Fig. 1</span>{" "}
          {blocked ? "Why there is no forecast" : "Nights so far"}
        </h2>
      </header>
      {progress && <NightsTally nights={progress.nights} needed={progress.needed} />}
      {blocked && refusal && (
        <div className="home-setup-blocked sheet">
          <p>{blocked.sentence}</p>
          {blocked.sentence !== refusal.message && (
            <p className="home-setup-detail">In the estimator&apos;s words: {refusal.message}.</p>
          )}
        </div>
      )}
      <h2 className="section-title">{blocked ? "What would help" : "Ways to add nights"}</h2>
      <ol className="home-ways">
        {(blocked ? [blocked.next] : ways).map((way, index) => (
          <WayEntry key={way.title} way={way} index={index} />
        ))}
      </ol>
    </section>
  );
}
