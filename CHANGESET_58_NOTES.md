# Set 58 — actual wallet recipients for pool payments

Branch: `slug-pool-gym-binding`, stacked from Set 57 `de9e02de`.

## What changed

Added a separate native-SegWit recipient helper for the two published recovery
wallet fixtures. Each uses the independently frozen `m/84'/0'/0'/0/0` address
and script. These payment keys are distinct from the gym-signing key; a pool
must not substitute that key when selecting a wallet receive address.

Allocation uses the existing allowed wallet RPC. Ownership, recovery profile,
public cursor, derivation path and script are verified against the concrete
wallet. A lost response after dispatch poisons allocation on that runtime;
close/reopen and reattachment recover the original index0 address. Established
recipients use verify-only behavior and cannot allocate a missing replacement.

An outer public binding records immutable account, fixture and complete pool
source intent before wallet I/O. Separate READY publication waits for wallet
synchronization and original-token/view authorization. Wallet I/O holds no
account lease. Missing established intent, conflicting source/account identity,
symlinks and noncanonical files fail closed. Review identified and fixed one
durability edge: matching existing markers now repeat directory synchronization
before recovery accepts them after an uncertain publication acknowledgment.

See `POOL_RECIPIENTS.md` for the exact public destinations and lifecycle.

## Deliberate non-changes

The old scope44 payout, existing wallet markers, address cursors and desktop
route remain intact. No slugwallet source or RPC protocol changed. Only
published fixture material is used; no real seed, key, production derivation,
genesis or custody service is created or read. D58/D79 accounting rules stay
unchanged. App organization and the joint D74 design review remain deferred
to the final review session with Mike.

## Verification

Artifacts are under `../artifacts/set58/`, using the pinned Go 1.27.4 no-CSE
candidate profile. Independent expectations come from the unchanged recovery
corpus with SHA256
`d5141f4a8ee4378d77d483b09f6ecd809f8b328f82e79a69ff83164e73bc799a`.

- Initial helper tests passed (`wallet-attempt1`, 13.039 seconds package time):
  both actual wallets, allowed legacy HTTP, old payout preservation, exported
  cursor restoration and a deliberately dropped committed allocation response.
  The same runtime refused retry; reopening recovered index0 with cursor1 and
  zero further allocations. Its notification-only fixture made no chain claim.
- Binding compile and focused tests passed (`binding-attempt1`, 1.172 seconds):
  durable intent ordering, synchronization, original view/token, uncertain
  intent/wallet/READY recovery, account/source mismatch, missing established
  evidence, callback guards, canonical decoding and symlinks.
- Actual normal-chain composition passed on its first attempt (67.53 seconds
  test time, 72.867 seconds including the wrapper). Both actual wallets enrolled
  through account signatures and installed their immutable recipient bindings.
  Two accepted shares closed one pool round. At 60 descendants the round
  confirmed and settlement deferred; at 99 descendants the coinbase was
  spendable in the next block and one transaction paid both wallets.
  At the next block, each wallet reported exactly 495,000,000 atoms spendable,
  one UTXO at its bound address/script with one confirmation, and one matching
  receive-history entry. There was no immature or unconfirmed wallet amount.
  Same-day retry retained transaction
  `206516b98364dd5b4e68e77eef7f3ffef12cb0e7289485edef82d9358fa806dd`
  with one backend broadcast. Both owned nodes shut down cleanly.
- Final scoped checks passed on the first attempt: normal 16.006 seconds,
  race 115.560 seconds, cheap-profile checks 6.397 seconds and vet 0.725 seconds.
  Root daemon dependency checking confirmed no desktop package is linked.
  Full-repository gofmt and `git diff --check` are clean. No failed test attempt
  occurred in this set; the marker durability improvement came from review
  before final qualification.

## Remaining integration

The current solo stable/projection assumes all source records belong to one
gym and that assignment sequences are contiguous. A shared pool issuer needs
an explicit scoped mode preserving global source sequences, with a separate
local assignment count and audited foreign skips. The next set addresses that
boundary before the parent pool runtime and balance view are enabled.

This set changes desktop fixture wiring only. No live seed configuration or
production distribution changed; no seed rollout is needed. Master and the
live seed remain Set 50 `592e143c`. The existing qualified bundle remains Set 51
until the new shared runtime is connected and packaged. No new user policy
decision was needed.
