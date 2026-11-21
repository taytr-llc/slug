# Set78 daily liability and late-winner reservation — proposed, not frozen

Source-only design against Set76/77 sources, 2026-11-20. No implementation,
reference generation, Go/runtime, private store inspection or checkout changes.
This is a finite fresh DAILY integration prerequisite for `PLAN.md`, not a
continuous-capacity or hosted-readiness claim. V1 roots, formulas and APIs stay
unchanged. Root must select the contract before an independent producer/code.

## Existing boundaries that must compose

- `poolshare/operator_admission.go:43` accepts only Source1; its immutable
  ordinary ceiling is at most256. `operator_recovery.go:34,109` enables the
  separate full-history canonical exception; `pool.go:285–328` checks exact
  saved nonce recovery before the ordinary/physical limits. No unseen winner is
  broadcast by that exception. Receipt and accepted indexes commit together.
- `internal/poolcapacity/capacity.go:44,101,112` is specifically V1:64MiB full
  state,256 credit rounds,366 batches. Its `uint64(money)*N` expression at81
  overflows for daily maxima:2,100,000,000,000,000 ×16,384 =
  34,406,400,000,000,000,000 >18,446,744,073,709,551,615. Never widen this helper.
- DAILY has Source2 lifetime<=16,384 rows/8,192 works/8GiB (`poolshare/config.go`),
  Round2 lifetime<=16,384/8GiB but unchanged Round1 window<=4,096
  (`poolround/daily.go`, `model.go:112`), and Config2<=16,384 credits/32,767
  batches (`poolsettle/daily_config.go:15`). None are per-page budgets.
- Indexed settlement reserves128KiB head,8MiB report and256 delivery bytes per
  batch, with128KiB credits,2MiB immutable bodies and8GiB logical total
  (`daily_types.go:10–19`, `daily_commit.go:47–68,125–169`). Source and every
  stored namespace are fully audited before import/recovery/signing. Body input
  references and exact original raw remain immutable; repeated delivery rewrites
  only its retained state/head/report. The fixed reference's full-maxima logical
  upper bound is1,939,922,194 bytes, conditional on its explicit grammar/premises
  (`testdata/daily-ledger/reference.py:222–263`, `reference.json`).
- Active original claims<=N+R<=32,768; shape<=1,464 inputs/256 payees and400,000
  weight. Node readiness sees the whole frontier in one call, not several
  independently sampled pages. D84 defers the whole ready set; it does not promise
  eventual payment or reduce a backlog (`daily_operations.go:46,126`,
  `daily_candidate.go`, `pooltxshape/native.go`).

## Proposed smallest useful envelope

Add **separate** `poolcapacity.DailyEnvelope(DailyInputs)` and
`DailyRecovery(DailyInputs,N,R)` returning a DAILY reservation. Pure inputs bind
original Source2 byte/row limits, original Round2 byte/row limits, Config2
credit/batch limits and the selected bounded gym union. This arithmetic grants
neither source nor funding authority. Use checked integers and big integers;
compare to each unchanged codec limit before converting to native counts.

N counts ALL retained submissions, R counts ALL block rows including pending,
orphaned and alias variants. They are never reset at a day/page/reopen, and no
old credit, spent body, reserved payout or projected change is subtracted. All
possible confirmed histories with r<=R are covered conservatively:

- credits<=R; original claim leaves<=N+R; active origins<=R;
- successful/prepared signed bodies B(R)=0 atR=0, otherwise<=2R−1;
- all historical consumed inputs<=R+B; all payments<=G×B;
- one original-credit row per root, one immutable body per signed batch, every
  event/inverse/spent/day/tx/object index, and all delivery reserves remain.

These are the existing single-change/once-consumed lineage premises. A change
alone cannot create another configured payable balance after the whole planner
has paid each eligible gym down below minimum. Unknown unconfigured claims stay
unpayable. Pending signed A counts in B; its exact projected change is a
reservation projection only. No new signature may use it before original
pending recovery resolves and fresh funding/source checks succeed. Distinct
nonce-only winners may alias the same coinbase outpoint: count each receipt/block
row conservatively; never turn aliases into extra money or silently deduplicate
D80/D81 evidence. Existing conflicting-credit refusal/hold stays in force.

