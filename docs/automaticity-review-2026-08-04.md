# Applicability, utility, and automaticity review disposition

Date: 2026-08-04

This is the disposition ledger for an external evaluation of ZeitBoard against
its own premise: that the app would follow the user's moving rhythm from
passive sources and adapt daily life around it, rather than asking the user to
maintain it. The review assessed applicability, utility, and automaticity, not
code quality.

Every factual claim below was checked against the repository before being
accepted. `Accepted` means the claim was verified and its disposition is
adopted. `Accepted with a narrower scope` means the finding is real but the
recommended remedy is broader than this project should take. `Rejected` means
the claim did not survive checking, or the existing commitment is stronger
than the one proposed.

## The finding that matters

The architecture is not the problem. The evaluation's central claim is that the
project moved sideways into platform maturity — portal, agent surfaces,
medication breadth, installer engineering — before closing the loop the product
exists for. Fresh evidence still mostly arrives because the user typed it.

That is correct, and it is visible in the code:

```text
delivered:   manual/imported sleep → drift → broad forecast → calendar/task/
             medication/portal infrastructure
intended:    Health Connect + device activity → continuously refreshed evidence
             → current estimate → task/reminder/calendar adaptation → sharing
```

Everything downstream of the estimate is well built. What feeds the estimate is
not.

## Priority findings

| # | Disposition | Verified state and remaining work |
|---:|---|---|
| 1 | Accepted | **The desktop activity collector is startup-only.** [`core/platform/activity/collector.go`](../core/platform/activity/collector.go) is 58 lines and emits one observation with `"state": "startup"`. `SafeCollector` in a package called `activity` reads as more capability than exists. Real evidence needs lock/unlock, idle transitions, suspend/resume, and display state. Until then desktop activity is not an evidence source, and no document should imply otherwise. |
| 2 | Accepted | **Android ingests Health Connect sleep but cannot reach the estimator.** The companion has `HealthConnectSleepRepository` and `LocalUserDataRepository` and no enrollment or push/pull client. The device most likely to hold fresh wearable sleep cannot close the desktop loop. This is roadmap slice 9 and is now the highest-value single change in the project. |
| 3 | Accepted | **The owner's current-state claim has no freshness policy while the public one does.** The portal withholds an estimate past 6 hours as stale and past 24 hours entirely (ADR-0029). The desktop Overview has no equivalent: a search for freshness, staleness, or generation-time handling in `OverviewScreen.tsx` and `app.go` returns nothing. The stricter policy was written for strangers and never applied to the person who depends on it. One shared policy should serve desktop, server, local agent, and portal. |
| 4 | Accepted | **The desktop presents categorical confidence labels the measured data does not support.** Overview renders a Confidence badge and meter, while ADR-0022 measured the buckets inverted on real history — High at 0.61 hit rate against Medium's 0.81 — and the portal withholds them for exactly that reason. The owner-facing surface is currently less honest than the public one. Recalibrate, rename to what the value actually measures (model fit or evidence quality), or withhold the label; do not fix it by widening already broad windows. |
| 5 | **Delivered 2026-08-08** | **The Sharing screen still says link creation is not connected.** `SharingScreen.tsx` reads "Link creation and recipient access are not connected in this build" and shows "Not connected". That was true when written and is now stale in a worse way: P5-a shipped owner-side profile create/list/revoke/erase on the backend, and nothing in the desktop was wired to it. The screen is honest about the user's experience and wrong about the system's capability. |
| 6 | Accepted | **Approval is uniformly coarse.** Propose-only is correct for external, coordinated, destructive, and medically adjacent actions. Applying the same ceremony to reversible internal operations — reordering the task list, moving an internal reminder inside a user-defined range, refreshing a projection — is burden without a corresponding safety gain, for a user whose cognition is worst exactly when the app asks most of them. See the scope limit below. |
| 7 | Accepted | **Analysis is event-insensitive.** Recomputation is driven by screen loads and local mutations rather than a durable queue that reacts to a synced record, a correction, or a source conflict. An assistant that only updates when looked at is a report. |
| 8 | Accepted with a narrower scope | **Pause portal breadth.** P5-c messaging threads and P5-d live/SSE are paused, not cancelled. Their design stays in `portal-design.md`. The already-built P5-a/P5-b surfaces stay maintained and tested. Public exposure was already prohibited and remains so. |
| 9 | Accepted | **Validation measures correctness, not benefit.** The suite proves the software does what it claims. Nothing measures whether the user does less work or decides better. The pilot metric framework below closes that gap. |

## Where this ledger departs from the evaluation

**Rejected — the proposed activity-privacy floor is weaker than the existing
one.** The evaluation suggests foreground application *category* "only if later
justified — never window titles by default". `privacy.md` already commits that
activity collection "must not retain application names or content", with no
by-default escape. The stronger commitment stands. A future proposal to collect
application categories would be a privacy-policy change requiring its own
review, not a collector implementation detail.

**Narrowed — "approval is too coarse-grained" must not touch the agent
surfaces.** Finding 6 is about the *user's own* reversible operations under an
explicit opt-in policy. It is not licence to relax ADR-0012's invariant that no
agent surface exposes an approve or apply tool, nor ADR-0016's one-use decision
tokens, nor ADR-0030's rule that a visitor request is decided by a human
choosing an exact block. Anything an agent, a visitor, or an external system
can reach stays proposal-only. The relaxation applies to internal state the
user could undo without consequence, and every such action must record undo.

