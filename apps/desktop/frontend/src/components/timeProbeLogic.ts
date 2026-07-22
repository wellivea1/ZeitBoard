import { useCallback, useEffect, useRef, type PointerEventHandler, type RefObject } from "react";

const MINUTES_PER_DAY = 24 * 60;
let activeProbe: HTMLDivElement | undefined;

export interface TimeProbeReading {
  position: number;
  label: string;
  zoneId?: string;
}

type ProbeResolver = (fraction: number) => TimeProbeReading | undefined;

export interface TimeProbeController {
  onPointerMove: PointerEventHandler<HTMLDivElement>;
  onPointerLeave: PointerEventHandler<HTMLDivElement>;
  probeRef: RefObject<HTMLDivElement | null>;
  labelRef: RefObject<HTMLSpanElement | null>;
}

function clamp(value: number) {
  return Math.min(1, Math.max(0, value));
}

function parseCivilDate(civilDate: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(civilDate);
  if (!match) return undefined;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return undefined;
  }
  return date;
}

const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

export function formatCivilDate(civilDate: string, dayOffset = 0, includeWeekday = true) {
  const date = parseCivilDate(civilDate);
  if (!date) return civilDate;
  date.setUTCDate(date.getUTCDate() + dayOffset);
  const prefix = includeWeekday ? `${WEEKDAYS[date.getUTCDay()]} ` : "";
  return `${prefix}${MONTHS[date.getUTCMonth()]} ${date.getUTCDate()}`;
}

export function formatClock24(hour: number) {
  let totalMinutes = Math.round(hour * 60);
  totalMinutes = ((totalMinutes % MINUTES_PER_DAY) + MINUTES_PER_DAY) % MINUTES_PER_DAY;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}`;
}

export function civilProbeLabel(
  civilDate: string,
  minuteOffset: number,
  options: { predicted?: boolean; approximate?: boolean; includeWeekday?: boolean } = {},
) {
  const roundedMinutes = Math.max(0, Math.round(minuteOffset));
  const dayOffset = Math.floor(roundedMinutes / MINUTES_PER_DAY);
  const time = formatClock24((roundedMinutes % MINUTES_PER_DAY) / 60);
  const approximate = options.approximate ? "~" : "";
  const qualifier = options.predicted ? " · predicted" : "";
  return `${formatCivilDate(civilDate, dayOffset, options.includeWeekday !== false)} · ${approximate}${time}${qualifier}`;
}

export function useTimeProbe(resolve: ProbeResolver): TimeProbeController {
  const probeRef = useRef<HTMLDivElement>(null);
  const labelRef = useRef<HTMLSpanElement>(null);
  const activatedProbeRef = useRef<HTMLDivElement | null>(null);

  useEffect(
    () => () => {
      const probe = activatedProbeRef.current;
      if (activeProbe === probe) activeProbe = undefined;
      activatedProbeRef.current = null;
    },
    [],
  );

  const onPointerMove = useCallback<PointerEventHandler<HTMLDivElement>>(
    (event) => {
      const bounds = event.currentTarget.getBoundingClientRect();
      if (bounds.width <= 0) return;
      const fraction = clamp((event.clientX - bounds.left) / bounds.width);
      const reading = resolve(fraction);
      const probe = probeRef.current;
      const label = labelRef.current;
      if (!reading || !probe || !label) return;

      if (activeProbe && activeProbe !== probe) activeProbe.hidden = true;
      activeProbe = probe;
      activatedProbeRef.current = probe;
      const position = clamp(reading.position);
      probe.hidden = false;
      probe.style.transform = `translateX(${position * bounds.width}px)`;
      probe.dataset.edge = position < 0.14 ? "start" : position > 0.86 ? "end" : "middle";
      if (reading.zoneId) probe.dataset.zoneId = reading.zoneId;
      else delete probe.dataset.zoneId;
      label.textContent = reading.label;
    },
    [resolve],
  );

  const onPointerLeave = useCallback<PointerEventHandler<HTMLDivElement>>(() => {
    const probe = probeRef.current;
    if (probe) probe.hidden = true;
    if (activeProbe === probe) activeProbe = undefined;
    activatedProbeRef.current = null;
  }, []);

  return { onPointerMove, onPointerLeave, probeRef, labelRef };
}
