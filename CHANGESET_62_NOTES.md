# Set62 — shared fixture wallet recovery

Branch: `slug-shared-pool-recovery`, stacked from Set61 `31878a68`.

## Change and reason

Connect the Pool Pilot's recovery controls to its actual shared-pool recipient
wallet. Native selections retain directory/file identities across the helper
handoff. Exclusive owner-only export rechecks the original recovery capability;
fresh verification restores from the selected file and compares synchronized
wallet state without replacing the original. Reset holds block export while
allowing verification. Cancellation joins cleanup and never restarts mining.
See `SHARED_POOL_RECOVERY.md`.

The Mac bundle is version 0.5.1/build62. Approved logos and layout remain in
place. Shared league attachment follows; the final joint D74 design review is
still required before friends/tester or public release. These are published
test wallets, not real account issuance or production custody.

## Verification

Evidence is retained under `../artifacts/set62` using the exact Go 1.27.4 no-CSE
candidate profile. Source and artifact bindings qualify finite macOS fixture
behavior; no production hosting, notarization or native chooser rendering is
claimed.

- `recoveryfile-attempt2`: all 18 normal and 18 race tests, vet, Linux arm64
  compile and Windows unsupported-stub compile passed. Runtime evidence is
  macOS only. Includes exclusive publication, original capability, symlink and
  parent substitution, uncertain outcomes and cleanup. Pre-final passing
  attempt1 is retained separately.
- `parent-tests-attempt2`: focused parent, protocol, bridge and native normal
  and race checks passed. Original pre-dialog view, reset while export is
  blocked, hold enforcement, strict nested selection grammar, uncertainty and
  replaced-view refusal are exercised. `parent-compile-attempt1` passed.
- `child-attempt2`: lifecycle/export normal and race checks passed.
  `child-comparison-attempt1` subsequently passed affected normal/race checks
  for pending-state retry and issued native84 metadata. Any unconfirmed balance,
  output or transaction prevents a false mismatch or false success.
- `genesis-regression-attempt1`: actual fresh-wallet synchronization, complete
  state equality and gym-signing checks passed normally and under race in 20.87
  and 98.41 seconds. All three fresh restores waited for true synchronization,
  including at height zero. Full comparison remains unchanged.
- `frontend-attempt4`: all 294 browser tests passed, including existing native
  controls, cancellation, hold behavior, retry guidance and uncertain-save copy.
- `actual-attempt2`: complete normal-PoW shared-runtime acceptance passed in
  275.50 test seconds (279.91 seconds including build). Each gym received 39.6
  test SLUG in the same confirmed batch. Export and fresh verification passed.
  An independent file-only restore recovered 3960000000 atoms and made a
  confirmed spend while the original database pathname was unavailable. Decoded
  transaction checks prove its inputs spend the restored payout outpoints,
  exactly 1 SLUG reaches the independently frozen sink script, remaining change
  belongs to an internal wallet derivation, and values conserve with a positive
  fee. Reset preserved the wallet/source, blocked held export, revoked the old
  view, and reopen retained exact payments/history. Explicit test descendants
  supplied maturity; setup, login, recovery and settlement never mined.
  The public result under that attempt's `pool-desktop-*` directory records
  original recipients, amounts and both transaction IDs. The earlier passing
  attempt1 predates decoded spend-semantic assertions and is retained separately.
- `package-attempt2`: normal node/helper/GUI built in 10.67 seconds with unchanged
  sources and strict ad hoc signatures. `packaged-helper-attempt2` passed in
  48.54 seconds: both wallets exported owner-only files, verified fresh restores,
  released native-side selection descriptors immediately after helper handoff,
  preserved source/ports/recipients across reopen and exited cleanly on EOF.
  Private recovery bytes never crossed the public pipe or entered its evidence.
- `desktop-full-attempt1`: all normal accountpilot, pilot, bridge, recoveryfile,
  helper and native packages passed in 158.604 seconds overall. Opt-in
  actual nodes were disabled in this run; all local Go/build/testdata and embedded
  frontend inputs were included in its unchanged-source guard.
- `static-attempt3`: repository-wide formatting, diff checks and affected-package
  vet passed. `smoke-attempt1`: two isolated normal-PoW blocks, zero mining peers,
  10-SLUG subsidy and report-only checkpoint conflict passed. Read-only seed sync
  reached the exact height-61 tip in 12.937 seconds; sixty-descendant
  checkpoint finality, next-height-62 activation, verifychain, stale-template
  refusal, restart and clean exit passed. No public block was mined.

## Preserved failures and fixes

Packaged-helper attempt1 exposed a real genesis-wallet race. A bounded
reproduction (`genesis-diagnosis-attempt1`) showed the original wallet was
synchronized while the fresh restore was still finishing its initial scan.
Accounts, outputs, history and signing already matched; only the readiness
flag differed. Removing the height-zero wait bypass fixes the mismatch without
weakening comparison. Package1 built successfully but its helper acceptance
failed; the corrected package2 and successor checks are the qualified artifacts.

Child attempt1 omitted a protected node-parent fixture directory and waited for
a hook that could never run. The corrected fixture and bounded early-error
wait passed without relaxing path protection. Parent attempt1 tried to forge
a managed-path request using Capture, which correctly refused it before parent
dispatch; the corrected test constructs the invalid request explicitly.

Frontend attempts1/2 used a fixtureB view with the wrong shared source pin;
the parser correctly rejected it. The independent shared-reference pin fixes
the fixture. Actual compile attempt1 had an unused import; attempt2 compiled.
Static attempt1 used an invalid SSA phase name; successors read the exact
committed profile. All failed reports are retained.

## Remaining work and deployment impact

Connect shared-pool fighters to Saturday entries and durable noon snapshots,
then the shared publication/card/result/career pipeline. Never repin solo
histories or spend share-accounted pool rewards on event carriers. Real provider,
custody, ownership/transfers/marketplace, operational capacity, release signing
and permanent-chain approval remain open. D74 remains the final joint review.

No compiled seed-node/control input changed across 1770 dependency files
(`node-input-assessment.json`), and no deployment configuration changed. No
redundant Railway rollout was performed; master and the live seed remain the
Set50 release `592e143c`. Final commit, source, evidence and bundle hashes are
recorded in `final-binding/commit.json` after commit.
