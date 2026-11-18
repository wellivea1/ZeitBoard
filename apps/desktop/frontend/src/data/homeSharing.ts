import type { OverviewData } from "./overview";
import type { ShareLinksData } from "./sharing";

// What can be said about sharing from what this computer knows: how many links
// are working, and whether the people holding them see any times just now.

export type Freshness = OverviewData["freshness"];

/**
 * The server shows no times once the sleep records are too old to say where
 * the owner is in their rhythm, by the same policy this computer applies. So
 * this computer's own verdict says what a link's reader sees.
 */
export function linksShowNoTimes(freshness: Freshness): boolean {
  return freshness.state === "withheld";
}

/** Home's line about sharing, or nothing when no link is working. */
export function sharingLine(
  links: ShareLinksData,
  freshness: Freshness,
): { working: string; note?: string } | undefined {
  if (links.status !== "ok") return undefined;
  const active = links.links.filter((link) => link.state === "active").length;
  if (active === 0) return undefined;
  const one = active === 1;
  return {
    working: one ? "One share link is working" : `${active} share links are working`,
    ...(linksShowNoTimes(freshness)
      ? {
          note: `${one ? "It shows" : "They show"} no times right now, because your newest sleep record is too old. Record your latest sleep and the times return.`,
        }
      : {}),
  };
}

/**
 * Why a link's reader sees no times, said to the owner beside the preview.
 * The reader's own page says only that there is nothing current to show.
 */
export function previewNote(label: string, freshness: Freshness | undefined): string | undefined {
  if (!freshness || !linksShowNoTimes(freshness)) return undefined;
  const age = freshness.ageLabel ? ` (${freshness.ageLabel.toLowerCase()})` : "";
  return `${label} sees no times right now, because your newest sleep record is too old${age}. The link itself is fine. Record your latest sleep and the times return.`;
}
