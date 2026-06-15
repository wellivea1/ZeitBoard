# Roadmap

## Phase one: executable scaffold

- Establish strict v1 contracts and deterministic synthetic fixtures.
- Implement append-only observations/corrections and effective reads.
- Implement robust drift estimation with typed refusal and uncertain forecasts.
- Produce deterministic schedule proposals without mutating fixed events.
- Build desktop, trusted-web fixture prototype, and Android fixture shell.
- Enforce default-deny projection, privacy checks, and feasible CI builds.

Exit criteria are the acceptance checks in `implementation-plan.md`, with any
environmental build limitation recorded explicitly rather than hidden.

## Phase two: local usability

- Harden import validation, conflict handling, retention, export, and deletion.
- Represent source-specific missingness and forced-schedule/travel disruptions.
- Add user-visible provenance and correction history.
- Add refusal states, correction diffs and undo, proposal review, and onboarding.
- Improve accessibility, localization readiness, and time-zone test coverage.
- Implement light/dark parity and independent reduced-stimulation controls.
- Evaluate local database encryption and operating-system credential storage.
- Run structured usability research using synthetic or participant-controlled
  data and conservative product language.

The implementation/deferment map for the added analysis and UI specifications
is maintained in [`specification-alignment.md`](specification-alignment.md).

## Phase three: optional interoperability

- Define a migration path for contract versions and local database schemas.
- Evaluate read-only calendar adapters with least-privilege permission scopes.
- Prototype encrypted or minimized relay transport only after the relay design
  and threat model receive a separate security review.
- Add explicit remote revocation semantics and metadata-minimizing operations.

## Deferred

No phase currently includes exact circadian phase/DLMO claims, autonomous health
recommendations, hidden background collection, advertising, data brokerage, or
default cloud upload. Such changes would require a new product scope, privacy
review, threat model, and user-consent design.
