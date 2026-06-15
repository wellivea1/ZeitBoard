# ADR 0001: Monorepo and runtime stack

- Status: accepted
- Date: 2026-06-15

## Decision

Use a standalone monorepo with a Go core, Wails v2 desktop shell, React/TypeScript/Vite desktop frontend, native Kotlin/Compose Android companion, SQLite local storage, and versioned JSON Schemas. The core and desktop are separate Go modules linked by a root `go.work` file.

The repository uses Wails v2.12.0 because v2 is the stable line. The Go core has no Wails dependency. SQLite uses the pure-Go `modernc.org/sqlite` driver to keep Windows builds independent of a C compiler.

## Consequences

- Core behavior is testable without a desktop shell.
- Windows-specific collection and tray behavior remain behind interfaces and build tags.
- The Android project has its own Gradle build but shares contracts and fixtures.
- A future Linux desktop and background daemon can reuse the core packages.
