# Shared fixture pool desktop

Set 61 adds an explicit `--candidate-pool` route and a separate **SLUG Pool
Pilot** macOS bundle. Its data root is `LocalPoolPilot`; the qualified solo
Candidate Pilot and its histories retain their existing route and namespace.
This route uses the two published recovery fixtures on an owned normal-PoW
Phase-A loopback chain. It does not enable real registration or a public pool.

## Runtime ownership

The account parent owns one account authority and one shared runtime. That
runtime owns the primary/peer nodes, concrete manifest issuer, accepted shares,
rounds, settlement, pool HTTP service and fenced admission-only service. The
primary explicitly enables transaction indexing. Listener ports and the full
pool source are durably pinned before creating any worker outbox. A conflict
at a pinned port fails startup instead of changing transport identity.

Setup temporarily authenticates and verifies both actual fixture wallets,
enrolls their existing gym keys and verifies synchronized native84/index0
recipients. These setup sessions are revoked before the interface signs in.
Only then can the immutable settlement recipient table become ready. Partial
wallet provisioning resumes the same identity; an unpinned orphan issuer or a
missing established chain/store fails closed. Wallet shutdown must succeed
before a fixture is marked verified for the current runtime.

A signed-in gym borrows the shared nodes and issuer. Its child owns its actual
wallet handle, separate durable pool outbox, PoolWorker, Pin2 stable and cached
presentation. Original token/generation/view checks bracket slow wallet and
network operations. Final publication uses short leases; Start is never nested
inside a parent account lease. Only explicit Start mines. Login, provisioning,
status refresh, synchronization and settlement never mine.

Logout, expiry and reset invalidate the old view and join that child. They do
not replace or close the shared source. Quit cancels and joins children, the
scheduler and listeners before closing journals and owned nodes. Startup also
responds to Quit while the first helper response is pending.

## Earnings and fighter views

One bounded maintenance loop synchronizes accepted work, refreshes round
confirmation and calls the existing daily settlement operation. Daily retries
retain the same prepared transaction. D58/D79/D80/D81, the 60-descendant earning
lock, 100-block coinbase maturity, 1% fee and 1-SLUG threshold are unchanged.
Network fees use the operator share; the miner sees the recorded deferral
reason rather than an added charge.

`PoolSnapshot` is separate from the old manifest projection. It carries the
current gym's local assignment count and a source-bound balance response with
its original observation time. Snapshot reads copy a cache; they do not call
wallet/node APIs or mint credentials. Failed balance reads display unavailable,
not a fresh zero. Failure-only cache publication uses a fresh bounded child
context and the same original view even after the request deadline expires;
no cancelled response payload is disclosed. Busy contention retains the original
observation timestamp for a later retry. Amounts retain exact rational atoms; display truncates only
to whole atoms and explicitly labels any retained fractional atom.

Shared stable pages retain their original global source sequences. The browser
requires Pin2 in pool mode and Pin1 in the old route. Recent reads select the
local tail, inventory cursors can cross foreign receipts, and Previous uses
observed page boundaries rather than subtracting eight global sequences.
Portrait and public-receipt operations remain bound to the original gym view.

## Current integration limits

Shared-pool fighters support inventory and reversible roster selection. Set62
adds recovery export and fresh backup verification as described in
`SHARED_POOL_RECOVERY.md`. Shared fight preview/event/career attachment remains
subsequent functional work. This route never delegates those operations to incompatible
solo stores or relabels old histories. Their existing solo routes remain intact.
Public receipts are not wallet recovery exports.

The unchanged layout and approved logos are reused. This engineering integration
is not the joint D74 design review or approval to invite friends. Real provider
and custody configuration, permanent-chain identity, production capacity and
service operations, signing/notarization and release acceptance remain on the
burn list. No node consensus or seed-deployment input changes in this set.
