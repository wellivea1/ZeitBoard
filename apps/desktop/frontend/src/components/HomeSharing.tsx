import { sharingLine, type Freshness } from "../data/homeSharing";
import { loadShareLinks, shareLinksUnavailable } from "../data/sharing";
import { useLoaded } from "../state/useLoaded";

// What Home says about sharing: how many links are working, and whether the
// people holding them see any times just now. It used to be a fixed line of
// developer words, the same whether no link or several were working, so Home
// could not answer "is my portal working, and what does it show?".

export function HomeSharing({ freshness }: { freshness: Freshness }) {
  const { data } = useLoaded(loadShareLinks, { fallback: () => shareLinksUnavailable });
  const line = data && sharingLine(data, freshness);
  if (!line) return null;
  return (
    <p className="home-sharing">
      <a href="#/sharing">{line.working}</a>
      {line.note && <span>. {line.note}</span>}
    </p>
  );
}
