# Set 61 — runnable shared fixture pool desktop

Branch: `slug-shared-pool-desktop`, stacked from Set 60 `f21fe5d7`.

## Change and reason

Connect the verified pool, wallet, miner and shared stable components through
one runnable Go desktop route. A parent keeps the pool and chain alive across
account switches; each original account view owns its wallet, worker, outbox
and local fighter inventory. Both actual fixture recipients verify before
settlement starts. Authenticated balances expose exact carry, observation time
and payout deferrals. See `SHARED_POOL_DESKTOP.md`.

The separate `SLUG Pool Pilot` bundle uses `LocalPoolPilot`, version 0.5.0/build61.
The solo Candidate Pilot retains its routes and histories. Consensus, genesis,
checkpoint doctrine, payout policy and approved logos remain unchanged. Shared
wallet recovery follows in Set62, then explicit shared league attachment. The
joint D74 design review remains last, before friends/testers or public release.

## Verification

Evidence is retained under `../artifacts/set61`, with the exact Go 1.27.4 no-CSE
candidate profile and source/artifact bindings. These are finite Mac candidate
checks, not production hosting, notarization or universal platform acceptance.

- `frontend-attempt1`: all 283 browser transport/DOM tests passed, including
  sparse Pin2 inventory, local Recent/Previous, scope rejection, exact fractions,
  visible deferrals and failed-balance blanking.
- `desktop-full-attempt1`: complete normal accountpilot, pilot, bridge, helper
  and native packages passed in 148.159 seconds overall. Startup cancellation
  race checks passed in 10.180 seconds, including Quit before the first request
  and during the initial snapshot. Only the separately tagged actual harness
  was excluded from this run's source guard.
- `child-attempt4`: focused normal/race checks passed, including joined
  Stop/restart/Close, original-view rejection, partial installation recovery,
  and clearing an expired balance request's old payload. Failure-only publication
  uses a fresh bounded child context and the same original view; successful
  responses retain their request context. The HTTP deadline remains ten seconds.
- `runtime-final-attempt1`: ten normal and ten race tests plus cheap-profile
  refusal passed. Its conservative whole-tree guard noticed an unrelated change
  in the excluded accountpilot integration test. The original failed wrapper
  report is preserved; `scope-assessment.json` verifies unchanged compiled inputs.
  `runtime-actual-attempt2` independently passed partial provisioning, exact
  wallet reopen, pinned-port refusal/recovery and clean shutdown in 14.72 seconds.
- `actual-attempt5`: the complete normal-PoW two-wallet app acceptance passed
  in 212.71 seconds. Both workers mined; sixteen fighter receipts remained scoped
  to their original gyms. The scheduler paid 29.7 and 49.5 test SLUG in one batch,
  transaction `eb3bd019318cb8ce5635794caae4b970fbb0480f905ea61e1033289a9ab3ed26`,
  included in canonical block
  `484db6919991b669066251626d502fb2b304559429e648da1db993c812221795`.
  Reset preserved recipient/source and applied the hold; reopen preserved payment
  state and fighter history. Explicit test-only descendants provided maturity;
  provisioning/login/settlement did not mine. This test checks wallet coverage
  and retained payment state, not an injected lost-payment response or an exact
  per-output UTXO oracle; Set55/56/58/60 provide the separate component evidence.
- `package-attempt2`: the normal node, helper and native GUI built in 14.91
  seconds with unchanged source; all binary profiles and strict ad hoc bundle
  signatures passed. `packaged-helper-attempt2` exercised both gyms through the
  exact bundled binaries, Pin2 inventory, authenticated zero-work balances,
  previous-view refusal, fixed source/ports/recipients and clean EOF/reopen in
  32.09 seconds. No forced process termination was needed.
- `native-attempt1/native-observation.json`: the exact bundle's GUI showed the
  approved logo, both correct fixture wallets, timestamped pool balances,
  sign-out blanking and disabled recovery controls. Login stayed stopped at
  height zero. Menu-bar Quit ended the GUI and its complete process group.
  The first CUA call stalled overnight; its bounded launch had already ended,
  so CUA relaunched the same bundle into the default fixture namespace. Preserve
  this distinction. The original timed runner is not native-Quit evidence.
  Cmd+Q had no visible effect; menu Quit worked. This was a functional check,
  not D74, a menu-icon contrast review or native funded-mining acceptance.
- `static-attempt2`: repository-wide formatting, diff checks and affected-package
  vet passed. `smoke-attempt1`: fresh normal node mined two isolated blocks with
  zero peers and 10-SLUG subsidy, accepted a report-only checkpoint conflict,
  then synchronized read-only to the exact seed height61 in 12.505 seconds.
  Checkpoint sixty-descendant finality, next-height62 activation, verifychain,
  stale-template refusal and exact restart passed; clean exit, no public mining.

## Preserved failures and fixes

Earlier child tests omitted a required enrollment; parent/native path fixtures
used `/var` aliases instead of canonical `/private` paths. Corrected fixtures
passed without relaxing production checks. Review fixed nested Start leases,
unpinned issuer adoption, recreation of missing established chain data and
publishing readiness before successful wallet shutdown. Actual runtime recovery
found `wallet.db` incorrectly used in place of the wallet package's
`slugwallet.db` constant; the failed attempt and successful retry are retained.

Actual launcher1 selected a missing node path and started no test. Attempt2
created correct two-recipient outputs but mined without requiring payout
broadcast or template inclusion; the payment remained unconfirmed. The exact
ordering cause was not established by that run. Attempt3 cancelled the context
retained by an asynchronous Recent job; the harness now retains it through
completion. Attempt4 proved that an observed mempool payment can still be absent
from a cached GBT template. The harness now requires the exact transaction in
the template, decoded candidate and canonical submitted block. These failures
remain failures; none changes production timing, settlement or mining policy.

Package1 failed at iconutil inside the sandbox; the same source packaged
successfully outside it. Its exact failure mechanism remains unestablished.
Packaged-helper1's preflight wrongly expected ldflags metadata omitted by
trimpath; the corrected checker verifies the exact GUI's advertised defaults.
Static1's wrapper selected an unsupported helper verb and ran no checks.

## Remaining work and deployment impact

Connect native shared-wallet export and file-only fresh restoration with the
original recovery capability and D72 hold, then shared league/career services
without repinning old histories. Production provider/custody, capacity,
operations, ownership/transfers/marketplace, release signing and permanent-chain
approval remain open. Preserve the final joint D74 gate and current layout.

No compiled seed-node/ctl dependency or deployment configuration changed
(`node-input-assessment.json`). No redundant seed rollout was performed; master
remains the Set50 release `592e143c`. Final source, evidence and bundle hashes
are recorded separately in `final-binding/commit.json` after commit.
