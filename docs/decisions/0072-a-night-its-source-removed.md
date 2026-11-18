# ADR 0072: A night its source removed

- Status: accepted
- Date: 2026-11-17
- Builds on [ADR-0013](0013-append-only-sleep-model.md),
  [ADR-0032](0032-android-sleep-synchronisation.md) and
  [ADR-0054](0054-sources-say-what-they-covered.md).
- Chosen by the owner on 2026-11-17, from three options.

## Context

The phone reads every sleep session Health Connect holds over the last 30 days, and syncs each
new one as an observation and each revision as a provider correction (ADR-0032). Nothing
happened when a session was deleted in Health Connect, by the owner or by the watch app that
wrote it: ZeitBoard's copy stayed in the estimate, the forecast and the share page.

Erasing the copy everywhere would follow the source, but it cannot be undone, and a watch app
that tidies or rewrites its sessions would remove nights without telling anyone. Asking about
each night would leave an unanswered one counting.

## Decision

1. **A removed night is kept, and left out.** The provider gains two corrections:
   `source_deleted`, which sets `excluded` to true, and `source_restored`, which sets it to
   false. Each changes nothing else, and only a provider (`acquisition_method: health_connect`)
   may give one. The imported record is never rewritten, as ADR-0013 requires.
2. **They are no revision of the times.** A presence correction does not move a record's source
   revision, so a correction the owner made stays reviewed when the record goes or comes back.
3. **The owner's correction stands on a removed night.** Folding a night, the provider's word is
   applied first; a correction of the owner's then counts the night again unless it left the
   night out too. The night stays marked "removed at the source" either way.
4. **The phone judges absence narrowly.** A session counts as gone only when a complete read of
   this phone's own Health Connect no longer returns it, and only if:
   - this phone read it before and it has synced, so a night another phone read is never
     judged here;
   - it began a day or more inside the window read, since near the window's start a session can
     fall out of a read without being deleted; and
   - the phone's last word on it was not already that it is gone.
   One the phone said was gone that a read returns again is said to be back. Each word is dated
   by the read that found it, and its id follows from the record, the word and that moment.
5. **Every surface says so.** The desktop's sleep log marks the night "Removed at the source" and
   says how to count it again, or that it still counts because the owner's correction stands.
   The phone's list of nights marks it, and its review explains it. The server's review context
   carries `removed_at_source`.

## Consequences

- A night the watch app deleted stops bending the estimate on every device once the phone has
  read Health Connect and synced, and comes back if the session does.
- Erasure is unchanged: the owner can still erase a night, which removes it everywhere.
- `testdata/v1/sync-android-source-presence.json` is a removal and a restoration of one night.
  The phone writes it byte for byte (`SyncContractTest`) and the server validates and folds it
  (`TestSharedAndroidPresenceFixtureValidatesAndFolds`).

## Not decided here

- Sources other than Health Connect. None of the others can tell ZeitBoard that a record went.
