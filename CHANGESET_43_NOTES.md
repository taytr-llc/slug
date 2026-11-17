# Change set 43 — reversible Saturday entry intake

Branch `slug-saturday-entry`, stacked from set 42 implementation
`81ef2816b42b9a10f0a37da90c5a967c7a93330b`. Implementation and source checks
are complete; final package acceptance and deployment remain pending.
Set 42's completed deployment/observation is recorded in its updated notes.

The candidate gym gains explicit Saturday selection, up to five rostered
fighters with one per class, save/replace/withdraw, history and durable pending
operation recovery. The same helper and UI retain mining, stable pages,
roster controls and unrecorded previews. Noon Pacific closes intake; event
locking, seeds, matching and official fight results are not implemented here.

`GYM_ENTRIES.md` describes the trust boundary, exact-view apply rules,
clock/cutoff semantics, bounded store, explicit parent schema 3 and separate
protocol family. D65, condition v1, set 42 engine behavior, chain consensus,
wallet derivation, production genesis and D72 restrictions remain unchanged.
No real keys, public-chain mining, new provider resources or public release
are authorized by this set.

The contract hash is
`958d3fda0cb50cda3bad38f7e461a44537a6a3c20ef5cc2b0c2abb7d2fdafcae`.
The original audited reference corpus hash is
`f3926a520771605aa5e071526d59bbdcc06a0027b16429e0f02a1e26c7c8af23`.
Its full 31-file tree is preserved in commit `b73c4da3` and the original
workspace archive. Comparison exposed two canonical-order expectation defects:
the ordered-whitespace EntryResponse cases retained reversed nested Job/View
field order. Their decoded values and acceptance were correct. The successor
changes only those two expected strings to match the frozen contract and the
original independently generated positive DTOs. It uses no Go output as
expected data. Its complete 33-file freeze and correction record are now under
`gymentry/testdata/reference`; corpus SHA256 is
`d8f226964429e8068bbe8300c73b6061f9a6dfd4e737a06ddde40c5eaa992bee`.
Separate author, independent and root reviews verify that exact delta.
A documented execution-order amendment
allowed independent reference generation and runtime authoring to overlap.
Runtime comparison began only after corpus freeze and independent approval.
No expected value was obtained from new Go output.

## Verification before packaging

- Core comparisons passed all 3,861 calendar cutoffs and boundaries, both
  namespaces, 449 core codec cases, 36 canonical core/disk DTOs and two receipt
  digests. The 54 reference state scenarios are not all executed by the Go
  bridge; independent transition/capacity tests cover runtime state behavior.
- Normal/race/tagged core checks, actual proof selection and held-account
  capability checks pass. Actual capacity tests fill 4,096 operations and 256
  dates. Core recovery includes eight process-kill cases.
- Installer tests actually kill the second fixture's process at prepared,
  created and ready stages, preserving the first gym. They check partial-state
  refusal, empty-directory resume, old-route refusal and missing-ready refusal.
  Normal/race runs passed in 40.651/253.604 seconds; owned processes were joined.
- Full desktop normal/race/tagged suites, vet and frontend checks passed
  (46.967/375.712/2.530 seconds for the Go suites). That aggregate capture is
  deliberately marked failed as a source-freeze gate: the integration harness
  and a narrowly reviewed helper fix changed while it ran. Final helper
  normal/race/tagged/vet checks pass on unchanged hashes; the exact final
  package run must bind the combined source.
- All 133 frontend DOM tests pass. Native correlation, helper and core reviews
  have no outstanding findings. All ten chain-module builds pass.
- The development helper completed the actual normal-PoW two-fixture flow in
  96.988 seconds, including save/replay/replace/withdraw, historical queries,
  pending recovery, reset-held edits and process restart. It built before the
  final helper fix; this is diagnostic evidence, not final-package acceptance.

Three development attempts are preserved. The first failed because the
harness placed its export destination inside the protected node directory;
the existing export guard correctly refused. The harness layout was corrected.
The second received an unknown Abandon acknowledgement; its exact cause is
unproven. The harness now queries the exact durable operation rather than
blindly resending the mutation. Separately, a deterministic regression exposed
a response-classification bug after successful Prepare/Apply: a subsequent
projection error could appear definitive. It now reports an unknown outcome,
with the existing fatal-stop handling preserved. An Abandon cancellation test
confirms unknown acknowledgement followed by durable abandoned state; no
Abandon runtime change was needed. The third development attempt passed.

Captured reports and logs are retained in the project workspace under
`set43-*.json` and `artifacts/set43/`. Package, smoke and deployment evidence
will be appended after those gates actually complete.

Actual visual rendering remains pending at Mike's request. The thorough joint
design review (D74) is still required before friends/tester/public release.
Engineering deployment alone does not close that gate.
