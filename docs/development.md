# Development

## Prerequisites

- Go 1.26.x for the root module and Wails service.
- Node.js 24.16.0 and its bundled npm for the checked-in frontend lockfile.
- Python 3.11 or newer for deterministic fixture tooling.
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
.\scripts\dev.ps1 -Action dev -Component desktop
.\scripts\dev.ps1 -Action build -Component android
.\scripts\dev.ps1 -Action fixtures
```

```sh
bash scripts/dev.sh check all
bash scripts/dev.sh dev desktop
bash scripts/dev.sh build android
bash scripts/dev.sh fixtures
```

Components are discovered conservatively. Missing application subtrees are
reported and skipped so the phase-one scaffold can land incrementally; a
present component that fails its command causes the script to fail.

## Fixture and contract checks

`python scripts/generate-testdata.py --check` detects fixture drift. CI also
validates each fixture against its corresponding v1 JSON Schema. Test and demo
data must remain synthetic.
