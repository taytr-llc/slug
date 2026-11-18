# Change set 49 — finality-timed career awards

Implemented on `slug-career-awards`, stacked from completed Set48 topic
`dd74828f3957d252f16f0e4925e2bcbb518ed540`. Local runtime and actual normal-PoW
composition gates pass. Final normal package and deployment are still pending.

## Decision

D78 applies permanent records, XP and all result-derived points/title changes
exactly once at the result's own signed checkpoint plus sixty descendants.
Provisional outcomes remain labeled and grant no permanent effects. Later
reported chain conflicts retain finalized awards. Noon snapshots consume only
awards already effective at lock. See `RESULT_AWARD_POLICY.md`.

## Implementation and verification

The implementation adds explicit prospective career activation to the existing
Result journal, preserving publication identities and its reservation book.
Activation refuses journals with any pre-existing Finality. A first post-activation
Finality is the one durable both-fighter award event; no separate increment or
Apply operation exists. The candidate slice grants standard W/L/D, XP capped at
15, and base points. It does not invent ranked-bout or title inputs.

Stable and Card bind the concrete career reader. Current display projects
finalized XP through unchanged condition/D65, retaining the original raw page.
Entry, noon capture and first booking enforce C1's zero-record restriction in
the core stores. Strict-before cutoff receipts commit their complete bounded
set; a missing receipt is detected on reopen. Existing snapshots, saved cards
and bout inputs remain unchanged. The separate desktop career route uses a
recoverable v9 parent and preserves the legacy Result route.

The pure arithmetic, temporal accounting and receipt-commitment references were
independently derived and frozen before their Go comparisons. The first temporal
model contained impossible owner clocks; original evidence was preserved and
the independent author corrected the contexts before comparison. The review
also found and closed a missing-receipt audit gap. Exact evidence is retained in
the workspace's Set49 reports; no old corpus was rewritten to match Go output.

Completed local gates include reference comparisons, focused race checks, the
full tagged Stable/Card/Entry/Event and Result regressions, and core activation
process-kill recovery. The concrete 256-event Result audit took 4.674 seconds;
Current/Cutoff reads took about 3.28/3.97 ms under the accepted profile with four
Go execution threads. These capacity rows are modeled, not mined evidence.
The first actual two-fixture desktop setup/recovery test passed with empty career
reads around 132–137 ms; that early run used the default thread count and is not
full-capacity acceptance.

All six real parent process-kill recovery boundaries passed in 100.93 seconds,
preserving activation and history, refusing legacy writers and refusing missing
ready guards. The first full-capacity parent startup passed in 9.33 seconds.
Its companion pilot fixture incorrectly expected awards for three inventory IDs
absent from its modeled career history; that failed attempt is retained. The
corrected page measurements below are separate evidence. The final asynchronous
parent repeated all six process-kill recovery boundaries in 103.25 seconds.

The corrected three-item full-history read passed at about 158 ms. A separate
eight-item diagnostic first passed with the exact-header cache warm, then failed
with the cache cold: the synchronous page reached its 500 ms deadline during
normal proof verification and returned no payload after joining the running hash
(about 740 ms total). Both measurements are retained with those scopes. The
correction moves proof-backed pages through the existing 30-second asynchronous
Stable job and uses an opaque, original-session-bound handle for the 500 ms
read. The core handle implementation passed independent review, focused normal
and race checks, and the full tagged Stable suite. Desktop focused checks and all
242 frontend checks also pass.

The corrected cold eight-item diagnostic passed with 256 modeled career and bout
histories and actual fixture accounts. Inventory/roster proof jobs took about
1.92/1.86 seconds; their separate metadata reads took 84.05/88.07 ms. The diagnostic
cleared the exact-header cache before each job and again before each metadata
read: all eight normal proofs were verified during each job, and neither read
performed another header hash. Exact terminal source pages and career totals
matched. Test-only cache controls stayed outside shipping source; modeled
acceptance/history inputs do not establish new chain admissions.

The final desktop normal regressions, focused race checks, normal/test-clock
static analysis and test-clock/cheap-profile compilation pass with unchanged
selected sources. The first normal run was refused by the sandbox's loopback
restriction; that failure remains recorded separately from the successful
authorized loopback run. Cheap PoW is used only for diagnostic compilation,
never for the normal package or chain acceptance.

The first actual normal-PoW Career composition prepared its genuine source and
saved the provisional bout, then failed its funding reconciliation before any
result publication. The new `career_init` metadata key shares the `c` prefix
used by funding coins: opening audit recognized it, but funding enumeration
treated it as a malformed coin. The failed owned copy and logs are retained;
all owned processes shut down without forced cleanup. The corrected scanner
accepts only the exact, canonically encoded activation intent in an active v2
store. Malformed or similar coin keys and legacy-store intents still refuse.
The regression first reproduced the defect; the correction then passed focused
normal/race tests, full normal/scoped-clock Result suites and independent review.
The second actual composition completed the result proof path and its first
eleven career observations, then failed at the final C1 bootstrap assertion:
the test expected `ready`, while a healthy attached Entry snapshot reports
`idle`. This was a test-only error. The correction requires `idle`, no snapshot
error, both original session Views and a matching store. A diagnostic on a fresh
owned copy then confirmed the exact awarded fighter is refused, with no entry
receipt or revision change and its existing award retained. Neither failed full
attempt is relabeled passed.

The fresh third actual composition passed in 341.41 seconds, including all twelve
career observations and the final C1 refusal. It retained one both-fighter award
through repeated attestation, restart, reported conflict and reset-held reads;
all owned processes shut down without forced cleanup. The independent verifier
checked six public result proofs, 279 distinct normal-Argon proof headers, the
single award event and all twelve captured career views. Its actual-evidence
controls passed one positive and twenty-one deliberate corruptions: twelve
inherited proof/source checks and nine career checks. They reused only the exact
independently verified work evidence, performed no repeated Argon work, and left
the original evidence unchanged. The normal-PoW composition uses a scoped fixture
clock and preserved forced-invalidation restart recovery; it does not claim
real-calendar closed-event coverage or uninterrupted invalidation liveness.

All ten module builds, five retained conformance packages, formatting and final
source/profile checks pass. No source membership or byte changes occurred during
that module gate.

Remaining gates are final normal package verification, then
the authorized promotion and deployment observation. Set49 is not deployed.
Production authority, permanent genesis, portrait/GUI checks and the joint D74
design review remain open. Awarded fighters cannot enter the old zero-XP practice
preview; a separate career-aware preview contract has not been introduced.
