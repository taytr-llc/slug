# Official pool accounting — D79, D80 and D81

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

Deep-reorg incident resolution, payout transaction fees, rounding and dust
allocation remain to be specified before implementing money movement.
Set 54 now retains fixed rounds, exact fractional gross allocations and durable
confirmation locks. It uses the accepted-share journal and fresh node evidence;
it creates no payable wallet balance, live service, reserve funding or payout.
Work classification, share acceptance and round confirmation remain distinct
from a successful payment. See `POOL_ROUNDS.md`.
