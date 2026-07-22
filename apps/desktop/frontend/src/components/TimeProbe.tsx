import type { RefObject } from "react";

interface TimeProbeProps {
  probeRef: RefObject<HTMLDivElement | null>;
  labelRef: RefObject<HTMLSpanElement | null>;
}

export function TimeProbe({ probeRef, labelRef }: TimeProbeProps) {
  return (
    <div className="time-probe" ref={probeRef} hidden aria-hidden="true">
      <span className="time-probe-label" ref={labelRef} />
    </div>
  );
}
