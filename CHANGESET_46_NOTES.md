# Change set 46 — candidate event cards

Branch `slug-event-card`, stacked from set 45 implementation
`ca84a62d178386646aef119cbe2edbb6ad38f508`. The first branch commit records
set 45's completed engineering closeout. Implementation `ede24725` was pushed
after source and independent evidence review. Its actual normal-clock package
test exposed a metadata responsiveness problem; a narrow correction and renewed
package acceptance are in progress.
No set 46 deployment is claimed yet.

Connect the complete noon snapshot and fixed, finalized candidate seed to one
retained card for the event. Mike approved scoped candidate exception C1:
two gyms, zero professional records, same class, inclusive half-to-double D65
rating band. Only the widen-to-five trigger is waived. See
`CANDIDATE_CARD_EXCEPTION.md`; D13, D28 and D52 remain unchanged.

Every entered fighter must be accounted for by a booked pair or a retained
unmatched reason. Publish total and per-gym counts. Preserve cutoff eligibility,
ownership attribution and condition; later roster edits or retirement do not
change the card. A source conflict remains visible without replacing the seed
or rewriting a retained card. Recover the same card after interruption or a
lost acknowledgement; an explicit repeated request cannot reshuffle it.

The fixed two-gym scope has at most one opposing entrant in each class. It
cannot establish general random-draw fairness, five-opponent widening,
professional-record filtering or next-week priority. Those policies stay open;
the unmatched evidence will inform later decisions. This set books candidate
bouts; professional result/award authority remains a separate workstream.

## Implementation

`gymcard` stores the complete source, deterministic plan and immutable card in
one synchronized record. A separate status reports current source health;
conflict does not overwrite saved pairings. Explicit replay retains the same
card and creation time. The v6 candidate parent owns event, seed and card stores
across logout, reset and account switches, and drains the helper before closing
them. Installation resumes its recorded prepare step and refuses missing
established data.

The desktop adds Build, status, record and paged history through the existing
serial work lane. The native boundary validates the closed transport and current
view. The frontend preserves newer explicit observations over older completed
jobs and accounts for its reserved card bytes within the shared cache, including
portrait admission. An unknown write outcome is shown without an automatic retry.
Candidate build 0.4.6/build46 enables the card family; normal bundles contain no
test clock or cheap-PoW profile. See `EVENT_CARDS.md` for the contract and bounds.

## Verification recorded so far

- The reviewed independent reference was frozen before comparison. Runtime and
  reference were developed concurrently; the reference was not derived from Go
  card outputs. Its retained corpus includes 60 matcher cases, 27 band cases,
  3,861 Saturday identities, closed-codec and transport cases, and bounded state
  models. Frozen reference bytes and earlier failed diagnostic attempts remain
  preserved. Private installation/filesystem behavior has separate concrete tests.
- Core comparisons cover 935 supported public codec/source cases, all calendar
  identities, matching and record-to-plan derivation. Targeted journal, crash,
  cancellation and account-lifetime checks pass under normal and race builds.
  The parent installer also recovered after actual process termination at each
  of three durable setup stages under both builds. Two empty test comments were
  removed after the executable checks; the preserved delta changes no code.
- The populated 256-card helper journal returned first and late eight-row pages
  in about 149 ms and 139 ms, within its unchanged 250 ms limit; exhausted history
  took about 1 ms. This is a normal-build capacity check, separate from race
  instrumentation. Desktop normal and race checks pass. A later cheap-profile
  check exposed a missing skip for a normal-profile fixture; only that test
  loader changed, and the earlier attempt remains preserved.
- All 181 frontend tests pass, including 18 card controls. Independent review
  found two stale-status presentation issues, which were fixed and rechecked.
  A later review added the existing core rule that card creation cannot precede
  source finality to both the helper transport and standalone UI status check.
  Targeted normal/race checks and the final frontend suite pass.
  This is DOM/state testing, not visual, accessibility or D74 acceptance.
- The first actual owned-chain run used normal PoW and an explicitly scoped test
  clock. It completed in 265 seconds, retained four admissions per gym, and
  entered three fighters: two paired in one bout, one unmatched because no
  opponent shared its class. The unmatched fighter was Emil Marsh, light heavy.
  The immutable card survived held-session reads/replay, restart and later target
  conflict. Independent arithmetic reproduced the card and checked all 175
  exported chain headers, including the fixed target with 60 descendants; 16
  tamper controls were rejected. A final-source native private-pipe replay then
  reopened the genuine journals, checked both gyms including a held session,
  preserved the conflicted card and exported three complete public admissions.
  Its captured listeners and owned processes closed cleanly. The initial
  pipeline's exact runtime differences remain documented; its original run is
  not relabeled as having used the final source.
- The independent admission addendum verified all three complete public proofs,
  signatures, generated fighter facts and birth/source bindings. The normal
  release smoke synchronized to the unchanged height-61 public seed, verified
  its report-only checkpoint and restarted cleanly. GitHub returned no runs or
  checks for the pushed implementation; remote CI is not claimed passed.
- The first package attempt failed icon conversion under the sandbox, which also
  blocked cleanup inspection; a separate process check found no owned survivors.
  The next attempt built the approved icons and all three signed normal-profile
  binaries, then failed on the first ready card-status request. Its original
  `outcome_unknown` response has no raw timing capture. Separate cold-copy runs
  of that exact helper produced seven valid `busy` responses at 253–267 ms.
  Instrumentation isolated repeated source authorization/clock-sync work;
  this is a real-clock liveness failure, not a malformed response. The previous
  stationary test-clock capacity check did not cover that cost. All failed
  attempts and diagnostic source bytes remain preserved; none is a package pass.

Workspace evidence includes `../set46-reference-root-review.json`,
`../set46-core-independent-review.json`, `../set46-frontend-checks.json` and
`../set46-independent-public-card-verification.json`. These record their precise
scope and source/input hashes. The initial chain export is pre-reorg evidence;
it is not a claim of continuous consensus observation or production authority.

## Remaining release gates

Validate the prepared source-view correction for Card Read, Status and History,
including original-session authorization before retained-row validation, final
authorization, post-sync uncertainty, default-clock empty metadata and advancing
scoped-clock capacity timings with the actual account journal. That stronger
capacity test also found duplicate immutable card validation exceeding the
budget. The additional correction retains a private bounded digest index from
the full opening audit and successful inserts; reads compare actual stored bytes
to that audited identity. The complete current seed-source audit remains in
place. No external validation verdict or imported record is accepted.
The full parent-equivalent account checks still exceeded the original 250 ms
limit after those reductions. An explicit engineering review approved a Card-only
500 ms metadata context budget; preserve the original failures, immutable card
bytes, shorter caller deadlines and existing Build behavior. Other families and
the separately proposed Set47 Bout budget do not inherit this change.
Then renew exact-source packaged helper acceptance and independent checks,
release smoke as required for the corrected source, and fresh live seed preflight.
Then promote the reviewed source, verify Railway's
deployment and complete the 15-minute observation plus restart. Report remote CI
honestly if GitHub supplies no runs. Later closeout records must identify the
actual implementation and deployed commits; this document does not predeclare
those checks passed.

Keep the original detailed app icon and simplified letterless menu icon.
The portrait check and full joint D74 review remain pending before product
release. No private operator keys, public-chain mining, official activation,
production ownership or consensus changes are authorized by this set.
