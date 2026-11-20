# Daily pool settlement

Set 55 implements D58's completion through a separate operator-owned
`poolsettle` journal. It consumes a concrete `poolround.Store`; arbitrary
receipts, JSON allocations, wallet totals and caller-selected payout addresses
cannot authorize a credit. The source identity, native SegWit pool script,
gym destination mappings and capacity limits are pinned at creation.

Every sixty-descendant confirmed round creates exact reduced rational claims:
99% for miners, 1% for the operator. Fractions use atomic units throughout.
Neither carry-forward nor retry deducts that fee again. Fresh canonical UTXO
checks decide which original coinbases or retained change can fund a batch.
An immature or conflicted round stays in the confirmed balance but cannot
advance another miner's payment.

The operator invokes `RunDaily`. Its internal clock assigns a UTC calendar
date solely for daily idempotency; share accounting remains per block.
Deferred attempts may retry that day. Once a signed batch is prepared, its day
is permanently occupied. No catch-up transaction is created for missed days.
A monotonic persisted clock high-water rejects rollback. A future service
scheduler will choose the daily run time; this library does not start one.

`RecoverPending` is a separate operator recovery pass. It uses the same exact
retained-transaction observation/rebroadcast path as `RunDaily`, then stops
before new credit import, payment planning, fee quotation or signing. It never
occupies a new daily slot. This lets a future scheduler recover old delivery
before its configured payment time without accidentally preparing today's
batch. Existing `RunDaily` callers retain their previous behavior.

Recovery's pending/deferred reports retain their existing meanings. A recovery
report with `state: complete` and an empty reason means the saved-transaction
scan finished, not that today's payment ran. Its day and timestamp date that
scan; only a persisted batch's day occupies the daily slot. Unreserved funding
was not inspected, so available is conservatively zero and recovery does not
invent a below-minimum reason. Confirmed, reserved and paid ledger amounts
remain exact; a known missing destination remains visible. A future scheduler
must display its own next due time separately from this recovery report.

One batch combines ready inputs, pays each configured gym the floor of its
aggregate funded net balance if at least 1 SLUG, and carries all remaining
claims and unused fee budget in one change output. Original round attribution
survives in change. The quote must fit the exact retained operator fee balance.
Insufficient fee budget, missing recipient mapping, immature/conflicted funds
and a nonzero change below the supported witness dust floor defer explicitly.
There is no fee-only transaction when nobody reaches the minimum.

Before broadcasting, one synchronous LevelDB write saves the exact signed
transaction, UTC day, all input reservations, payments, fee and residual claims.
The signer cannot change inputs, outputs, amounts, locktime, sequences or
sighash mode; the journal independently verifies every witness. A failed write
poisons the handle and requires reopening. Delivery uncertainty never frees a
reservation. Recovery first looks up the exact transaction; if absent and its
inputs remain ready, only those original bytes may be retransmitted.

Pending/mempool payments appear as reserved. One current canonical confirmation
marks a payout paid and allows its change to be considered for a later day.
This is an engineering condition for change reuse, not a new finality claim.
Every prior payout is re-observed before another batch. A later disappearance
of a previously confirmed payout blocks further settlement as a conflict;
paid history and locked earnings are retained for incident handling.

`View` is an operator report. `MinerBalance` selects only the gym derived from
the original live session and rechecks it after the read. Both include explicit
deferral reasons and the latest attempt timestamp. Reports are historical
observations, never spend authority. No public balance transport or desktop
navigation redesign is implied by this API.

The first adapter, `poolsettlefixture`, only signs with the explicitly published
fixture scalar 3 and accepts a loopback Phase-A node. It verifies chain identity,
canonical membership, exact outputs, mempool spends and next-block maturity.
Its 1,000-atom fee is a private test quote, not a production fee estimator.
Production signer custody, recipient enrollment, deployment and UI integration
remain separate work. No real key is generated or read by this milestone.

The bounded first implementation supports native P2WPKH inputs/change/payees,
256 rounds/distinct gyms/destinations/inputs and up to 366 retained daily batches. Limits are
explicit and immutable; there is no silent migration, eviction or rollover.
No consensus, existing source journal schema, public seed or genesis changes.
