# Change set 55 — daily pool settlement

Branch: `slug-pool-settlement`, stacked from completed Set 54 `09aab718`.

## What changed

Mike completed D58: carry exact fractions, pay at 1 SLUG, batch the daily
payments, pay network fees from the existing pool 1%, and defer visibly if that
portion cannot cover the fee. The spec and `POOL_ACCOUNTING_POLICY.md` record
this as completion of D58/D36/D79, not another invented payout decision.

`poolround.SettlementSnapshot` exports authenticated, owned records from the
concrete source. The new `poolsettle` journal derives exact 99% miner and 1%
operator claims once per confirmed round. A trusted funding adapter checks
the original canonical coinbases, maturity and exact unspent outputs. Prior
payout change retains original round hashes/heights and remaining claims.

One daily batch pays the whole-atom part of eligible funded balances, retains
fractions and smaller balances, and charges network fees only to operator
claims. Before the first broadcast, a synchronous transaction persists the
exact signed bytes, input reservations, daily slot, payments and change.
Every signature, input, output and fee is checked. Restart/retry recovers or
rebroadcasts only that transaction. Unknown delivery never frees reservations.

Pending payments remain reserved; a current canonical confirmation marks them
paid. Every historical payout is observed before another batch. Funding loss
or conflicting payout evidence blocks further settlement while retaining the
locked history. The authenticated `MinerBalance` view includes the latest
attempt timestamp and explicit deferral reason, without another gym's data.

Review fixes preserve pending recovery at capacity, reserve space for expanded
fractional balance reports, validate persisted display amounts, bind the daily
slot at durable preparation across midnight, and verify original-round
ancestry even when spending previously confirmed change.

## Deliberately unchanged

No consensus, genesis, checkpoints, existing journal schema, public seed,
desktop navigation or product-release approval changes. Native P2WPKH is the
bounded first settlement profile; limits and destination mappings are pinned.
The `poolsettlefixture` adapter uses published scalar 3 on an explicit loopback
test chain, with a fixed 1,000-atom test fee. It is not production custody or a
production fee estimator. No real wallet or operator key was read or created.

## Validation

Checks use the pinned Go 1.27.4 no-CSE candidate profile. Evidence is outside the
repository under `../artifacts/set55/`, with failed attempts preserved.

- `store-attempt1` and `store-attempt2`: focused normal-PoW concrete-source
  tests pass. They cover durable-before-broadcast ordering, exact fee/carry,
  maturity/fee/conflict deferrals, session revocation, corrupt balance reports,
  signer mutation, midnight/day replay, capacity recovery and uncertain writes.
- `model-attempt1` preserves a nil-pointer error in the test coinbase helper.
  `model-attempt2` passes after using an explicit zero outpoint. Independently
  calculated fractional expectations were unchanged. Later model guards are
  included in the final package checks.
- `fixture-ancestry-attempt1`: controlled-node cases pass for original-round
  canonicality, orphaning, unavailable RPC evidence and malformed membership.
- `actual-attempt1`: an isolated two-node normal-PoW chain demonstrated an
  actual accepted pool winner, sixty-descendant confirmation, deferral at
  sixty and ninety-eight descendants, and payout readiness at ninety-nine
  descendants for inclusion at consensus maturity. Two gyms each received
  4.95 SLUG in one transaction; its 1,000-atom network fee came from the pool's
  1%. A deliberately lost acknowledgment, journal reopen and repeated retries
  made only one broadcast. Node logs and exact test-source bindings are saved.
- `validation-attempt1`: all three changed packages pass normal tests (15.719s),
  race checks (87.551s), cheap-profile refusal checks (4.134s), and vet (0.335s).
  The seed dependency check confirms none of `poolround`, `poolsettle` or
  `poolsettlefixture` is linked into the daemon. Formatting and diff checks
  are clean. No seed rollout or desktop rebuild is required for this component.
- `actual-attempt2`: the final reviewed sources repeat the entire actual-chain
  milestone successfully in 68.839s. The single payout transaction is
  `5664b9b8a6169a22a13ccbd1b08014965cce8fbb77f6a95bfe428293151ae131`.
  It pays two 495,000,000-atom outputs, retains 9,999,000 atoms of pool change
  and pays the 1,000-atom network fee from the original 1,000,000,000-atom
  reward. Lost-ack restart and repeated retries still invoke broadcast once.
  Both owned nodes shut down cleanly. This is private normal-PoW evidence,
  not a public-chain payment or product launch.

## Remaining work

Connect the pool protocol and desktop miner to these components, expose the
authenticated balance/report route, add the daily service scheduler and
authoritative recipient enrollment, and integrate reviewed operator-owned
signing/custody with a real fee estimator. Define operational resolution for
post-confirmation funding conflicts without erasing locked earnings or making
reserve advances. The full joint D74 design review remains last before release.
