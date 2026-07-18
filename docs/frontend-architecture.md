# Desktop frontend architecture

Status: implemented and enforced as of 2026-07-18.

This document defines the maintainability boundary for the Wails React
frontend. The product and interaction rules remain in `ui-ux-design.md`; the
visual refactor decisions remain in `ui-refactor-plan.md`.

## Module boundaries

- `src/App.tsx` owns the application shell and route selection. It imports
  screen modules directly and does not contain screen implementations.
- `src/screens/*Screen.tsx` owns route-level orchestration. Each module exports
  exactly one screen and stays below 600 lines. Large screen-specific sections
  live in a subdirectory such as `src/screens/settings/`.
- `src/components/` owns reusable visual objects and charts. Proposal cards and
  rhythm visuals are shared instead of copied between screens.
- `src/data/` is the only UI layer that knows how to locate Wails methods. Each
  adapter validates and normalizes an unknown DTO before returning typed UI
  data. `wailsBridge.ts` owns method discovery and binding.
- `src/state/` owns state that genuinely spans routes. Small invalidation
  signals, such as sleep-data changes, remain explicit events rather than a
  second application store.
- `src/theme/` owns preset definitions, persistence, and root data attributes.
  Theme presets change semantic tokens; components do not branch on preset
  names.

Screens and components must not import generated Wails packages or read
`globalThis.go` directly. The Go service remains responsible for domain work
and for mapping private domain objects to explicit DTOs.

## Visual composition

The desktop uses one primary surface per screen. Rules, spacing, and type
establish hierarchy inside that surface. Cards are reserved for objects with
their own lifecycle, such as proposals, sleep entries, tasks, and chat action
cards.

Overview has an additional structural contract because it is the primary
fatigue-state screen:

1. one `overview-surface`, with no generic nested panels or metric cards;
2. a dominant current-state band;
3. a cycle strip derived from the same estimate source as Overview;
4. three compact fact rows, a confidence row, conditional attention, and the
   trust boundary;
5. an honest empty/refusal state when no estimate exists.

The cycle strip visualizes the estimated waking span and predicted sleep. The
exact useful-task window remains text because the current Overview DTO exposes
it as a formatted label, not structured numeric bounds. The UI must not parse
that prose to manufacture chart precision.

`styles.css` contains global tokens, resets, shell primitives, and established
shared components. Reworked route-specific composition lives in
`src/styles/*.css`. New component CSS uses spacing, radius, type, and color
tokens rather than raw colors or ad-hoc radii.

## Data and privacy semantics

- Sample fixtures are labeled `Sample data` and are never presented as local or
  synced output. They keep the browser preview usable when no valid Wails DTO
  is available.
- Overview and its cycle strip render together only when both DTOs are
  estimated and come from the same source.
- Suppressing a sleep entry appends an `excluded` correction. The immutable
  observation and correction history remain in export and sync history.
- Permanent deletion is a different binding and requires the exact `DELETE`
  token. It removes the observation, correction history, and local sync payload
  rows, then checkpoints the WAL and vacuums SQLite. IDs already sent to the
  backend remain only in the erasure outbox until tombstones propagate; no
  sleep payload remains there.
- Database erasure does not claim to remove copies held by external backups,
  filesystem snapshots, or storage-device wear leveling.

## Enforced standards

`npm run lint` runs ESLint and `scripts/lint-ui-standards.mjs`. The UI standards
check enforces:

- a 600-line ceiling for screen and component modules;
- one exported screen per `*Screen.tsx` module;
- no recreation of the former `SecondaryScreens.tsx` monolith;
- no direct Wails access from screens or components;
- the structural Overview contract;
- required design tokens and token-only colors/radii in route styles.

ESLint also limits complexity and nesting in UI modules. These checks are
architecture guards, not substitutes for tests or visual review.

## Review disposition

| Finding | Resolution |
|---|---|
| Nine screens shared a 2,512-line module | Split into one route module per screen; Settings sections and shared visuals extracted |
| The first U-B pass restyled cards but did not build the specified Overview | Rebuilt as a status band, cycle strip, fact rows, confidence, attention, and trust within one surface |
| Overview and Rhythm fixture labels contradicted each other | Consolidated the fixture values so state, wake duration, forecast, drift, and useful window agree |
| Seven adapters copied Wails method discovery | Centralized in `src/data/wailsBridge.ts` |
| UI architecture was convention only | Added ESLint limits and the repository-specific UI standards check |
| Theme selection was a text control | Replaced with a visual preset manager with previews and explicit selected state |
| Synthetic previews exposed enabled controls with no action | Removed the dead affordances; preview surfaces now remain visibly read-only |
| Repository checks could hide native Go failures | Hardened `scripts/dev.ps1` to propagate Go, npm, Wails, contract, and Gradle exit codes |

Rhythm-linked appearance switching and the agent-readable appearance action are
not part of this refactor. They remain U-D and require the direct-display-action
ADR described in `ui-refactor-plan.md`.