**Not adopted as repository facts — the numeric readiness scores.** The
evaluation's per-dimension scores are useful as a summary judgement and are not
measurements. This ledger records the findings and their dispositions; the
pilot metrics below are the numbers this project should be held to.

**Qualified — the pilot targets are targets, not commitments.** Coverage of
85%, boundary error under 45 minutes, and the rest are starting points to be
revised from measured live data. The evaluation says this itself, and it is
worth repeating: a target must never become a reason to publish a more
confident output than the evidence supports.

## Progress against this ledger

Findings 1, 3, and part of 7 are addressed by
[ADR-0031](decisions/0031-evidence-freshness-and-shadow-inference.md):

- **Finding 1 (startup-only collector) — addressed.** The collector now records
  eight behavioural states through two narrow system calls, with the judgement
  in a pure, tested state machine. Its remaining half is reliable background
  execution while the window is hidden.
- **Finding 3 (no shared freshness policy) — addressed.** `core/freshness`
  decides on evidence age, and the desktop, server projection, and portal
  materializer all defer to it. The work also uncovered that the portal's own
  policy measured the wrong thing, which is now fixed.
- **Finding 7 (event-insensitive analysis) — partly addressed.** Shadow
  inference exists and can turn activity into candidates. The durable recompute
  orchestrator does not.

Findings 2 (Android sync), 4 (confidence labels), 5 (the Sharing screen), 6
(approval granularity), and 9 (benefit metrics) are open.

## Resulting sequence

The next phase is the automatic loop. `phase-goals.md` carries the pasteable
goal; the order is:

1. Android synchronization onto the existing enrollment and push/pull model.
2. A real privacy-minimized Windows activity collector.
3. Multi-source candidate sleep inference, shadow-only at first.
4. A durable event-driven recompute orchestrator.
5. One shared freshness and withholding policy across every surface.
6. A 48–72 hour operational view, ahead of emphasizing seven-cycle forecasts.
7. Confidence recalibration or honest relabelling.
8. Bounded reversible automation under explicit user policy.
9. Only then: wire the Sharing screen, complete the independent review, and
   release read-only availability before requests.

An inferred episode may influence production planning only after a documented
positive validation decision, on the same measured-delta gate ADR-0022
established for estimator changes.

## Pilot metric framework

Correctness tests stay as they are. These measure benefit, and belong in a
30–60 day private pilot report rather than in CI.

**Passive coverage.** Principal sleep episodes entering the estimator without
manual transcription; days with two or more independent evidence sources;
Android observations synced within 30 minutes when both devices are online;
collector uptime during logged-in sessions; median daily minutes spent
maintaining the app.

**Boundary inference**, measured separately against user-confirmed boundaries
and against wearable records: median onset and final-wake error; P90 boundary
error; false principal-sleep rate; quiet-wake false positives; median time to
complete a correction.

**Forecast utility by horizon** (next cycle, 24 h, 48 h, 72 h, seven cycles):
point error, interval coverage, interval width, refusal rate, stale-output
rate, and calibration by evidence quality. A forecast must not pass by becoming
wider.

**Task and calendar utility:** suggested windows accepted, proposals moved or
rejected, tasks completed in the suggested window, reminders auto-shifted then
undone, office-hours tasks surfaced while both the office is open and the user
is awake, deadlines missed despite a feasible window, and honest no-feasible-
window refusals. Segment by task type, because an office call and a household
chore are not the same scheduling problem.

**Resource cost:** idle working set and CPU, CPU during recomputation, database
growth per month, startup time, background wakeups, laptop battery impact,
Android battery and scheduled-work cost, and sync bandwidth. Baselines must be
recorded before the collector and orchestrator land, or the cost of adding them
cannot be stated.

**Manual burden, as of 2026-08-09.** One-tap logging (roadmap slice 15) does
not clear the blocker below and is not claimed to: a tap is still a self-report,
and every principal sleep still needs two of them. What changed is the cost of
each one — two taps instead of a four-field form filled in by someone who has
just woken up — and the honesty of the result, since an implausible pair now
produces a question rather than a recorded night. The blocker asks for sleeps
that arrive *without* the user, and that still depends on items 1 and 2.

**Release blockers.** The product must not be described as automatically
rhythm-aware while any of these hold: Android data does not reach the
estimator; desktop activity remains startup-only; more than half of principal
sleeps require manual entry; current-state claims do not expire when sources go
stale; confidence labels remain demonstrably inverted; or task suggestions are
not measured against what the user actually did.

## Pilot staging

Shadow collection first (passive sources run, inference cannot affect
planning), then forecast shadowing (an alternative estimate is compared, not
used), then recommendation mode, then reversible low-risk automation under an
explicit policy, and only then a single trusted-recipient trial with coarse
availability, short expiry, and no messaging.

Each stage should publish the same compact table: version and commit, pilot
dates, source coverage, manual burden, boundary error, forecast error and width
by horizon, refusal rate, task acceptance, stale-claim incidents, privacy or
security failures, resource use, and a ship / revise / reject decision.

## What is explicitly not being rewritten

The Go core, the Wails desktop, the native Android app, SQLite local storage,
immutable observations with append-only corrections, versioned contracts, the
deterministic scheduler, the proposal and approval boundary, the separate
portal store, the BYOK assistant, and the real-history backtest gate all stay.
This is a change of sequence, not of stack.
