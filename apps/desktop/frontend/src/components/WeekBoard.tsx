import { useEffect, useRef, type CSSProperties } from "react";
import type { CalendarEventSegment } from "../data/calendar";
import type { ProposalDecision } from "../state/approvals";
import { Icon } from "./Icon";
import {
  hourLabel,
  MIN_BLOCK_MINUTES,
  type WeekBand,
  type WeekBlock,
  type WeekColumn,
} from "./weekLayout";

// The calendar-native view of Plan (ui-refactor-plan.md §14): days as columns
// on one vertical clock. Sleep is painted into the days — recorded behind the
// past, the forecast from now on, the uncertain edges hatched — and plans sit
// on top: events solid, accepted times framed, suggestions dashed with their
// decision beside the title.

type BoardStyle = CSSProperties & { "--hour"?: string; "--days"?: number };

const gutterHours = [3, 6, 9, 12, 15, 18, 21];

function nowLabel(minute: number) {
  const hour = Math.floor(minute / 60);
  return `${((hour + 11) % 12) + 1}:${String(minute % 60).padStart(2, "0")}`;
}

interface BlockHandlers {
  hour: number;
  deciding: boolean;
  onSelect: (event: CalendarEventSegment) => void;
  onInspect: (proposalId: string) => void;
  onDecide: (proposalId: string, decision: ProposalDecision) => void;
}

function Band({ band, px }: { band: WeekBand; px: (minutes: number) => string }) {
  const minutes = band.endMinute - band.startMinute;
  return (
    <div
      className="week-band"
      data-kind={band.kind}
      style={{ top: px(band.startMinute), height: px(minutes) }}
    >
      {minutes >= 50 && <span>{band.label}</span>}
    </div>
  );
}

function Block({
  block,
  hour,
  deciding,
  onSelect,
  onInspect,
  onDecide,
}: BlockHandlers & { block: WeekBlock }) {
  const px = (minutes: number) => `${(minutes / 60) * hour}px`;
  const minutes = block.endMinute - block.startMinute;
  const style: CSSProperties = {
    top: px(block.startMinute),
    height: `calc(${px(Math.max(minutes, MIN_BLOCK_MINUTES))} - 2px)`,
    // Inset 2px each side, so neighbouring blocks never touch.
    left: `calc(${block.lane} * 100% / ${block.lanes} + 3px)`,
    width: `calc(100% / ${block.lanes} - 5px)`,
  };
  // Short blocks set the time beside the title rather than under it.
  const short = minutes < 50 || undefined;

  if (block.kind === "suggested" && block.proposalId) {
    const id = block.proposalId;
    return (
      <div className="week-block" data-kind="suggested" data-short={short} style={style}>
        {/* The words open the suggestion under the board, where there is
            room for the reasons; a narrow block keeps only the words. */}
        <button
          className="week-block-text"
          type="button"
          aria-label={`${block.title}, suggested for ${block.timeLabel}`}
          onClick={() => onInspect(id)}
        >
          <strong>{block.title}</strong>
          <small>{block.timeLabel}</small>
        </button>
        <span className="week-block-actions">
          <button
            className="week-decide"
            data-decision="accept"
            type="button"
            disabled={deciding}
            aria-label={`Accept the suggested time for ${block.title}`}
            onClick={() => onDecide(id, "approved")}
          >
            <Icon name="check" />
          </button>
          <button
            className="week-decide"
            type="button"
            disabled={deciding}
            aria-label={`Decline the suggested time for ${block.title}`}
            onClick={() => onDecide(id, "rejected")}
          >
            <Icon name="close" />
          </button>
        </span>
      </div>
    );
  }

  return (
    <button
      className="week-block"
      data-kind={block.kind}
      data-short={short}
      data-continues={block.continuesAfter || undefined}
      type="button"
      style={style}
      aria-label={`${block.title}, ${block.timeLabel}${block.kind === "accepted" ? ", a time you accepted" : ""}`}
      onClick={() => block.event && onSelect(block.event)}
    >
      <span className="week-block-text">
        <strong>{block.title}</strong>
        <small>{block.timeLabel}</small>
      </span>
    </button>
  );
}

