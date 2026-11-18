# Change set 48 — candidate result publication

Completed on `slug-result-publication`, stacked from completed Set47.
The independent reference was committed and pushed at `17e6045d` before runtime
implementation. Implementation `de4c1497` passed the candidate application and
package gates and was released on master at `cc4f41e4`. The completed engineering
release and its remaining product limitations are recorded below.

## Behavior

An explicit action publishes the exact result of an already saved candidate
bout. Its statement, transaction and original source remain immutable. A
separate result checkpoint and sixty descendants establish observed result
finality. Before that checkpoint, the same transaction can be included again
after a reorganization. After it, a crossing reorganization retains the first
inclusion and certificate and reports a conflict.

The desktop can connect its owned test nodes for publication without opening
a wallet or starting mining. Passive display polling does not connect them.
Original account sessions and source journals authorize each protected action;
held sessions cannot publish or finalize. Public proof export uses a native
destination and retains complete public bout, card and chain evidence.
See `EVENT_RESULTS.md` for the storage, authority and proof limits.

## Deliberate non-changes

No professional or exhibition record, XP, training, title, ranking or prize is
awarded. D65, C1 matchmaking, original fighter births, the noon snapshot and the
fixed event seed remain unchanged. Checkpoints do not alter chain selection.
Seed and result signing domains and funding scripts remain separate. The new
publication uses published fixture keys only; production authority and permanent
genesis are outside this set.

The approved detailed app icon and simplified menu icon remain intact. D74's
thorough joint design review is still required before friends/testers or public
release. Portrait rendering and native chooser/GUI/accessibility acceptance
remain separate gates.

## Independent reference

The reference freeze is
`7d597804a76d6e5c4490c0455f1e7bb571454f77e5cb619580135b51698b0d93`;
the corpus is
`07c00eef8d87ae9ae5dcd2d120a5dfd1f7b1ba33a1447b773cfd4d5194fd1a94`.
Python 3.9 and 3.14 reproduced the exact bytes. Frozen inputs include the
genuine Set47 saved bout and its independently verified source chain. Additional
funding/result blocks are explicitly unmined structural models, not evidence
of new normal proof of work.

The finite cases cover core and wire codecs, signatures and transactions,
state transitions, journal graphs, initialization and public proof structure.
Runtime tests compare against these independently derived values. Earlier
failed reference candidates remain preserved.

## Local validation and preserved diagnostics

Core, helper and native reference comparisons have passed during integration.
The adapter's earlier normal, race, cheap-profile refusal and vet checks passed.
All 227 frontend tests pass following review fixes to uncertain-request
correlation and refusal handling.

Three actual installer SIGKILL/resume stages passed, preserving wallet and
league journals. Full-capacity checks with 256 saved bouts and 256 result rows
measured selected metadata at 223–242 ms against the 500 ms budget; full parent
startup took 10.773 seconds against 25 seconds. These rows include explicitly
modeled capacity fixtures. An earlier 500.208 ms timeout remains recorded as a
failed attempt; a subsequent quiet diagnostic and the capacity check do not
erase it.

Those earlier Go checks used the default compiler and do not qualify the new
build profile. The original reservation regression subsequently exposed a
compiler lifetime defect. `BUILD_TOOLCHAIN.md` records the preserved failures,
the rejected all-optimization-disabled attempt and the exact temporary Go
1.27.4 profile now being checked. Core normal, scoped-clock, full race, original
reservation and GC-pressure checks passed. Vet found three unkeyed aliased
struct literals; naming the original fields preserved every expression and its
evaluation order. Independent review, focused final-source normal/race checks,
both vet profiles and formatting pass after that correction. The original
aggregate still records its vet failure. Instrumented result actions add zero
fight-engine entries. The same profile now passes the desktop normal/race and
vet checks, adapter normal/race/refusal checks, all ten module builds and focused
conformance tests. Its actual installer passes all three SIGKILL stages in both
normal and race modes. Full-capacity selected metadata took 228–242 ms; parent
startup and first response took 11.16 seconds. Original failed vet and
test-runner working-directory attempts remain retained. Assertions and runtime
limits remain unchanged; these local checks do not establish a compiler fix or
Linux container acceptance.

