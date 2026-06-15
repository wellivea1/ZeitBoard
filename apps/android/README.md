# Android companion scaffold

Native Kotlin and Jetpack Compose phase-one companion for Non-24 Planner.

## Toolchain

- Android Gradle Plugin 9.2.1 with built-in Kotlin
- Gradle wrapper 9.4.1
- JDK 17 or newer
- compile SDK 36.1, target SDK 36, min SDK 26
- Compose BOM 2026.05.01
- Activity Compose 1.13.0, Navigation Compose 2.9.8, Lifecycle 2.10.0
- Health Connect 1.1.0

## Architecture

- `domain`: repository-neutral observations, corrections, events, and imported estimate snapshots.
- `data`: fixture repositories, append-only in-memory correction/event stores, settings persistence, and the Health Connect adapter.
- `ui`: one application view model and four Compose destinations.

The Android app does not implement estimation. Fixture mode exposes a prebuilt synthetic estimate snapshot. Health Connect mode imports recent sleep sessions after the user grants only `READ_SLEEP` and leaves estimation to the shared core.

## Build

```powershell
$env:JAVA_HOME = 'C:\Program Files\Android\Android Studio\jbr'
./gradlew.bat testDebugUnitTest assembleDebug
```
