# Architecture and performance review disposition

Date: 2026-07-27

This is the implementation ledger for the architecture, complexity, and
efficiency review performed against `24ffc17`. It distinguishes the specific
blocker fixed in this pass from broader follow-up work. `Complete` means the
reported behavior and its regression coverage are addressed. `Partial` means
the highest-risk path was fixed but the broader architecture concern remains.
`Deferred` means a large structural change was intentionally not mixed into a
behavioral hardening pass.

## Priority findings

| # | Disposition | Implementation and remaining work |
|---:|---|---|
| 1 | Complete | Desktop sleep and task outboxes page from SQLite and push batches by both the 100-record and 1 MiB wire limits. Each successful batch is marked before the next begins. Boundary tests cover 101 records and encoded-size limits. |
| 2 | Partial | Server projection reads capture a sequence high-water, query only observation or correction rows through `(kind, seq)`, and discard decrypted pages as they are visited. The fold still retains all relevant sleep observations and corrections. A rebuildable incremental effective-sleep read model remains appropriate when retained sleep history becomes large enough to justify its state and invalidation cost. |
| 3 | Complete | Backtesting now selects and sorts once, fits only the bounded recent model window, and shares estimator fit/forecast code. Regression tests compare the optimized path with the previous prefix implementation; benchmarks cover 100 through 20,000 episodes. Server-side caching remains an optional later optimization, not a correctness blocker. |
| 4 | Complete | `core/sleepv1` owns v1 sleep decoding, validation, domain conversion, correction folding, target-zone handling, and overlap resolution for both local storage and server replay. Domain classification is explicit, and only principal sleep enters estimation. |
| 5 | Complete | Task edits/status/deletes use revision compare-and-swap transactions with a typed conflict. Pulled observations, corrections, tasks, sync markers, and the cursor commit as one page transaction. |
| 6 | Complete | Imported source IDs have a supporting partial index. Import parsing is single-pass at the envelope boundary and conflict lookup avoids decoding unrelated payloads. Migration and query-plan tests cover the index. |
| 7 | Complete | Scheduling merges busy intervals once and subtracts them with a linear sweep. Interval and benchmark coverage includes dense, disjoint, and large event sets. |
| 8 | Complete | Proposal history is bounded and opaque-cursor paged, with active unexpired proposals first, a stable high-water/snapshot cursor, joined unused nonces, and a matching partial index. Store, API, desktop adapter, provider, and load-older tests cover the flow. |
| 9 | Complete | A transactionally consistent sleep snapshot now derives raw, corrected, and effective views from one observation query and one correction query. Mutation follow-up uses a point snapshot instead of rebuilding the full list. |
| 10 | Complete | Sleep and task pending counts/pages use SQL anti-joins over stored payloads rather than reconstructing every domain record and hash in memory. |
| 11 | Complete | One ordered pull page and its cursor apply atomically. Tombstones are applied last, hard erasures are compacted once after commit, and a durable pending marker retries compaction after an interruption without replaying the page. |
| 12 | Partial | Medication context uses one sorted sleep interval index with binary search, and local-agent/assistant summaries are bounded before presentation. The private desktop DTO still materializes all medication events; repository-backed history paging is the remaining long-history improvement. |
| 13 | Complete | Calendar source counts and civil-day buckets are single-pass, and report sleep intervals are placed directly into their first/last touched rows. Loaded locations are reused. |
| 14 | Partial | Desktop-local agent tools now call explicitly local overview/rhythm paths and backend clients share lifecycle-managed transports. Normal UI overview/rhythm remain backend-first by design and can still wait for the bounded backend timeout before local fallback; asynchronous server projection refresh remains future work. |
| 15 | Partial | New native import/export, proposal, local-agent, sync, and projection paths use the application or request context. Older Wails facade methods still use `context.Background()` and should move behind context-aware feature services as those services are extracted. |
| 16 | Partial | Sleep, medication, report, and proposal surfaces bound mounted rows or provide client paging; closed report detail is lazy. Some Go DTOs still load complete private histories before the renderer pages them. Repository-backed pages remain necessary for truly long-lived stores. |
| 17 | Complete | Installed desktop import/export uses native file dialogs. Go performs bounded reads, one-use digest-bound preview/commit, and atomic flushed export replacement; React receives only bounded metadata and a short preview. Browser fixture fallback remains bounded. |
| 18 | Complete | Medication and rhythm-marker mutation owners accept returned DTOs without immediately refetching their own full projection; invalidation is coalesced for other consumers. |
| 19 | Partial | One backend-proposal provider owns fetch, pagination, token decisions, stale-generation rejection, and assistant/Approvals publication. The unrelated local approvals provider remains broad and can be split when its next feature requires it. |
| 20 | Complete | Representative Overview, Rhythm, Settings, appearance, sleep, and proposal refreshes now coalesce in-flight work and reject superseded completions. Polling schedules the next read after settlement and pauses where visibility is nonessential. |
| 21 | Complete | Hash-route screens, the assistant rail, and clinician report implementation are lazy chunks with synchronous shell/loading states. Production bundle output verifies separate chunks. |
| 22 | Deferred | Splitting the desktop `App`, sync, and medication facades into services remains directionally correct. A bulk file move would add review risk without changing behavior, so extraction should occur at the next feature boundary with narrow interfaces and context ownership. |
| 23 | Partial | Sleep wire/fold ownership is shared, fixture/schema mappings use one manifest, and live projection producers are schema-validated. Assistant action definitions remain copied across prompts, MCP tools, validation, dispatch, and presentation; one versioned action registry is still required. |
| 24 | Partial | Boundary validation remains strict, paged responses reduce its largest copies, and common refresh/parsing helpers were extracted where used. The large calendar/medication/report validators are still handwritten and outside generated schema ownership. |
| 25 | Partial | New local task/index changes use ordered transactional migrations and are recorded only after success. Legacy local baseline DDL and server DDL still run idempotently at startup and should be converted incrementally, with upgrade fixtures, rather than rewritten in one risky migration. |
| 26 | Partial | Android persistence has repository/store boundaries, fixture generation has one manifest, and installer publication paths share policy helpers. The desktop/server stores and large UI/script modules still merit domain-boundary extraction when changed. |
| 27 | Complete | Android corrections, Health Connect snapshots, and medication events use a durable append-only SQLite repository with explicit loading/ready/failed state and last-good UI data. Instrumented process-recreation tests cover persistence. |
| 28 | Complete | Health Connect consumes page tokens under record/page caps, deduplicates immutable source revisions, retains the last good snapshot on errors, and preserves source offsets without inventing an IANA zone. DST ambiguity/gap handling is explicit. |
| 29 | Complete | Android builds indexed latest-correction projections, queries active corrections independently of bounded history, and appends medication events without full-list repository rewrites. Duplicate save guards and retry semantics preserve failed input. |
| 30 | Complete | Installer ACL policy uses one safety walk and a versioned post-reset marker. Recursive reset is skipped only when both the root ACL and current marker prove the policy; tests reject a root-only false positive. |
| 31 | Complete | Update compares verified installed component commits with `HEAD`, exits before build/backup/publication when unchanged, and retains an explicit force-rebuild path. |
| 32 | Complete | One fixture manifest owns generated path, schema, and version. Validation checks the exact file set, including the clinical-chart request and unexpected stale files. |
| 33 | Complete | Root `check:web` is the canonical format/lint/type/test/build command used by local scripts and one CI job after a single workspace install. |