The actual-chain harness and offline verifier are prepared. The harness uses
a cold copy of Set47, preserves both older conflicted cards and all original
births, and creates a later candidate source with no additional admissions.
It checks publication, restart, same-transaction re-inclusion, depths
59/60/61, checkpoint conflict, held/other-gym access and six proof exports.
The first attempt reached initial inclusion/export and retained the orphan
correctly, then failed to acquire a replacement mining template after forced
block invalidation. The original failure and clean provenance/cleanup audits
remain preserved. Source and log review found stale in-memory best-header and
peer-height observations after that test-only rollback. The harness now verifies
exact durable records across an owned restart before reconnecting at both forced
rollback boundaries. This tests restart recovery; it does not establish
uninterrupted mining after forced invalidation. Node readiness and consensus
checks were not relaxed.

The fresh actual run passed in 286.793 seconds, with graceful cleanup and exact
original-tree/source preservation. All six exports passed independent public
verification: 279 unique normal-PoW headers, including 178 already checked in
the source-chain pass and 101 additional headers. The verifier accepted one
positive control and rejected twelve semantic mutations without repeating work.
No additional fighters were admitted. The unchanged fight engine produced the
same saved result independently; D65 is unchanged and publication awards no
records or XP.

The exact temporary profile is locally qualified for this candidate application
scope. Its initial qualification recorded two nonblocking test-assertion gaps:
require the actual SIGKILL termination signal in the core crash test, and check
temporary-file cleanup in the renamed original export directory. Source review
found no corresponding runtime defect. Existing failures remain preserved.

After deployment closeout, both assertions were tightened. The crash test now
requires an actual SIGKILL termination and joins its child before checking the
status. The export test checks both the renamed original directory and its
replacement, including directory-read errors. Both affected suites passed in
normal and race modes with no forced cleanup or surviving process group.
This follow-up changes tests and documentation only; the deployed implementation
and packaged binaries remain the exact `de4c1497` build. The original findings
and qualification reports are preserved in the workspace.

## Engineering release acceptance

The exact implementation's normal-clock Mac package passed in 167.93 seconds.
Eight independent reports verified the existing proof, stable, preview and league
scopes, including six Result metadata/refusal phases and process restart. Source,
binary, profile and icon bindings remained exact. This normal-calendar package
run made no successful closed-event result publication or export; the separate
scoped-clock actual run above supplies that coverage.

Railway deployment `66e97bfb-0587-430a-b5cd-60e7ed946d24` released
`cc4f41e4176aafd593836ee4ff853775f2a87b66` with status SUCCESS. The final provider
closeout confirmed one healthy replica and unchanged service configuration,
volume and TCP settings. The isolated report-only checkpoint observer passed
900.0553332919953 seconds with 31 samples at height 61,
sixty descendants and one peer per sample; restart and owned shutdown passed.
Workspace `set48-closeout.json` (SHA-256 `58ee3253b65b7c5da34321c4e1a481126d5421842421f0f541a1ae040f46c53f`) binds the final
observer, provider and package evidence. Exact implementation and release CI
queries returned no workflow or check runs; no hosted CI pass is inferred.

Railway still runs only the Phase-A node, not a production league service. Mac
cross-build evidence and the provider's container build/deployment evidence remain
separate; no local Docker build is claimed.

Supplementary provider evidence in workspace
`set48-container-build-acceptance.json` records 82 build-log entries confirming
completed Linux/amd64 build-stage tests, shipping builds and the wrapper's
embedded-profile verification. This is provider-log evidence only. Direct CLI
readback failed as unauthorized using both the inherited token and the existing
login. Raw build metadata and built/runtime hash files were not retrieved or
independently compared; the failed readbacks remain preserved.

This candidate engineering release
does not certify universal compiler correctness, production authority, permanent
genesis, visual UI, portrait or native chooser acceptance, accessibility or D74.
The original test-assertion findings, their verified follow-up and the
forced-invalidation restart scope remain recorded; no professional records,
XP or awards are granted.
