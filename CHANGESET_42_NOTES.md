# Change set 42 — deterministic unrecorded fight preview

Branch: `slug-fight-preview`, staged from Set41 implementation
`bbce6106c95eaa9284db38cdebd50c754c8ec775`.

Engineering closeout is complete. Implementation `81ef2816` was promoted to
master as `fc672113` on November 17 after packaged acceptance and the required
normal-PoW smoke. Railway deployment `97799d14-3dc3-42af-8685-98783a82059c`
succeeded; the full 15-minute observation and restart verification passed.
D75 studies passed independent audit and successor acceptance. Existing D65
stays unchanged. No coefficient-v2 implementation is included.

The candidate gym now has a deterministic fight preview between two saved
same-class candidates. Selection is explicit and survives inventory paging.
The helper verifies both retained assignments and obtains one trusted as-of,
then computes a bounded three-round transcript. The frontend shows a paused,
unrecorded result with local Play/Pause/step/restart controls, native portraits
and a whole-fight scorecard. This creates no professional result, XP, training,
entry lock or official league entitlement.

## Implementation and compatibility

New `fighterbout` Version1/`slug-fighter-bout-candidate-v1` uses checked integer
fixed point, half-even rounding, framed SHA256 streams, canonical full-ID corner
ordering and bounded rejection sampling. Complete canonical Input/Result bytes
have explicit digests. Result validation re-simulates in the pure engine; native
polling instead uses bounded shape/commitment checks without running the engine
or doing proof work.

`gymstable.ReadPreviewPair` reads only existing assigned IDs from the pinned
trusted source, verifies at most two proofs serially, preserves original birth
and uses one final original-session View lease. Only existing observation
metadata may advance. It never implicitly catches up assignments or changes
roster/operations. The desktop pins zero XP/training until real event authorities
exist. The explicit seed is predictable and has no future-block fairness claim.

The separate Preview v1 command family shares the current helper pipe and work
lane. One active plus one queued task bounds work across mining/stable/preview.
Cancellation joins the current work; stale views cannot borrow a new session.
Private receipts are removed before publication, and only one owned result is
retained for the current view. Restart drops it. Typed native methods correlate
pair/job/view/result, and portrait disclosure requires fresh job authorization
even for a cached SVG. Old Manifest/Stable commands and DTOs remain exact.

No stable migration or new namespace is introduced. Condition v1, its original
D65 coefficients, stable pins/StoreIDs, original admission birth, assignment
records, roster choices, operation history and preview-v1 recipe/seed stay
unchanged. Mining/account/backup behavior and all enumerated D72 hold restrictions
remain; a currently authorized held view may use this unrecorded preview.

See [FIGHTER_BOUT.md](FIGHTER_BOUT.md) for the pure model/API and
[FIGHT_PREVIEW.md](FIGHT_PREVIEW.md) for the trusted read, protocol, caches and
actual-helper acceptance boundary.

## Numerical evidence and D75

The independent Python model and corpus were frozen before Go execution. All
55 complete canonical transcripts/IDs/digests match, with additional fixed-point,
stream/rejection, physical-grid, finish, invalid-input and codec coverage. Normal,
race, tagged, vet and fuzz gates pass at captured hashes. Expected bytes were
not generated from the Go runtime.

The original predeclared calibration **failed** its exact rounded-refit-equality
criterion: measured `[0.9,2.5,0.2,0.6,0.8]` differed from D65
`[1.0,2.5,0.2,0.4,0.8]`; its other numerical gates passed. Current D65 scored
85.05% on 2,000 held-out prime fights, versus 85.40% for the continuous fitted
predictor. A separate rounded-tuple diagnostic scored 85.65%, with a paired 95%
improvement interval of −0.10 to +1.35 percentage points. It did not establish
superiority beyond uncertainty. The original report and failed rule are retained.

After those results, the user made D75: keep D65, rescind the exact-equality
criterion, and require positive paired bootstrap support excluding zero for
future coefficient changes across prime, aging and XP-varied coverage including
close pairs. The incumbent wins ties or inadequate evidence. This is an explicit
post-result policy decision, not a retroactive claim that the original gate
passed. No alternative is automatically adopted regardless of study outcome.

