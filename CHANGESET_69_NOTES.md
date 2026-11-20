# Change set 69 — bounded restoration of existing backups

Status: implementation and candidate qualification complete. Paired commits and
remote topic verification are bound separately in `artifacts/set69/final-binding`.
No live-seed rollout or product-release approval is implied.

## Result

The Pool Pilot verifies its existing Phase-A version-1 backup through bounded,
resumable wallet recovery steps. The scratch wallet preserves the original
coin-0 derivation, mixed purpose-49 branches, root and gym identity. It uses
explicit schema 12; installed wallets and older schema/profile APIs are unchanged.
A restore never recreates the installed payment journal or its authorization.

The app continues to cancel, join and remove successful-close scratch work.
Explicit library Close/Open and process-restart resumption are separate from
app-level resume, which is not added. Exact private wallet tip hashes as well as
heights must match before verification succeeds. Uncertain closure preserves the
scratch data and retained owner; a later no-op Close cannot clear that failure.

`PHASE_A_BACKUP_RECOVERY.md` and `PHASE_A_RECOVERY_CLEANUP.md` were reviewed and
frozen before dependent implementation. The cleanup clarification applies the
same rule to ordinary ready-wallet Open/Unload. No new product or consensus
policy, custody authority or production identity is introduced. The frozen files
retain their original proposal headers; this change set adopts their reviewed
bytes, with the preimplementation freeze recorded separately.

## Independent inputs established before implementation

- The existing v1 Python reference reproduces its original standards/corpus
  before deriving 160 sparse public address/script cases, including mixed purpose-49
  branches and index 4095. New address corpus SHA256:
  `1558bf4a134ab01acfe84fdbb2edc3e8af842b602cb5150feaa9ce2df26b9cf7`.
- Seven bounded progress plans, eight discovery transitions and two invalid
  cursors model clipped 20-address lookahead, greedy ordered 128-attempt steps and the
  236-byte progress record. Derivation completion retains ready=false.
  Progress corpus SHA256:
  `fb0c9a5b1d59711ecb6d855534358f09175df5eb547c1bf88d0474b4b3515801`.
- Python 3.14/3.9 and portable repository-only reproduction match exactly.
  Producers, locked source inputs and outputs are retained in the wallet
  repository at `wallet/testdata/phase-a-recovery`.
- A separately compiled pre-Set 69 intake binary captured current exports from
  exact copies of the two Set 68 public fixture wallets. Originals remained
  byte-identical. Both observed native SegWit issued cursors are external 1/internal 1;
  other cursors remain 0. Private fixture exports are owner-only and never
  printed. This is old export evidence, not a new scanner result or live-chain
  observation. See `artifacts/set69/old-source-intake-attempt1`.

## Qualification

All required candidate-profile checks passed:

- The final immutable-scanner/cursor regression passes normal/race in
  10.326/47.871 seconds, with 993 local source inputs unchanged.
- All six wallet runtime modules build and pass vet, with 1,041 inputs unchanged.
  The final binding is `wallet-build-vet-attempt2`.
- The complete six-module wallet suite passes in `wallet-full-attempt3`:
  root 345.328 seconds; the five nested modules 1.951, 0.236, 0.716, 1.917 and
  3.732 seconds. Explicit cheap-profile refusals pass in 5.130 seconds.
  All 1,041 recorded inputs remain unchanged.
- The final Phase-A recovery and watch race group passes in 359.849 seconds,
  with 993 inputs unchanged. This includes all 32,768 actual derivations in 256
  chunks, forged invalid-child rejection, bounded watch dispatch, real process
  kills at initial/uncommitted/ready boundaries, joined cancellation, sticky
  cleanup and canonical/orphan completion auditing.
- Actual normal-PoW file-only recovery passes in 53.377 seconds. Both fixed
  preimplementation exports restore their full post-withdrawal balances, two
  UTXOs including change, three transaction IDs, current cursors and gym proof.
  Exact height/hash 110 is preserved through original-wallet and node reopen.
  Pending Close/Open retains the first 128 attempts. No new mining or payment
  occurs, original fixtures/exports remain byte-identical, and all owned
  processes and scratch directories close cleanly. See
  `artifacts/set69/actual-backup-attempt1`.
- The exact macOS package builds in 15.637 seconds with all paired source inputs
  unchanged: `artifacts/set69/package-attempt1/SLUG Pool Pilot.app`, version
  0.5.8/build 69. It is ad hoc signed, not notarized.
- All six complete desktop packages pass in 207.119 seconds with their source
  inputs unchanged. The exact packaged helper passes in 121.448 seconds across
  both fixture identities and name versions, initially and after reopening.
  This includes owner-only recovery export and file-only verification at genesis,
  original-view refusals, existing withdrawal/payout/league boundaries and clean
  process closure. Native selection is emulated with held, stat-pinned file
  descriptors; this is not a visible native-dialog or D74 design-review claim.

## Review findings and evidence limits

Review found and corrected a shortened pending lookahead target, incomplete
completed-session row auditing, and an acquired manager discarded after an
ordinary-open transaction commit failure. The commit-failure regression proves
refusal and later reopen; explicit manager closure is separately code-reviewed,
not directly observed through that test's public result.

The completed scanner is historical evidence, while the ordinary wallet may
later allocate addresses or change its live ledger. Its audit must not require
those two states to remain equal. Imported cursors and retained canonical owned
outputs establish a minimum completed address prefix; all stored rows still
need public derivation, contiguous coverage, bounds and count/crosslink checks.
An earlier orphaned payment can legitimately have extended that prefix. Without
a separate immutable completion cursor, coherent truncation above the provable
minimum cannot be distinguished from an earlier completion followed by later
allocation. No exact immutable-inventory or coherent-filesystem-rollback
integrity claim is made.

Focused wallet attempts 1 and 2 passed plain-Go diagnostic normal/race checks on
their recorded sources. Attempt 2 took 19.608/155.294 seconds. Root's provenance
review found that these and the first final-delta run did not use the mandatory
candidate no-CSE wrapper. They are retained as diagnostics, not candidate-profile
qualification; the correct-profile successor checks above supersede them. The
independent desktop
restore-owner checks passed normal/race in 1.58/13.28 seconds. All ten chain
module builds passed with 2,217 inputs unchanged. The required normal-PoW smoke
passed; its 1,770 compiled node inputs are identical to Set68, so this work
does not require a redundant live-seed rollout.

The first broad wallet attempt hit the sandbox's denied loopback listener in
an inherited RPC test. Its log, unchanged-source diagnosis and joined-process
cleanup are preserved. Its unchanged candidate-profile successor used authorized
loopback access and found one stale future-version test fixture: schema 12 was
the old unknown version and is now explicitly supported. Only that input changes
to 13; the expected ErrUpgrade and every assertion stay intact. The focused
regression passes in 2.116 seconds, followed by the complete final suites above.
Neither failed broad attempt is counted as a pass.

`test-fixture-closeout` preserves the exact one-line before/after bytes. This
test-only file is not compiled or embedded in the normal package. It is the
sole source exception after package qualification, alongside the two unembedded
closeout notes; final wallet qualification binds its new bytes. Production
sources, assets and executed package binaries remain unchanged.

Live exports already include current issued/change cursors. The separate hosted
vault still retains its original snapshot and needs its own integration work.
Permanent production identity, real custody and the final joint D74 review
remain release gates.
The next functional slice is bounded own-wallet activity in the existing wallet
section. App organization and navigation remain deferred to D74.
