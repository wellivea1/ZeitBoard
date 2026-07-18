# Development

## Prerequisites

- Go 1.26.x for the root module, the Wails service, and the contract/fixture tooling.
- Node.js 24.16.0 and its bundled npm for the checked-in frontend lockfile.
- JDK 17 through 21 for Android; CI uses 17 and Android Studio's bundled JDK 21 is supported.
- Wails system prerequisites when running the desktop shell.

## Setup

From the repository root:

```powershell
.\scripts\setup.ps1
```

```sh
bash scripts/setup.sh
```

Setup verifies tool versions and downloads only dependencies described by
existing module files, lockfiles, and Gradle wrappers. It does not generate or
rewrite dependency manifests.

## Common commands

PowerShell uses `-Action` and `-Component`; POSIX shells use positional action
and component arguments.

```powershell
.\scripts\dev.ps1 -Action check -Component all
.\scripts\dev.ps1 -Action check -Component server
.\scripts\dev.ps1 -Action dev -Component desktop
.\scripts\dev.ps1 -Action build -Component android
.\scripts\dev.ps1 -Action fixtures
```

```sh
bash scripts/dev.sh check all
bash scripts/dev.sh check server
bash scripts/dev.sh dev desktop
bash scripts/dev.sh build android
bash scripts/dev.sh fixtures
```

Components: `contracts`, `core`, `server` (the self-hosted backend module
under `apps/server`, producing `zeitboardd` and `zeitboard-mcp` — operator
docs in `self-hosting.md`), `desktop`, `trusted-web`, and `android`.
Components are discovered conservatively: missing application subtrees are
reported and skipped, and a present component that fails its command causes
the script to fail.

## Fixture and contract checks

The contract and fixture tooling is a Go module under `tools/`. From `tools/`,
`go run ./cmd/genfixtures -check` detects fixture drift (omit `-check` to
regenerate), and `go test ./...` validates every schema against the draft
2020-12 metaschema and every fixture against its v1 JSON Schema (including
negative cases). `go run ./cmd/validatecontracts` runs the validation alone.
CI runs the same checks. Test and demo data must remain synthetic.

## Frontend quality checks

Run the repository-level commands so both web workspaces and the desktop UI
architecture guard are covered:

```powershell
$env:PATH = "$PWD\.tools\node-v24.16.0-win-x64;$env:PATH"
npm.cmd run format:check
npm.cmd run lint
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
```

`npm run lint` includes ESLint and `npm run lint:ui`. The latter enforces the
screen/data/style boundaries in
[`frontend-architecture.md`](frontend-architecture.md), including module-size
limits, one screen per module, no direct Wails access from UI modules, and the
structural Overview contract.

`scripts/dev.ps1` treats every native tool exit code as authoritative. A failed
Go test, vet, build, npm script, Wails command, contract validator, or Gradle
task fails the wrapper instead of allowing a false-green check.
