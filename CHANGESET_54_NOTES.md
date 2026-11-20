# Change set 54 — fixed pool rounds

Branch `slug-pool-rounds` stacks on Set 53 `298cb1d7`. D81 and the implementation
contract were recorded at `e3487b47`. App organization remains deferred to D74.

`poolshare.AccountingPage` exposes bounded operator-only reads of independently
reverified accepted work. A fixed acceptance tail and immutable projection
exclude pending block intents and changing delivery status. This adds no miner
endpoint and changes no source journal schema.

`poolround` creates a separate durable journal of fixed per-block windows,
including each accepted winner. With the source's single pinned difficulty,
allocations use exact share-count ratios and reduced fractional atomic amounts.
There is no rounding or fee deduction. Prefix binding protects consumed source
history, and an orphan or return cannot move shares into another round.

`prospectnode.ObservePoolBlock` supplies fresh bracketed canonical membership
and descendant depth. The ledger locks the first canonical observation at sixty
descendants. Before that, orphaned rounds have no earnings; exact returns resume
their original allocation. After confirmation, a conflict retains locked gross
earnings and reports missing canonical funding. RPC errors preserve prior
evidence without fabricating an orphan or confirmation. Unknown writes require
reopen; reserved status capacity permits recovery when new-round capacity is full.

The independent Python Fraction corpus reproduced byte-for-byte. Focused normal
tests, race tests, cheap-PoW refusal and vet passed under the recorded Go 1.27.4
no-CSE candidate profile. Evidence is in
`../artifacts/set54/source-attempt1/report.json` and
`../artifacts/set54/ledger-attempt1/report.json`. Source/observer/ledger review
found no concrete defects. Repository formatting is clean.

Tests cover independent two-gym fractional allocations, fixed windows and late
append behavior; concrete accepted-source replay; orphan/return and the 59/60
boundary; post-lock conflict; restart and uncertain-write recovery; source/config
corruption; capacity; cancellation and draining. Controlled RPC responses exercise
the real client. They do not claim a freshly mined live 60-block chain.

No changed package is linked by the seed daemon, and deployment configuration
is unchanged, so no seed rollout is needed. There is no desktop route change or
rebuild. Maturity/unspent funding checks, daily settlement, fee/rounding policy,
public pool transport and miner integration remain ahead. No money moves.
