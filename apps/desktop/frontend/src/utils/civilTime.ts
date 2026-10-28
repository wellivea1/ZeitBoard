/** "New York time" for a region's zone; "UTC" for a zone that names no place. */
export function zoneWords(zoneId: string) {
  const slash = zoneId.lastIndexOf("/");
  return slash < 0 ? zoneId : `${zoneId.slice(slash + 1).replace(/_/g, " ")} time`;
}

let checkedZone: { reported: string; zone: string } | undefined;

/**
 * This computer's zone. Where ICU cannot name the host's zone it reports
 * "Etc/Unknown", which no formatter accepts, and every screen that formats a
 * date would fail; then it is UTC, as the desktop's own zone is in that case.
 */
export function localZone(): string {
  const reported = Intl.DateTimeFormat().resolvedOptions().timeZone;
  if (checkedZone?.reported !== reported) {
    let zone = "UTC";
    try {
      new Intl.DateTimeFormat("en-US", { timeZone: reported });
      zone = reported || zone;
    } catch {
      // A zone the formatter refuses is no zone to write times in.
    }
    checkedZone = { reported, zone };
  }
  return checkedZone.zone;
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

/**
 * A stored time read in its zone: the wall clock it shows there and, when the
 * value names its offset, the exact instant. "2026-10-25T06:00:00-04:00" in
 * New York is 6:00 on Oct 25; a bare wall time names no instant of its own.
 */
export function zonedWall(value: string, zone: string): { wall: string; at?: string } {
  if (/(?:[zZ]|[+-]\d\d:\d\d)$/.test(value)) {
    const instant = Date.parse(value);
    if (Number.isFinite(instant)) {
      return { wall: civilMinute(instant, zone), at: new Date(instant).toISOString() };
    }
  }
  return { wall: value.slice(0, 16) };
}

/**
 * A wall time with the UTC offset of the instant chosen for it, in RFC 3339:
 * "2026-11-01T01:30:00-05:00" is the second 1:30 that night in New York.
 */
export function offsetTimestamp(wall: string, instant: string): string {
  const minutes = Math.round((Date.parse(`${wall}:00Z`) - Date.parse(instant)) / 60_000);
  const whole = Math.abs(minutes);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${wall}:00${minutes < 0 ? "-" : "+"}${pad(Math.floor(whole / 60))}:${pad(whole % 60)}`;
}