Use the indexed ledger reference's **linear** grammar budget, parametrized by
N,R,B,G (fixed frames + original claim rows + input references + payments + exact
raw upper bound + indexes/reserves). Bound old round storage by R fixed winner/
record/status frames plus N share-key/allocation leaves. The largest round uses
window<=4,096 and<=256 allocations, separately checked against2MiB. A credit
uses<=257 short original claims, separately checked against128KiB. The2MiB body
bound remains the selected maximum raw/input/payment shape, not copied claims.

For residual fractions, use D=100×LCM(1..4,096), not LCM(1..N). Every original
claim denominator divides D; integer fee/payment subtraction and ordered claim
debits preserve that property. All balances and residual claims therefore have
that same common denominator bound. An elementary independent bound suffices:
L(2m)/L(m) divides binomial(2m,m), so L(4,096)<=2^8,190; D has at most2,468 decimal
digits. Even the loose numerator bound16,384×MaxMoney×D needs<2,490 digits,
below the unchanged4,096-digit grammar. With<=256 distinct balance gyms,
2 fractions/balance and a separately enumerated fixed frame, the whole report
fits8MiB. The producer must prove exact frames, counters and integer conversions;
it must not derive expected sizes by calling Go encoders. Include the existing
signed paid/reserved int64 bound: the first conservative proof can require
R×MaxMoney<=MaxInt64 (computed, not an arbitrary new cap). A later actual-reward
sum proof could be less conservative; it is not assumed here.

All resulting byte/count bounds are monotone in N,R with fixed D. The ordinary
ceiling is the largest n whose conservative **(n+1,n+1)** reservation fits,
subject to n+1<=SourceMax and n+1<=4,096. This reserves one genuine unseen
canonical winner and a closing-window slot after ordinary fullness. It gives a
useful ceiling up to4,095 ordinary rows, rather than the old183 or an arbitrary
fixture ceiling. Use a bounded binary search over0..4,095 (<=12 probes) after
proving monotonicity, or closed-form minima plus monotone byte search. Recovery
checks one full (N+1,R+1) bound; no nestedN×R big-integer computation.

This deliberately conservative first ordinary ceiling also solves the
winnerless-window edge without introducing a partial-round or block-only worker
mode. At drain the accepted ordinary tail is at most4,095. Saved unaccepted rows
are block winners only; nonblock rows became accepted atomically on initial
retention. The first later accepted block therefore closes a window<=4,096;
subsequent unseen canonical block appends close one-row windows. Keep validating
this invariant from retained records; a source outside it is held, not truncated.
A more permissive dynamic ordinary-window gate is separate engineering because
refusing ordinary work at that boundary can stop the current worker before it
finds/broadcasts the closing block.

One reserved winner is a **minimum guaranteed post-ordinary-full case**. Further
canonical winners may append only while their whole N/R and physical budgets
still fit. This does not promise unlimited future reorg/nonce receipts, all2^32
variants per job, or acceptance once physical storage is exhausted. The original
finite-history/continuous-retention release boundary remains explicit.

## Source bytes, attachment and exact APIs

Proposed public seam (names/grammar pending root selection):

```
func (p *Pool) AttachDailyOperatorAdmission(
    ctx context.Context, expected Config, budget DailyRecoveryBudget,
) (*OperatorAdmission, error)
```

`DailyRecoveryBudget` binds original round/ledger limits and bounded gym selection
as trusted composition configuration, not caller/miner allowance. Derive the
ordinary maximum inside attachment; do not accept a caller-chosen lower test cap.
One private constructor-selected mode and owned budget can reuse the existing
opaque OperatorAdmission Enable/Drain/Status/Close contract. Legacy attachment
and EnableCanonicalRecovery refuse DAILY; the daily constructor atomically
attaches paused with canonical recovery selected. No persisted trusted cache,
new signer callback, or new source/config digest is needed for this arithmetic.

Before listener enable, Runtime selects fresh DAILY owners, calls original
`rounds.DailyBinding` and `settlement.DailySchedulerBinding` to full-audit and
compare exact Config2/Source2/limits, completes original daily wallet one-use
binding, then attaches the original Pool under its gate. Bind the immutable
credential enrollment/destination gym union (<=64 selected enrollments, existing
<=256 report union) and enforce it on every admitted new row; do not assume a
bare Gyms number authenticates arbitrary sessions. Original session/public-key
checks remain authoritative. Reopen repeats all original audits and attachment;
above-ordinary journals remain drained and may only recover within a valid whole
reservation. No V1 or unqualified legacy DAILY source is silently adopted.

