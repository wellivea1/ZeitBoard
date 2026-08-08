# UI guideline review disposition

Date: 2026-08-04

Disposition ledger for an external UI/UX guideline covering visual language,
information architecture, flows, content, and accessibility. Claims were checked
against the running interface before being accepted.

`Applied` means the change is in this pass. `Agreed, planned` means the finding
is right and the change is larger than a documentation pass should carry.
`Already true` means the repository does it. `Declined` means the existing
commitment is better or the recommendation does not fit this product.

## Applied

| # | Finding | What changed |
|---:|---|---|
| 1 | §2.4 — do not rely on `High / Medium / Low` until calibration is proven | Confirmed: Overview rendered a confidence meter *and* badge under the heading "Confidence", as the estimate-quality answer. ADR-0022 measured those buckets inverted on real history, and the portal already withholds them for that reason — the owner's surface was less honest than the public one. Estimate quality now leads with **evidence**: how old the newest record is and what the shared freshness policy concluded. The categorical label moves behind a "Model confidence" disclosure that states plainly that episodes marked High were not more accurate than those marked Medium. It is demoted, not deleted: the reasons behind it stay useful once the reader knows not to rank by it. |
| 2 | §5.3 — every current-state presentation carries a visible but quiet freshness line | Confirmed missing: ADR-0031 added the freshness verdict to the service, and no part of the UI displayed it. Overview now shows the evidence age and the policy's explanation directly under the current state, quiet by default and escalating only when the claim is stale or withheld. A permanent warning teaches people to ignore warnings. |
| 3 | §15 — a withheld claim must not read like a normal one | When the policy withholds, the supporting line no longer says the state was "estimated from recent observations". It says the state is not being claimed and why. |
| 4 | §2.2 — "phase" is not what this estimates | The Overview kicker said "Estimated sleep-wake phase". It now says "Estimated sleep-wake **timing**", which is what the estimator produces and what §2.2's allowed vocabulary uses. |
| 5 | §3.9 — replace the "visuals win" framing | `accessibility.md` led with "visual feedback is never sacrificed for accessibility". That sentence protects the visuals, which remains the intent, but as a rule it only says what accessibility may *not* do. It now states the standard positively as **equivalent function, not identical presentation**, with visual-first retained explicitly. See the note below: this raises the floor slightly, on purpose. |
| 6 | §4.1 — five primary destinations instead of eight | **Delivered 2026-08-08** as slice U-H. `Home · Plan · Rhythm · Log · Sharing`, with Data Sources and Settings in a separate utility group. Plan hosts Calendar, Tasks and Approvals as tabs and carries the pending count, so a queue that is empty most of the time is no longer a permanent destination. Log hosts sleep, medications and context markers. Every old hash still redirects, the count is enforced by the UI-standards lint, and one tablist component replaced what would have been three. Full record in [`ui-refactor-plan.md`](ui-refactor-plan.md) §10. |

## Agreed, planned

| # | Finding | Why not now |
|---:|---|---|
| 7 | §5.1/§5.4 — Home as a quiet instrument with three quick actions | Depends on the one-tap "I woke up" / "I am going to sleep" actions, which do not exist. Those are the highest-value usability item in the automaticity review too, and belong with the P7 loop work rather than a visual pass. |
| 8 | §6.2 — richer task capture (business hours, demand, movement permission) | The scheduler core already enforces more than the form exposes. Widening the form is real work with its own contract implications. |
| 9 | §8.2 — correction preview showing the downstream forecast change | **Unblocked but not built.** The blocker named here — "requires the recompute path P7 has not built" — was removed by ADR-0033. What remains is the preview itself: computing the delta a pending correction would make, before it is applied. |
| 10 | §10.2 — `Ctrl+K` command entry | Agreed as the right shape for the assistant: a command surface, not a permanent panel competing with the current state. |

## Already true

- §11.4 — sage is limited to action and awake semantics; sleep visuals use the
  asleep blue family. Done in UI slice U-B.
- §3.5 — no punishment vocabulary. A scan for "missed goal", "failed day",
  "noncompliant", "streak", "bad sleep", "off track", and "score" across the
  frontend returns nothing outside tests.
- §7.1 — the actogram has full content width, a hover time probe, and a
  screen-reader table (U-C, U-E).
- §11.5 — all six appearance presets exist with Paper first-class, and
  rhythm-linked switching is opt-in (ADR-0021).
- §9.4 — visitor requests already state what approval reveals, above the
  buttons rather than in fine print (ADR-0030).
- §2.4's withholding requirement on the public page — the portal withholds a
  stale claim rather than showing it (ADR-0029/0031).

## Declined

**§11.3 — `Segoe UI Variable` as the named UI family.** The app ships Inter with
a system fallback. Naming a Windows-only family as the primary would regress the
Linux server-side pages and the trusted web view, and the guideline elsewhere
asks for cross-platform system fallback anyway. The existing stack already
satisfies the intent.

**§4.4 — user-pinnable secondary destination.** Agreed in principle and declined
for now: it is a preference surface, a persistence concern, and a lint exception
in exchange for saving one click, before the navigation itself has been
consolidated. Revisit after slice U-H, when there is a settled shape to pin
into.

## One tension worth naming

Adopting "equivalent function" raises the accessibility floor slightly. The
previous rule was "accessible wherever it does not compromise aesthetics or
functionality", which lets an aesthetic objection end the discussion. The new
standard requires that **core tasks** be completable without sight, without a
mouse, and without the assistant, even where that costs a little visually.

This is a deliberate change and it does not reverse the project's visual-first
stance: the guideline explicitly asks to *preserve* the strength of the visual
experience, and nothing here flattens a chart or removes a visual affordance.
What it removes is the option of leaving a core task unreachable and calling it
a design trade-off.
