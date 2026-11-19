# Fixed pool rounds — Set 54

This component implements D79/D81 round allocation and confirmation on the
Set 53 accepted-share journal. It does not round amounts, deduct fees, move
coins or declare a balance payable. The public pool and desktop integration
remain separate work.

## Source and membership

The concrete pool supplies bounded operator-only `AccountingPage` reads. They
reverify accepted work and expose immutable acceptance fields, original jobs
and exact payout-bound winner proofs. Mutable delivery observations are excluded.
These reads are not miner or gym endpoints and must not be exposed as such.

Process `AcceptedSequence` in order. Every accepted block-winning share closes
its own round, from the sequence after the previous accepted winner through
itself. An unaccepted block intent closes nothing. A later canonical-only
acceptance appends a new winning share; it never inserts a boundary into an
existing round. Original submission order and block height do not replace this
acceptance order. A timely accepted winner can have a fixed round while its
canonical membership is still unresolved; that fact grants no confirmed reward.

For example, winners at acceptance 5 and 8 permanently fix intervals 1–5 and
6–8. Changes to either block's canonical status never move those shares. If
the exact first block returns after an orphan, its original interval resumes.

The source pins one share target. All shares therefore have the same assigned
difficulty, and difficulty-weighted allocation reduces to each gym's count
divided by the round's total count. Retain that target explicitly. Gross
allocations are reduced exact rational amounts in the coin's smallest units:
`reward_atoms * gym_shares / total_shares`. They sum exactly to the block's
original coinbase value, including its transaction fees. No floating point,
rounding, dust assignment or repeated pool-fee deduction occurs here. Variable
targets require a separate versioned extension before they can be accepted.

## Fresh chain evidence and confirmation

`prospectnode.ObservePoolBlock` brackets a stable trusted node tip, exact
canonical parent/winner membership and the winning raw header. The locally
verified source supplies the work hash and original payout-bound proof. It
reports descendants excluding the winning block. RPC failures or changing
observations are errors; they are never synthesized as orphans or confirmations.

The separate `poolround` journal stores immutable rounds and current observed
status. At the first fresh canonical observation with at least sixty
descendants, atomically retain that confirmation evidence. It never changes
on duplicate calls, restart, later tip regression or reorganization.

Before that lock, an orphaned round has no confirmed earnings. Its allocation
plan stays fixed for a possible exact-block return. After the lock, preserve
the confirmed allocations and flag a funding conflict if a later observation
excludes the block. Unavailable node evidence preserves the prior record and
returns an error. Saved observations are historical, not a current payment
authorization. Coinbase maturity, unspent funding and settlement still require
their own checks; reserve advances remain forbidden.

## Durability and API

`Create`/`Open` take explicit finite limits, the concrete pool source and trusted
observer. `Sync` consumes a fixed acceptance tail and appends newly closed
rounds; open-tail shares stay unallocated. A framed prefix digest binds consumed
immutable source evidence. `Refresh` obtains fresh chain evidence for one
round. `Get` and bounded `List` return saved records. `Close` drains owned work.

Use a separate explicitly initialized, pinned journal with synchronous atomic
writes. Reserve status growth so a full journal can still observe existing
rounds. Uncertain writes poison the handle until reopen. Reopen validates
source continuity, fixed membership, allocations and stored confirmation
invariants without obtaining new confirmations. No source schema migration,
automatic eviction, signing key, wallet or listener is introduced.

Independent rational vectors cover membership and allocation. Focused tests
cover the 59/60-descendant boundary, orphan/return, post-lock conflict,
duplicate recovery, source changes, unavailable observations and uncertain
writes. Controlled-node tests exercise the real RPC adapter; they are not a
claim that a live 60-block chain was mined or that the public pool is deployed.
