export function localZone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

export function civilMinute(instant: string | number, zone = localZone()): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(instant));
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  return `${get("year").padStart(4, "0")}-${get("month")}-${get("day")}T${get("hour")}:${get("minute")}`;
}

// Enumerate offsets around the date, then round-trip each candidate through the
// requested zone. Gaps have no candidate; repeated clock times have two.
export function civilCandidates(
  wall: string,
  zone = localZone(),
): { instant: string; label: string }[] {
  if (!/^\d{4}-\d\d-\d\dT\d\d:\d\d$/.test(wall)) return [];
  const naive = Date.parse(`${wall}:00Z`);
  if (!Number.isFinite(naive)) return [];
  const offsets = new Set<number>();
  for (let hour = -48; hour <= 48; hour += 3) {
    const sample = naive + hour * 3_600_000;
    offsets.add(Date.parse(`${civilMinute(sample, zone)}:00Z`) - sample);
  }
  return [...offsets]
    .map((offset) => ({ instant: new Date(naive - offset).toISOString(), offset }))
    .filter(({ instant }) => civilMinute(instant, zone) === wall)
    .sort((a, b) => a.instant.localeCompare(b.instant))
    .map(({ instant, offset }) => {
      const minutes = Math.abs(offset / 60_000);
      return {
        instant,
        label: `UTC${offset < 0 ? "−" : "+"}${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`,
      };
    });
}

export function selectedCivilInstant(
  wall: string,
  selected: string | undefined,
  zone = localZone(),
): string {
  const candidates = civilCandidates(wall, zone);
  return (
    candidates.find((c) => c.instant === selected)?.instant ??
    (candidates.length === 1 ? candidates[0]!.instant : "")
  );
}