## Additional defects fixed

- Task erasure now records a logical-task tombstone, expands deletion to every
  retained revision, and rejects future offline revisions. This closes a
  resurrection path that record-level tombstones alone could not prevent.
- Tombstones include their original non-sensitive record kind when known, so
  clients do not route erasure by identifier shape. Legacy kindless records use
  stored evidence and fail on ambiguity.
- Native import invalidates stale tokens even when a new dialog is canceled,
  consumes tokens on failed commits, and rejects file changes after preview.
- Export replacement is staged in the destination directory, flushed, and
  atomically published so interruption cannot replace a valid file with a
  partial payload.
- Proposal paging now prioritizes actionable work over newer decision history;
  a high-water row excludes inserts that occur between continuation requests.
- Android preserves logical Health Connect source identity separately from
  immutable revisions, surfaces orphaned corrections for review, rejects DST
  gaps/ambiguous times without an offset, and does not report failed medication
  writes as successful.

## Verification gates

The pass is accepted only when these remain green:

- Go tests and `go vet` for core, server, desktop, and the isolated tools module.
- Contract validation plus exact fixture regeneration.
- Desktop frontend and trusted-view formatting, lint, UI architecture checks,
  type checks, tests, and production builds.
- Android unit tests, lint, debug assembly, and connected tests on
  `Pixel_10_Pro_XL_API_36_1`.
- Installer transaction, no-op, ACL, rollback, and static-policy tests.
- `git diff --check` and `git fsck --full` after the interrupted work session.

## Follow-up order

1. Add repository-backed medication and sleep history pages before increasing
   supported renderer history sizes.
2. Introduce one versioned assistant action registry and derive MCP definitions,
   validation, and presentation metadata from it.
3. Continue ordered migrations with upgrade fixtures; do not rewrite existing
   databases merely to make migration code aesthetically uniform.
4. Extract context-aware desktop feature services as adjacent behavior changes
   require those boundaries.
5. Add an incremental effective-sleep server read model only after production
   profiles show relevant-history replay, rather than mixed-log replay, is the
   remaining material request cost.
