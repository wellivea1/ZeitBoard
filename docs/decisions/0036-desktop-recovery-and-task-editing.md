# ADR 0036: Desktop recovery and lossless task editing

- Status: accepted
- Date: 2026-09-08

## Context

Desktop read failures could select the browser fixture fallback. Home also
initialized with sample forecasts, and nested Overview data could omit the
freshness verdict and crash. Time-sensitive views did not refresh solely
because time passed. The backend supported task updates but the UI exposed
only creation, completion and deletion; task read DTOs exposed timing as prose.

## Decision

- Fixture fallback is for an absent desktop bridge only. A present bridge with
  a missing method, invalid payload or failed read yields unavailable data.
  Personal projections never initialize to sample forecasts. Freshness is
  normalized on every supported Overview shape and cannot be trusted when
  its state is withheld or stale.
- Use a route-level render boundary with retry/reload/navigation recovery.
  Do not log or display raw exceptions that could contain private values.
- Re-read visible time-sensitive projections every minute and on focus/return.
  Home/Rhythm reads coalesce; hidden views do not poll. This does not change the core
  freshness policy, and it does not authorize background collection.
- The desktop-local task DTO now includes exact earliest/latest instants,
  preferred minutes after wake, and minimum confidence. The UI does not parse
  labels. Old label-only DTOs remain readable but editing is disabled.
- Task input accepts either a civil time plus IANA zone or an exact RFC 3339
  instant for each boundary, rejecting simultaneous representations. The
  editor resubmits untouched boundaries as exact instants, preserving seconds
  and the second occurrence of a repeated DST hour. Newly edited civil times
  use the core resolver: reject gaps, choose the earlier repeated occurrence.
  Revision checks remain mandatory and a stale edit is never silently rebased.
- This DTO is local to the desktop bridge. Existing persisted task contracts,
  sync records, server projections, private/allowlisted agent boundaries and
  approval-required calendar placement are unchanged.
- Task deletion needs a separate explicit decision. A failed save or refresh
  must keep the user's draft; refresh failures identify last-loaded records.

## Consequences

Service failure is distinct from insufficient evidence and can be retried.
The current-state UI can be up to one visible refresh interval behind the
core's time-based verdict, with immediate refresh after returning from sleep.
Read failures clear stale local proposal candidates. Task edits round-trip
constraints without reparsing prose or shifting times after travel. No new
database migration, network permission, data sharing, or agent mutation path
is introduced.
