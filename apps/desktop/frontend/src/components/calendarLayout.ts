import type { CalendarEventSegment } from "../data/calendar";

export interface CalendarLaneLayout {
  lanes: Map<string, number>;
  count: number;
}

export function assignEventLanes(events: CalendarEventSegment[]): CalendarLaneLayout {
  const lanes = new Map<string, number>();
  const laneEnds: number[] = [];
  const ordered = [...events].sort(
    (left, right) =>
      Number(right.allDay) - Number(left.allDay) ||
      left.startMinute - right.startMinute ||
      left.endMinute - right.endMinute ||
      left.segmentId.localeCompare(right.segmentId),
  );
  for (const event of ordered) {
    const eventEnd = Math.max(event.endMinute, event.startMinute + 15);
    let lane = laneEnds.findIndex((end) => end <= event.startMinute);
    if (lane < 0) {
      lane = laneEnds.length;
      laneEnds.push(eventEnd);
    } else {
      laneEnds[lane] = eventEnd;
    }
    lanes.set(event.segmentId, lane);
  }
  return { lanes, count: Math.max(1, laneEnds.length) };
}
