# Phase-one implementation plan

> **Historical document.** This is the delivered phase-one plan, kept as the
> record of the original scaffold's scope and acceptance checks. The living
> plan is [`roadmap.md`](roadmap.md); build status is tracked there and in
> [`specification-alignment.md`](specification-alignment.md).

## Scope

Build an executable architecture scaffold, not a complete health or scheduling product. All runtime data is local or synthetic. No public sharing endpoint is created.

## Change groups

1. Establish the monorepo, versioned contracts, domain invariants, and architecture decisions.
2. Implement the Go core: append-only observations/corrections, robust drift estimation with typed refusal, deterministic task proposals, medication-relative timing, SQLite persistence, collectors, and sharing projections.
3. Add the Wails v2 application service and restrained React/TypeScript screens backed by synthetic fixture data.
4. Add the static trusted-view prototype using a pre-projected, permission-safe fixture.
5. Add the native Android Compose shell, visible fixture mode, Health Connect availability/permission boundary, manual correction, medication entry, and settings.
6. Add setup scripts, synthetic data generation, CI, linting, tests, architecture/privacy/threat-model documentation, and runbooks.
7. Run all feasible tests and builds. Record environmental limitations without hiding failures.

## Major decisions

- Use separate `core` and `apps/desktop` Go modules linked by `go.work`; only the desktop module imports Wails.
- Use Wails v2.12.0. Wails v3 remains pre-stable and is excluded.
- Use `modernc.org/sqlite` to avoid a CGO toolchain requirement on Windows.
- Represent time as UTC instants plus IANA time-zone IDs. Intervals are half-open and never grouped by midnight.
- Keep acquisition method and epistemic status as separate provenance dimensions.
- Preserve imported observations unchanged and apply append-only manual corrections to an effective read model.
- Use recent principal sleep episodes, cycle indexing, and Theil-Sen robust regression for the first estimator. Refuse estimates when data is insufficient or ambiguous.
- Treat confidence as an ordinal assessment with reasons; represent temporal uncertainty explicitly in forecast windows.
- Return scheduling proposals with explanations. Never mutate fixed events.
- Use explicit, default-deny share projections that cannot contain medication, diagnosis, location, raw activity, or private calendar text.
- Use Android API 26 for fixture-only compatibility and require API 28+ for real Health Connect access.
- Keep Android estimation out of process for phase one; fake repositories return fixture estimates conforming to shared contracts.

## Initial pinned baseline

- Go 1.26.x
- Wails 2.12.0
- React 19.2.7, TypeScript 6.0.3, Vite 8.0.16
- SQLite driver `modernc.org/sqlite` 1.52.0
- Android Gradle Plugin 9.2.1, Gradle 9.4.1, JDK 17
- Compose BOM 2026.05.01, Activity Compose 1.13.0, Navigation Compose 2.9.8
- Lifecycle 2.10.0, Health Connect 1.1.0

## Acceptance checks

- Go unit and integration tests cover the required time, estimation, correction, scheduling, medication, and sharing cases.
- Both web frontends build with pinned local dependencies.
- Wails produces a Windows desktop binary and fixture data reaches the overview.
- Android JVM tests and debug APK build pass; emulator launch is attempted when an emulator is available.
- The trusted prototype contains only synthetic, allowlisted fields and performs no network requests.
