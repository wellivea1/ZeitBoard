# Repository conventions

## Product language

- Say **estimated sleep-wake phase**, **predicted sleep window**, **predicted waking window**, and **confidence/uncertainty**.
- Never claim that device activity identifies exact circadian phase or DLMO.
- Do not add diagnosis, treatment, medication, light, meal, or exercise recommendations.

## Architecture

- The Go core must not import Wails.
- Platform code belongs behind interfaces and OS build tags.
- Imported observations are append-only. Corrections are a separate layer.
- Every inferred value carries provenance, creation time, confidence, and algorithm version.
- Fixed calendar events are immutable inputs. Scheduling returns proposals.
- Android consumes observations and estimates through repositories/contracts; it does not reimplement estimation.
- Sharing uses explicit allowlisted projection DTOs. Never serialize the private domain model directly.

## Accessibility

- Screen-reader + keyboard operation is a **first-class mode**: many people with Non-24 are totally blind, so non-visual users are core, not edge. See `docs/accessibility.md`.
- Ship a text/table equivalent for every chart or visual-only element; give every control an accessible name; announce meaningful state changes via polite live regions; never convey meaning by color or position alone.
- New or changed UI must be keyboard- and screen-reader-operable before it ships, with its non-visual equivalent in the same change. Target WCAG 2.2 AA on desktop and Android (TalkBack).

## Privacy

- Local-first, no analytics, telemetry, tracking SDKs, or health-data upload by default.
- Never collect keystrokes, typed content, screenshots, browser history, or active-window titles by default.
- Do not log health payloads, notes, medication names, calendar content, tokens, or exact behavioral timestamps.
- Fixtures, screenshots, static web assets, and tests must contain synthetic data only.

## Engineering

- Pin tool and dependency versions.
- Format Go with `gofmt`, Kotlin with the configured formatter, and TypeScript with Prettier.
- Add focused tests for time-zone behavior, provenance/corrections, estimation refusal, scheduling constraints, and permission projection.
- Keep changes scoped and document architecture changes with an ADR.