Source byte reservation is essential: current saveWork retains only one row's
space (`poolshare/store.go:354`). DAILY issuance must leave
`(remaining ordinary slots + 1)×4,096` receipt bytes reserved. Before issuer commit,
check remaining source bytes for that reserve plus the existing worst permitted
savedWork size (16MiB template +16KiB job frame); an exact smaller prepared-work
proof is optional only if available before effects. Recheck actual saved bytes
inside the original gate. Work issuance cannot consume the reserved receipt
slot. Stop only new issuance/new ordinary retention on definite reservation
exhaustion; retain Work/Lookup/saved nonce delivery and fresh canonical recovery.
Original physical disk errors/unknown sync remain poison/uncertain outcomes, not
capacity hints. Signed authority counters remain separate hard limits.

Submit keeps the original order: session+original saved work, exact saved nonce
retry, complete physical/integrity checks, bounded pure nonce proof, then separate
canonical recovery when drained. New canonical recovery validates full candidate
N/R/gym/window/source-byte budget before sync, fresh original canonical proof and
original lease/final clock. It performs no speculative Broadcast or fighter
admission. Receipt/index acceptance is one synchronized original write. A lost
acknowledgement or reopen reuses that exact row and accepted sequence. Pure
capacity failures have no append/acceptance; mixed integrity/cleanup errors stay
fatal and cannot be converted into drain.

Do not make settlement take the admission gate or call the wallet from the
source. Its original RecoverPending runs first; full Sync/old-round Refresh and
scheduler remain alive during drain. Exact pending A is observed/rebroadcast
under unchanged checks before importing B. No accounting headroom calculation
changes signed A, a day slot, fees, minimums, reserves or D84's complete ready set.

## Required proof before fresh public enable

1. Freeze independent DAILY arithmetic/codec inputs before implementation:
   normal/full maxima, source-work byte holdback, zero blocks, monotone edges,
   integer overflow,4,095+1 window, gym union, selected smaller limits, indexed
   history/residual fraction/report bounds and sequential recovery candidates.
   Old V1 corpus remains byte-identical. Include a useful>721-row envelope.
2. Real original DAILY pool/round/ledger tests: paused attachment, wrong mode or
   owner/config/gyms, canceled/closed/poisoned owners, ordinary full refusal with
   no issuer/job/row effect, physical byte safety, pending/orphan/alias rows in
   N/R, corrupted tail before capacity, before/after-sync uncertainty, saved-row
   exact retry, original cold reopen, one late acceptance and one credit only.
3. Normal-PoW actual through the explicit V2 TLS client/worker/operator with
   real schema16 daily wallet: fill the **derived** ordinary envelope, retain a
   job before fullness and signed pending A, prove winner nonce absent, wait real
   expiry, make it canonical on the original node, append exactly once while
   ordinary issuance is closed, then fully close/join and reopen in a fresh
   process. Retry the exact job/nonce, check59/60, recover A unchanged after lost
   response, import B exactly once and preserve all input/output/fee/raw/txid
   fields except delivery state. A small selected MaxBatches may give e.g.
   n=721 fromB=1,443 only if every other independent bound permits it; it must
   not be a test-only lower admission ceiling. Independent economics freeze
   precedes test code. Do not relabel Set76's721 rewards as this transport proof.
4. Separate over-weight/backlog control requires visible transaction-capacity,
   no subset/quote/sign/send/day claim, all original balances and pending raw
   intact. Runtime periodic recovery/old-round maintenance continues during
   intake drain. Show finite job/publication/outbox capacity honestly.

No unresolved payout policy is needed for this slice: D58/D79–D81 and D84 remain
settled. Post-confirmation conflicting-funding **resolution** is still open;
retain existing conflict and original-credit history/hold, without inventing a
refund, clawback, advance or replacement transaction. A full physical journal
cannot satisfy unbounded D80 receipts; that is an unresolved continuation
engineering boundary, not permission to weaken the rule or reset history.
