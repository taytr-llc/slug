# DAILY finite reservation contract — source freeze candidate v1

Root-selected Set78 scope: fresh DAILY kernel liability/intake attachment and
original real pool/round/ledger recovery tests. No V1 migration, schema change,
public protocol/runtime rollout, altered payout rule or continuous claim.
This source contract awaits root/peer review and independent producer execution;
no product implementation is authorized by the file alone.

## Pure API and closed inputs

Add to `internal/poolcapacity`, keeping existing V1 code/bytes untouched:

```go
type DailyInputs struct {
    SourceMax    uint64 `json:"source_max"`
    SourceBytes  uint64 `json:"source_bytes"`
    Rounds       uint64 `json:"rounds"`
    RoundBytes   uint64 `json:"round_bytes"`
    CreditRounds uint64 `json:"credit_rounds"`
    Batches      uint64 `json:"batches"`
    Gyms         uint64 `json:"gyms"`
}
type DailyReservation struct {
    Submissions       uint64 `json:"submissions"`
    Blocks            uint64 `json:"blocks"`
    MaxBatches        uint64 `json:"max_batches"`
    ClaimLeaves       uint64 `json:"claim_leaves"`
    LedgerBytes       uint64 `json:"ledger_bytes"`
    RoundBytes        uint64 `json:"round_bytes"`
    LargestRoundBytes uint64 `json:"largest_round_bytes"`
    LargestCreditBytes uint64 `json:"largest_credit_bytes"`
    LargestBatchBytes uint64 `json:"largest_batch_bytes"`
    ReportBytes       uint64 `json:"report_bytes"`
    ReceiptBytes      uint64 `json:"receipt_bytes"`
    SigningBytes      uint64 `json:"signing_bytes"`
    ManagerCopyBytes  uint64 `json:"manager_copy_bytes"`
}
func DailyRecovery(in DailyInputs, submissions, blocks uint64) (DailyReservation, error)
func DailyEnvelope(in DailyInputs) (maximum uint64, reserved DailyReservation, err error)
```

These JSON tags/order are reference/comparison grammar, not a new public wire or
persisted authorization object. No hash domain or trusted serialized reservation
is introduced. Marshal compact ordered JSON without LF when comparing vectors.
Typed arithmetic inputs are unsigned; reference decoding rejects booleans,
negative/nonintegral numbers, unknown/duplicate/reordered keys.

Input validity: SourceMax,Rounds,CreditRounds in1..16384; SourceBytes in4096..8GiB;
RoundBytes in4096..8GiB; Batches in1..32767; Gyms in1..256. They must be bound to
unchanged actual Source2/Round2/Config2 limits by concrete composition. Pure
validation does not authorize a source or permit changing those limits.

`DailyRecovery` validates inputs, then rejects R>N as ErrInvalid. Valid but
insufficient bounds return existing `poolcapacity.ErrCapacity`: N>SourceMax,
R>Rounds/CreditRounds, B(R)>Batches, N+R>32768, R*MaxMoney>MaxInt64, or any listed
source/round/credit/body/report/ledger byte cap. All arithmetic uses checked
uint64 and/or math/big; no float math or overflow-before-comparison. Successful
N=R=0 retains fixed ledger reserves; B(0)=0, not unsigned2R−1.

`DailyEnvelope` finds the largest n in0..4095 for which DailyRecovery(n+1,n+1)
succeeds and `WorstWorkBytes + (n+1)*4096 <= SourceBytes`. Return that n and its
**n+1** reservation. If even n=0 cannot fit, return zero values and ErrCapacity.
An n=0 result is valid drain-only selection, not a promise that a fresh pool can
issue a job. Test n=0 once, then use at most12 binary-search probes (at most13 total
predicate calls). Both all-fit and none-fit endpoints are explicit controls; no nested
N/R scan or lifetime-sized LCM per probe. D and all static frames are computed
once locally or retained as reviewed immutable pure constants, never a trusted
persisted result. Recovery evaluates one linear grammar formula.

## Whole-history and numeric premises

N includes every retained row; R includes every block row whether pending,
rejected, orphaned, canonical or aliased. Never subtract old credits, paid or
pending bodies, reserves, orphan rows, old jobs or projected change. The
unchanged whole-payout planner/single-change/once-consumed frontier gives
B=2R−1 whenR>0, total historical inputs I<=R+B, payments P<=G*B, original claims
C<=N+R. Existing pending signed A counts as one body. Its validated projected
change is for reservation only; new signing remains blocked until unchanged
pending recovery resolves and fresh original funding/source checks pass.

