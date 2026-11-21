# Canonical late-winner recovery at ordinary intake capacity

D80 applies to a previously issued winning nonce even when ordinary share
intake has drained. The operator retains its original pool, round accountant,
settlement, custody and scheduler. It does not reopen ordinary intake or alter
the saved work, five-minute expiry, source identity, payout address or limits.

After those original stores pass their normal reopen audits, the operator
attaches a canonical-only recovery budget using their exact configured limits.
`Submit` still recovers an existing job/nonce first. For an unseen nonce after
drain, it checks the original work with normal PoW and requires fresh canonical
evidence for that exact configured-payout block. It does not broadcast unseen
work through this lane. A non-block share, unavailable canonical evidence,
wrong gym, revoked session or insufficient capacity cannot create a receipt.
No fighter is admitted by pool recovery, including after expiry.

The accounting reservation counts two quantities separately:

- N is every retained submission, including the proposed winner.
- R is every retained block-winning submission, including unaccepted, pending,
  rejected, orphaned and same-outpoint variants, plus the proposed winner.

The original ordinary envelope pessimistically used N for both. Recovery
checks the entire history for every possible confirmed-round count r from 1
through R. Fixed nonempty windows contain at most N shares, each window has
at most N-r+1 members, and there are at most N+r claim leaves. The original
exact-fraction, input-order and immutable-recipient planner permits at most
2r-1 successful batches. The bound reserves all historical batches and claim
copies; it never subtracts paid credits, resets fractions or frees a pending
input reservation. `100*lcm(1..N-r+1)` bounds the surviving denominators.

Both the ordinary and recovery calculations use the same conservative V1 byte
grammar. Recovery must fit the existing round count/bytes, credit count,
batch count, 64 MiB financial-state limit and report-growth reserve. The pool's
original physical row and byte limits are checked separately. The recount
also requires retained claim gyms plus configured destinations to
fit the existing 256-balance limit; counting any overlap twice is conservative.
Independent Python reference cases include the genuine ordinary ceilings 2 and 4, recovery
at N=3/R=2 and N=5/R=3, the 183-to184 boundary, and unchanged N=R results.

The canonical receipt, original job/nonce index, receipt sequence and accepted
sequence commit in one synchronized LevelDB batch. The final original-session
clock sample supplies both acceptance and canonical-observation timestamps.
Lost acknowledgment recovery selects that same durable identity. The existing
round consumer retains its original fixed window, and settlement imports its
confirmed credit once by the original round identity. Neither recovery nor
duplicate submission edits an existing payment. The scheduler resolves a
pending payment using its exact retained transaction before importing new
credits or preparing another batch.

This is a finite recovery extension within the original V1 journals, not a
segmented-history migration or continuous public-pool qualification. A physical
row/byte limit or failed financial reservation still holds recovery visibly;
it does not discard the job or extinguish a D80 claim. Same-outpoint variants
remain distinct receipts/rounds and consume conservative capacity separately;
they do not authorize a second funding coin. Existing funding-conflict and
historical-credit rules remain in force. Larger or saturated histories need
the separately scoped continuation work before continuous public operation.
