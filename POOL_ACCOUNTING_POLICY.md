# Official pool accounting — D58 completion, D79–D81 and D84

Mike approved per-block proportional rounds on November 18, 2026, Pacific.
The pool's daily payment schedule does not define its share-allocation window.

Each pool-found block closes the round containing shares accepted since the
previous pool-found block, including the winning share. Allocation is weighted
by share difficulty. It must be reproducible from the accepted-share log and
chain evidence; a miner-supplied difficulty or unverified share is not credit.

Daily settlement sums each miner's closed-round earnings, applies the existing
1% fee, pays balances at or above 1 SLUG, and carries smaller balances forward.
Carry-forward balances must not incur the same fee again. There is no
calendar-day share division. The known pool-hopping tradeoff of pure
proportional allocation is accepted at this stage; PPLNS is not authorized.

Mike clarified the two separate gates immediately afterward:

> Confirmed: rounds confirm at 60 blocks deep, earnings lock to the miner at that point, and settlement pays on the first daily run after the block's coinbase reaches 100-block maturity. No advances from pool reserves, ever.

The confirmation threshold is sixty descendant blocks, excluding the reward
block itself. Earnings lock to the miner then. Settlement waits for the first
daily run after the original coinbase is spendable under the existing
100-block consensus maturity rule, still subject to the 1-SLUG balance
threshold. Use the node's actual spendability rules, not an elapsed-time
estimate. Reserve advances are forbidden, including between confirmation
and maturity. Do not change consensus or promise a sixty-block cash payout.

Confirmation does not make the chain irreversible or create a signed league
checkpoint. If a later reorganization removes the funding coinbase, both
constraints still apply: keep the locked earnings and block the unfunded
payment. Do not silently erase those earnings or fund an advance from another
source. Incident resolution requires separate operational policy.

## Canonical late winners — D80

Mike approved the canonical late-winner exception on November 18, 2026:

> If a share submitted after its job expires is the winning share of a block that becomes canonical and pays the configured pool address, count the winning share in its round and distribute the block reward under D79. Ordinary late shares remain rejected, and no fighter admission or birth results from an expired job.

The exception requires the exact locally verified work, the retained configured
pool payout, and a trusted node's canonical-block observation. A successful
broadcast alone is insufficient. Timely accepted shares retain their original
acceptance through later expiry; delayed broadcast acknowledgment never creates
a second share. Late block evidence may be retained and broadcast while its
eligibility is pending, without claiming accepted credit or canonicality.
Canonical membership can later change; D79's confirmation, maturity and funding
rules still apply. No block or pool receipt bypasses D68 fighter admission.

## Fixed orphan rounds — D81

Mike approved this rule:

> Confirmed: if a pool-found block is orphaned before 60 descendants, keep its round's shares fixed and award no earnings while it is orphaned. If that exact block returns to the canonical chain, the same round resumes confirmation. Shares are never reused in another round.

Use immutable acceptance order to define the windows: each accepted winning
share closes the interval after the previous accepted winner through itself.
An unaccepted block intent closes nothing. If D80 later grants its acceptance,
it appends a winning share and round rather than inserting a past boundary.
Delivery observations may change; membership and the winning share's original
block binding do not. A timely accepted block win can retain a round while its
canonical status is unresolved, but earns nothing without canonical confirmation.

Fresh trusted node evidence controls confirmation. Before sixty descendants,
an orphaned round has no confirmed earnings. After a prior sixty-descendant
lock, D79 retains the earnings and blocks unfunded payment on a conflict. RPC
failure is unavailable evidence, not evidence of an orphan or a new lock.

## Daily settlement — completion of D58

Mike approved the remaining settlement details on November 18, 2026:

> Approved. Carry fractional balances forward, pay at 1 SLUG, fees from the pool's 1% cut, defer rather than charge miners if the cut cannot cover the fee.

He additionally requires one daily transaction batching the eligible miners,
and a visible reason in each affected miner's balance view when payment is
deferred. This completes D58 and clarifies D36/D79; it does not invent a new
daily cadence, minimum, or additional fee decision.

Keep reduced exact fractions of atomic units. Deduct the 1% once from each
round's gross allocation, sum funded net balances, pay whole atomic units when
the balance reaches 100,000,000 atoms, and retain every remaining fraction.
The pool pays network transaction fees solely from its earned 1% portion.
If that portion cannot cover the quoted fee, defer rather than reducing miner
principal. Never discard miner-backed change as an implicit dust fee.

Payments spend the original mature canonical rewards and retained settlement
change carrying those rewards' remaining claims. A wallet's unrelated balance
is not funding. Confirmation remains sixty descendants; coinbase spendability
is the consensus check at the next block height, so a currently canonical
coinbase with 99 descendants can be included in the next block at maturity.

Before broadcasting, atomically retain the exact signed transaction, its daily
slot, input reservations, miner debits and remaining claims. A retry or uncertain
broadcast may only recover or retransmit those same bytes. Pending payments
remain reserved, not paid. The first milestone must show confirmed earnings
reaching an actual on-chain payout with a retry that cannot pay twice.

Deep-reorg incident resolution remains separate: retain locked earnings and
payment history, visibly block conflicted funding, and never make reserve
advances. Work classification, share acceptance, round confirmation and
successful payment remain distinct. See `POOL_ROUNDS.md` and
`POOL_SETTLEMENT.md`.

## Whole-batch capacity deferral — D84

Mike chose **“Defer the entire payout until it fits”** on November 20, 2026.
When the complete ready funding/payment set exceeds one supported transaction,
defer the entire daily payout. Do not choose an oldest-first subset or split
miners into multiple transactions. Retain every exact balance, add no debit or
reservation, and leave that day available for a complete retry. Show the
transaction-capacity reason in the affected balance view.

This choice preserves the one-transaction daily contract and avoids choosing
which eligible miners get paid first. Its tradeoff is an operator capacity
hold: simply waiting may add inputs rather than make the batch smaller. A
future capacity upgrade must preserve the complete batch and all original
funding claims; this decision authorizes neither consolidation spending nor
a change of payout policy.

Inspect funding in bounded pages if necessary, but retain the complete ready
set. Page boundaries are never payout boundaries. Recheck original funding
before a new signature/broadcast. Recover any existing reserved payment first,
using only its exact signed bytes. The 60-descendant lock, 100-block maturity,
1 SLUG minimum, 1% fee, fractional carry and no-advance rules remain in force.