function Day({ column, ...handlers }: BlockHandlers & { column: WeekColumn }) {
  const px = (minutes: number) => `${(minutes / 60) * handlers.hour}px`;
  return (
    <div
      className="week-day"
      data-today={column.isToday || undefined}
      data-past={column.isPast || undefined}
    >
      {column.bands.map((band) => (
        <Band band={band} px={px} key={band.key} />
      ))}
      {column.forecastEndsAt !== undefined && (
        <div className="week-beyond" style={{ top: px(column.forecastEndsAt) }}>
          <span>Beyond the forecast</span>
        </div>
      )}
      {column.doses.map((dose) => (
        <span className="week-dose" style={{ top: px(dose.minute) }} key={dose.key}>
          {dose.label}
        </span>
      ))}
      {column.blocks.map((block) => (
        <Block block={block} {...handlers} key={block.key} />
      ))}
      {column.nowMinute !== undefined && (
        <span className="week-now" style={{ top: px(column.nowMinute) }} aria-hidden="true" />
      )}
    </div>
  );
}

export function WeekBoard({
  columns,
  hour,
  deciding,
  onSelect,
  onInspect,
  onDecide,
}: BlockHandlers & { columns: WeekColumn[] }) {
  const scroller = useRef<HTMLDivElement>(null);
  const anchor = columns[0]?.civilDate;
  const hasToday = columns.some((column) => column.isToday);
  const today = columns.find((column) => column.isToday);

  // Opens two hours before now, or at seven in the morning on days without
  // it, rather than at midnight with the day below the fold.
  useEffect(() => {
    const element = scroller.current;
    if (!element) return;
    const now = new Date();
    const minute = hasToday ? now.getHours() * 60 + now.getMinutes() - 120 : 7 * 60;
    element.scrollTop = Math.max(0, (minute / 60) * hour);
  }, [anchor, hasToday, hour]);

  const style: BoardStyle = { "--hour": `${hour}px`, "--days": columns.length };
  const allDay = columns.some((column) => column.allDay.length > 0);

  return (
    <section className="week-board" style={style} aria-label="Days with sleep and plans">
      <div className="week-scroll" ref={scroller}>
        {/* Inside the scroller and sticky, so the heads share the columns'
            width whether or not a scrollbar takes some of it. */}
        <div className="week-sticky">
          <div className="week-head">
            <span className="week-gutter-head" aria-hidden="true" />
            {columns.map((column) => (
              <div
                className="week-day-head"
                data-today={column.isToday || undefined}
                key={column.civilDate}
              >
                <span>{column.weekday}</span> <strong>{column.dayNumber}</strong>
                {column.isToday && <em>Today</em>}
              </div>
            ))}
          </div>
          {allDay && (
            <div className="week-allday">
              <span className="week-gutter-head">All day</span>
              {columns.map((column) => (
                <div key={column.civilDate}>
                  {column.allDay.map((event) => (
                    <button
                      className="week-allday-event"
                      type="button"
                      onClick={() => onSelect(event)}
                      key={event.segmentId}
                    >
                      {event.title}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          )}
        </div>
        <div className="week-grid">
          <div className="week-gutter" aria-hidden="true">
            {gutterHours.map((value) => (
              <span style={{ top: `${value * hour}px` }} key={value}>
                {hourLabel(value)}
              </span>
            ))}
            {today?.nowMinute !== undefined && (
              <span
                className="week-gutter-now"
                style={{ top: `${(today.nowMinute / 60) * hour}px` }}
              >
                {nowLabel(today.nowMinute)}
              </span>
            )}
          </div>
          {columns.map((column) => (
            <Day
              column={column}
              hour={hour}
              deciding={deciding}
              onSelect={onSelect}
              onInspect={onInspect}
              onDecide={onDecide}
              key={column.civilDate}
            />
          ))}
        </div>
      </div>
    </section>
  );
}