Closed round windows are at most4096. Every original claim denominator divides
D=100*LCM(1..4096); addition/subtraction and integer debit preserve that property.
L(2m)/L(m) divides binomial(2m,m), hence L(4096)<=2^8190. Use conservative decimal
widths denominator2468 and numerator2490: even N*MaxMoney*D atN<=16384 fits.
Neither width changes the existing4096-digit validator. R*MaxMoney<=MaxInt64 is
an intentionally conservative *derived* accounting-headroom check, not an
increase to any financial type. It bounds all cumulative paid/reserved integers.
No stronger actual-subsidy/fee/history bound is assumed.

The manager input-only textual-copy budget is proved separately from the signing
stream: at most C*(64+64+2490+2468+256)+1464*512+400000 bytes, below320MiB
for C<=32768. The512 coin allowance dominates the actual64+44+64+128 check.
The signing stream separately includes up toC input claims AND up toC change
claims and must fit640MiB. Neither test substitutes one budget for the other.

The exact independent frames and formulas in BYTES.md/inputs/frames.json cover
all permanent indexed keys, fixed head/report and per-body delivery reserves.
D84 candidate weight/input capacity is separate: retaining an obligation does
not promise all future mature inputs will fit. Never reduce the candidate,
charge a miner surcharge, advance reserves or occupy a day on deferral.

## Original owner attachment

Add to poolshare (names and signatures selected by this candidate):

```go
type DailyRecoveryBudget struct {
    Rounds       uint64
    RoundBytes   uint64
    CreditRounds uint64
    Batches      uint64
    GymIDs       []string
}
func (p *Pool) AttachDailyOperatorAdmission(ctx context.Context, expected Config,
    budget DailyRecoveryBudget) (*OperatorAdmission, error)
```

No arbitrary maximum argument. Strict Source2 only. GymIDs is nonempty, sorted,
unique, canonical lowerhex nonzero32-byte IDs, at most256; own the slice. It is
exactly the configured settlement destination gym list, checked by composition
against the original `settlement.DailySchedulerBinding` output, and must contain
every selected credential enrollment gym. This membership check grants no session
or signing authority. Original session/public-key/job checks still apply.

Use the same original pool action gate/lifecycle. Derive DailyInputs from the
original pool Config plus the budget; derive the ceiling and reservation there.
Private constructor-selected daily budget/mode on the original opaque
OperatorAdmission may reuse Enable/Drain/Status/Close. Original V1 attachment and
EnableCanonicalRecovery must explicitly refuse a daily handle; old behavior and
error compatibility stay unchanged. No daily budget can be injected after first
attachment; no caller-owned slice/DTO changes it. Zero/foreign handles refuse.

The daily attachment is paused initially; canonical recovery is selected with
it atomically. Root composition must construct/full-audit original source,
rounds and daily ledger, compare exact source and limit identities via existing
DailyBinding/DailySchedulerBinding, finish one-use original wallet attachment,
then attach intake before listeners. The next public runtime is separate.
Kernel tests compose these exact owners directly, with no new generic callbacks.
The Set78 actual also uses direct original DAILY owners with schema16 and normal
nodes; TLS/public runtime integration remains separate.

Audit exact current metadata/namespace/counters, saved work/nonce/sequence/
accepted indexes and original retained block flags before attachment. Reuse
existing daily pure-check memo only after fresh original work and metadata/index
checks; no cached canonicality or persisted verification flag. The candidate
audit must not overwrite current p.meta on an error. Check all issued job gyms
and stored row gyms against the exact set before selecting the budget.

Unqualified above-envelope DAILY history may be held for exact existing-row/
financial recovery, never silently granted a retroactive reservation. Status
OverBound is true if its complete N/R budget or accepted-window/post-ceiling
invariants fail. New intake/new canonical append remains closed in that state;
old obligations are not erased. A valid retained gym outside the configured set is ErrConfig; a malformed
retained gym/source/index is ErrCorrupt, not a budget squeeze. V1 is never adopted.

Before attachment publication, check original context and closing state again.
A context canceled before entry has no effects. If cancellation wins after an
in-memory paused handle was installed, return that exact nonnil handle with the
error; caller retains original Pool/handle for joined cleanup and never serves
it. No durable effect, implicit replacement or auto-retry is permitted.

## Admission, reserved source bytes and recovery

Ordinary n<=4095 derives from reserving(n+1,n+1), not a caller/test quota. At drain
ordinary accepted tail<=4095; all unaccepted saved rows are block winners (the
source commits ordinary retention+acceptance atomically). The first later
accepted winner closes<=4096. After the ceiling, newly retained rows must be
accepted canonical block rows; subsequent new acceptance closes a one-row
window. Exact saved pending block acceptance may close a shorter window. Audit
this ordering/invariant, rather than assuming the metadata count proves it.

