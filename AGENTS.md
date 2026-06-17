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

- The product is **visual-first** for its primary audience (people with *sighted* Non-24); visual feedback is never sacrificed for accessibility. But make every element accessible where it can reasonably be done **without compromising aesthetics or functionality**. See `docs/accessibility.md`.
- Give every control an accessible name; never convey meaning by color alone; keep the UI keyboard-operable; target WCAG 2.2 AA on desktop and Android (TalkBack). Provide a text/table equivalent for charts where it doesn't degrade the visual design.
- Don't gate shipping a visual feature on full non-visual parity; add the accessible equivalents that don't cost aesthetics or functionality.
- The **primary non-visual path is an agent + live voice**, not a transcription of the visuals: every feature must expose an agent-operable, non-visual interface — structured readable state + allowlisted *propose-only* actions through the approval gate — so an MCP client or assistant can drive it. Cloud agents are opt-in, off by default, and gated like any connected backend. See `docs/decisions/0006-agent-accessible-interface.md`.

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