The D75 coefficient bootstrap completed 1,000 actual refits: all were valid,
eight Decimal50 references agreed and the exact rounded proposal occurred in
161 replicates. The fresh study completed 20,000 pairs across prime/aging/XP
and natural/close/band-edge cohorts, with 10,000 paired resamples per panel.
All eight primary improvement intervals include zero. Independent audit
reconciled the retained fits, scoring, coverage, intervals and original hashes.
D65 stays unchanged. Finish/duration behavior is broadly similar to the source
simulator, but draws are less frequent; distributions are not identical.

[RATING_VALIDATION_REPORT.md](RATING_VALIDATION_REPORT.md) contains both full
study summaries and limitations. [RATING_VALIDATION_POLICY.md](RATING_VALIDATION_POLICY.md)
records the policy and preserves all other original numerical/source criteria.
Explicit D75 successor acceptance allows this existing engine to proceed through its
remaining engineering/package/deployment gates with D65 unchanged. They do not
authorize a hidden weight change, refit to a chosen answer or alteration of the
original expected corpus/report. Any later model revision informed by these
outcomes needs its own declared protocol and disjoint held-out namespace.

## Verification status and remaining gates

Focused helper/core tests passed for original-session/cancellation, same-time
projection, queue/capacity, private-evidence stripping, clock uncertainty and
no implicit catch-up. Native/UI review closed three concrete findings: stale
Snapshot eviction of a newer completed result, repeated-result correlation
omitting captured Pair.Status, and typed failure leaving old replay state.
Tests now exercise the corrected boundaries and fresh portrait authorization.

Final source checks include ten chain-module builds, full normal desktop tests
(49.554s), full tagged desktop tests after the explicit test-only fixture-profile
correction (3.930s), desktop integration compile-only (1.495s), vet and 111 frontend
tests. Before/after captured sources did not drift. The earlier tagged failure
is preserved separately. These timings describe local evidence, not CI or a
packaged/native-product acceptance claim.

The first two-wallet packaged attempt passed build, signature and mode checks,
then failed during login: default sandbox restrictions denied a loopback port,
and a failed child constructor exposed a typed-nil interface cleanup panic.
The failed bundle, test data and logs are retained. This was a real error-path
bug in both candidate modes, not a statistical or fight-engine failure.

Login now transfers a child into cleanup ownership only after construction
succeeds. Before-fix tests reproduced both pointer-type panics; after-fix tests
verify the failed state, cleared view, revoked original session, immediate Close
and successful same-account retry without mining. Focused normal/race checks
passed, as did complete account-component normal/race/tagged suites with required
loopback access, vet and formatting. The default-sandbox full-suite failure is
preserved separately. No protocol vocabulary, model, study or rating changed.

Fresh exact packaged acceptance passed for the repaired commit: two published
fixture wallets, 14 genuine normal-work records, 40 condition projections and
six complete independent preview comparisons across initial/reopen/held phases.
Public results, portraits and preserved wallet/stable facts passed independent
review. Both helpers exited successfully and all observed owned listeners
closed. These observations do not establish every node's direct exit code.
Three cached result rereads are not frontend playback testing.
Actual visual rendering, native chooser interaction,
screenreader/accessibility, local playback in the packaged UI and joint D74
review remain outstanding.

The live observer took 31 samples over 900.047 seconds, retaining height 61,
60 descendants of the signed checkpoint and the exact canonical hashes.
Restart and normal-PoW verification passed; the unchanged old tip still refuses
fresh mining templates. Railway reported one healthy replica with no crash or
pending rollout. Its volume and proxy configuration stayed unchanged. The
inherited missing-default-config warning was also present in set 41; explicit
startup flags work. No public block was mined.

No GitHub Actions run/check existed for the exact implementation commit despite
Actions being enabled. Remote CI is not claimed passed; captured local gates
and independent reviews support this engineering release. The first failed
package and a pre-push verifier failure on an inherited documentation symlink
are retained separately. The symlink verifier was narrowly repaired and
independently reviewed before promotion.

D74 and the actual visual/native chooser/screenreader gate apply
before friends/tester/public product release; they are not an additional veto
on that authorized engineering deployment process.

No chain consensus, wallet derivation, permanent genesis, signed checkpoints,
production keys/provider resources, public-chain mining, transferable ownership,
recorded fight/XP/training award or official league activation is enabled by this
change. The existing candidate fixture and unrecorded labels remain explicit.