Daily Issue checks exact configured gym membership and receipt holdback BEFORE
PrepareManifestIssue/CommitManifestIssue. With current retained rows S and
ordinary maximum n, required remaining receipt bytes are
`(max(n-S,0)+1)*4096`. Require free selected source bytes to cover this plus
WorstWorkBytes=16MiB+16KiB before issuer effects. Check actual serialized work
and holdback again before original saveWork sync. This can drain earlier than
n because of work bytes, without consuming the reserved canonical receipt slot.
Original issuer/job limits remain separate and can also stop new issuance.

Daily ordinary Submit still recovers exact saved nonce first. For a genuinely
new row, keep original physical/clock/pure-check/session checks and the selected
membership guard. At or beyond ordinary drain, only existing saved-row delivery
or the unseen canonical path may proceed. Before every new canonical append,
recount/authenticate complete existing N/R/gym/window/index state under the
original gate, evaluate DailyRecovery(N+1,R+1), and check actual source bytes
for4096 additional retained bytes. No row/index effect on failure.

Then preserve fresh original-node CanonicalBlock proof, final original session
and clock checks, exact timestamp/late classification, and one synchronized
receipt/nonce/global-sequence/accepted-index commit. No new broadcast, fighter
admission or invented confirmation. Before/after-sync faults preserve unknown
outcome and exact original reopen/retry; duplicate retry never increments N/R
or acceptance, changes a timestamp or reserves another payment.

Reuse existing static errors with exact precedence. For attachment: cheap PoW
=>ErrProfile before owner access; nil Pool/context or audit-only =>ErrInvalid;
original cancellation/lifecycle/gate/poison errors follow the existing gate.
Wrong V2 selection, expected Config mismatch, malformed budget/GymIDs or second
attachment =>ErrConfig. Then fully audit existing rows/indexes/jobs before any
capacity classification: a malformed retained gym or impossible internal N/R
is ErrCorrupt; a valid retained gym outside the configured set is ErrConfig.
Only after these checks may valid insufficient envelope headroom return
ErrCapacity with no new attachment. Pure ErrInvalid from a validated internal
census is corruption; pure ErrCapacity stays exact ErrCapacity.

For requests, preserve the original session acquisition and original job-key
binding first. A newly authenticated non-enrolled gym returns the existing
poolshare.ErrUnauthorized ("pool session is unauthorized") before the logical
intake/capacity check and all issuer/receipt effects. It does not poison the
Pool or change draining/OverBound, including when the healthy intake is already
full. No token/unauthenticated gym can turn this request error into global
integrity or capacity failure. Malformed request/token errors retain existing
ErrInvalid/session mapping; malformed persisted work is still ErrCorrupt.
Original Busy, Closed, Poisoned and cancellation propagate; session uncertainty
after actual effects retains original ErrOutcomeUnknown. Mixed errors never
become a capacity hint. Tests distinguish a valid nonmember request/no state
change, valid retained nonmember/ErrConfig, and malformed retained gym/ErrCorrupt.
Status's existing Maximum/Mode/OverBound/CanonicalRecovery/RecoveryBlocks
can represent the selected mode; no new public status serialization is in Set78.
Drain disables new issuance/ordinary retention only. Full original round Sync/
Refresh and daily pending recovery/scheduling remain operational. No source
admission action reenters settlement or wallet gates.

## Qualification boundary

Freeze independent producer and inputs before Go. Required cases include useful
>721 ordinary capacity, derived n and(n+1,n+1), multiple subsequent recovery
candidates, monetary/count/byte overflow, zero blocks, all fixed row/index caps,
report/fraction proof, exact4095+1 window, wrong membership, and monotonicity.
Classifications follow the pure API rules, not implementation-generated outputs.

Meaningful concrete tests use original Source2/Round2/Config2 owner paths and
normal-PoW fixture work, fill the actual derived selected ceiling, hold exact
signed pending A, append a previously unretained canonical winner under its
original job, reopen every owner, retry exactly once, preserve A and import one
credit only. Before/after-sync, old pending saved row, alias/conflict, corrupt tail,
source bytes before issuer effects, zero effects on refusal, cancellation/Close,
V1 parity and cheap refusal are separate controls. Node canonical observations
may be explicitly controlled in these kernel tests; do not claim TLS or actual
wall expiry/node acceptance. Set78 must also run the narrowed actual normal-node/schema16 proof: direct
original DAILY owners, real original job expiry, exact derived ordinary fullness,
unseen canonical B, full fresh-process reopen and one receipt/round/credit with
original signed A unchanged after lost response. B may remain coinbase-immature
at60 descendants; no second payout day or test clock is necessary. Freeze its
independent economics before test code. TLS/public runtime remains following
integration; neither controlled kernel observations nor direct owners claim it.

Further recovery is conditional on full remaining budgets. Physical full and
unbounded future nonce/reorg receipts remain unsolved continuation boundaries;
one reserved winner is not permission to erase D80. Existing post-confirmation
funding-conflict hold is preserved; its incident resolution is not invented.
