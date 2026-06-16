# ADR 0005: Desktop theming and reduced-stimulation controls

- Status: accepted
- Date: 2026-06-16

## Decision

Add an Auto / Light / Dark appearance selector and an independent reduced-stimulation toggle to the desktop application. The implementation is local to the desktop frontend and does not change the core, contracts, or Android shell.

- Theme preference is stored under `localStorage` key `zeitboard-theme` with values `auto`, `light`, or `dark`.
- Reduced-stimulation preference is stored under `localStorage` key `zeitboard-reduced` with values `true` or `false`.
- An inline script in `index.html` reads these preferences and applies `data-theme="light|dark"` and `data-reduced="true"` to `<html>` before first paint, avoiding a flash of the wrong theme.
- A small React provider (`AppearanceProvider`) exposes the current preference, effective theme, and setters; it re-applies attributes when the user changes settings and listens for `prefers-color-scheme` changes when the theme is Auto.
- The existing CSS custom-property system is extended with semantic variables for surfaces, text, status, confidence, buttons, and charts. Dark mode overrides these variables under `[data-theme="dark"]`. Reduced-stimulation mode removes shadows, disables transitions, softens saturation on decorative cues, and increases spacing under `[data-reduced="true"]`.
- Interactive targets across the desktop UI meet a 44 px minimum touch target.

## Consequences

- The desktop UI can be used comfortably in light, dark, and reduced-stimulation modes without maintaining parallel stylesheets.
- Android and trusted-web prototype theming remain out of scope and are tracked separately.
- Theme and reduced-stimulation preferences are device-local and are not part of the v1 contracts or shared fixtures.
- Future features that add new colors or status cues must define variables for both light and dark modes.
