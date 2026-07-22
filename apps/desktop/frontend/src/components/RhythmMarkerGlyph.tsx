import type { RhythmMarkerKind } from "../data/rhythmMarkers";

export function RhythmMarkerGlyph({
  kind,
  label,
  decorative = false,
  className = "",
}: {
  kind: RhythmMarkerKind;
  label?: string;
  decorative?: boolean;
  className?: string;
}) {
  return (
    <span
      className={`rhythm-marker-glyph is-${kind}${className ? ` ${className}` : ""}`}
      role={decorative ? undefined : "img"}
      aria-hidden={decorative || undefined}
      aria-label={decorative ? undefined : label}
    >
      <i />
    </span>
  );
}
