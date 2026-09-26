# ADR 0047: Accepted times sync as placement records

- Status: accepted
- Date: 2026-09-26
- Owner direction: of three options (sync accepted times as their own records,
  store them on the task, or leave the phone's dial without plans), the owner
  chose to sync them as their own records.

## Context

The Android dial shows the next 24 hours of forecast sleep and the last week of
recorded nights. It could not show plans: an accepted suggestion lives on the
desktop as an app-owned calendar block and a decision record, and neither
syncs. Tasks sync (ADR-0020), but a task carries no time.

Putting the time on the task would reuse task sync, but accepting would then be
an edit of the task, bumping its revision. Suggestions are checked against the
exact task revision they were made for, and the accepted block records that
revision, so the acceptance would invalidate itself.

## Decision

1. An accepted time travels as an immutable sync record of kind `placement`.
   Its id is the app-owned block's event id. Its payload is `placement_id`,
   `task_id`, `start_at`, `end_at`, `zone_id` and `created_at`, defined in
   `sync-batch.schema.json#/$defs/placementPayload`. It carries no title: the
   task record already does.
2. The desktop pushes a placement once its block exists and marks it pushed on
   acknowledgment. A trigger on deleting an app-owned block queues the
   placement's erasure the same moment, so undo, deleting the task and erasing
   all data each remove it from the server without separate code paths.
3. The server validates the payload (its own id, a named task, an interval of at
   most a day, a real zone) and stores and relays it like any record. Erasure
   uses the existing tombstones, which now may name `placement` as their kind.
4. The desktop authored every placement. Pulling one back confirms its upload;
   a placement tombstone settles bookkeeping and never deletes a local block.
   Restore reconciliation re-sends placements the server no longer has.
5. The companion keeps placements with its downloaded tasks. It draws the
   accepted times of open tasks, titled from each task's latest revision, as a
   thin ink ring inside the forecast ring for the next 24 hours. A placement
   whose task is erased or done is not drawn, and an erased task erases its
   placements from the companion's copy.

## Consequences

- Accepted times reach the owner's own server, which already holds the tasks
  they belong to. The phone gains no new permission and sends nothing new.
- The dial's legend shows "Plans" only when there are plans to draw, and its
  spoken description lists them with their times.
- Imported calendar events still stay on the desktop. Showing them on the phone
  would send new private data to the server and needs its own decision.
