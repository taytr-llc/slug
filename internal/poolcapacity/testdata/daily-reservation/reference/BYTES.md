# Independent DAILY reservation grammar and equations

Freeze candidate. No new persisted or wire format. Frames are independent
literal conservative bounds for existing source/round/indexed ledger encodings;
they need not be economically valid. No product encoder/output may supply their
lengths. `J(x)` is ASCII compact JSON, the **listed insertion order**, no LF or
extra spaces, unsigned decimal integers and literal strings; len counts bytes.
The producer must reject duplicate/unknown fields in its source recipe.

Let N=submissions, R=all block rows, G=DailyInputs.Gyms. Hard caps and errors are
in CONTRACT.md. B=0 ifR=0 else2R−1; I=R+B; P=G×B; C=N+R. Each original claim is
stored once; historical change claims are commitments, not repeated lists.
`uint64` conversion happens only after checked/big-integer calculations.

From `inputs/frames.json`, define:

```
F_c = len(J(credit))
F_q = len(J(original_claim)) + 1
F_i = len(J(input_reference)) + 1
F_p = len(J(payment)) + 1
F_b = len(J(batch))
F_x = len(J(reference))
F_a = len(J(allocation)) + 1
F_r = len(J(round))
F_balance = len(J(balance)) + 2*(2490+2468) + 1
F_report = len(J(report))
F_active = len(J(residual_claim)) + 2490+2468 + 1
```

The extra1 counts a comma even for the last array element. Empty arrays/braces
remain in the fixed frame, so the equations overcount their replaced positions.
The fixed frame's null winner and empty raw string are retained while the full
variable value is also added. Hash/script/number/time upper lengths come from
existing validators. The report state and reason frames each use32 ASCII bytes, separately bounding
every validated literal instead of borrowing slack from another field.
Report fraction widths follow the independent common-D
proof, not an assumption that all accepted residual fractions remain short.

The aggregate conservative native signed **raw** bound is

```
RawTotal = 16*B + 150*I + 31*(P+B)
CreditTotal = R*(10 + F_c) + C*F_q
BatchTotal = B*(10 + F_b) + I*F_i + P*F_p + 2*RawTotal
IndexTotal = R*(67+8)                  # cr/<round-hash>
           + B*(67+8)                 # bt/<txid>
           + B*(13+8)                 # bd/<day>
           + (R+B)*(10+F_x)            # e/<u64>
           + (R+B)*(70+F_x)            # o/<txid><u32>: roots and change
           + I*(74+8)                 # spent/<txid><u32>
DeliveryTotal = B*(11+256)             # bs/<u64>, full future state reserve
LedgerBytes = 131072 + 8388608 + CreditTotal + BatchTotal
            + IndexTotal + DeliveryTotal
ReceiptBytes = 4096*N
ReportBytes = F_report + G*F_balance
SigningBytes = 2*C*F_active + 8388608
ManagerCopyBytes = C*(64+64+2490+2468+256) + 1464*512 + 400000
```

Numbered c/ and b/ keys are2-byte prefix+8-byte big endian. cr/,bt/ prefixes
are3+64-byte lowerhex. bd/ is3+10-byte date. References contain original credit
or batch sequence<=32767; literal `credit` is at least as long as `change`/`batch`.
There is at most one object/change key per signed body. Delivery state reserves
all256 bytes immediately. Head and report key/value storage follows the exact
fixed-reserve accounting already used by daily_audit/daily_commit.

R=0 special case: no credits or batches exist, so CreditTotal=0, not N*F_q;
all other row/index/body totals are zero. LedgerBytes retains head/report
reserves. ClaimLeaves=0 atR=0, elseC (a conservative future-original-claims bound).
SigningBytes atR=0 retains only the conservative8MiB nonclaim request frame;
ManagerCopyBytes retains its conservative1464*512+400000 nonclaim allowance.
This special case remains monotone in both N and R.

For R>0, let W=min(N,4096) and A=min(G,W):

```
RoundBytes = R*(F_r+65536+4096) + 67*N + F_a*N
LargestRoundBytes = F_r+65536+67*W+F_a*A
LargestCreditBytes = F_c + (min(G,4096)+1)*F_q
LargestBatchBytes = F_b + 1464*F_i + 256*F_p + 2*400000
```

RoundBytes is original `roundBudget` accounting (canonical Round +4096 status
reserve), not actual disk/compaction. The64KiB winner bound is existing
`maxEntrySize`. Total allocation rows<=N and total closed share keys<=N. Largest
Round excludes its status reserve because `maxRoundSize` limits Round alone.
Largest credit uses one fee plus at mostG miner claims. Largest body intentionally
uses the full daily raw envelope even when that combination of inputs/payees
would be overweight; D84/final signed validation remain independent.

WhenR=0, RoundBytes/LargestRoundBytes/LargestCreditBytes/LargestBatchBytes=0;
the fixed grammar maxima are separately emitted in producer diagnostics for all
cases. ReportBytes remains a conservative selected-G report bound even empty.

DailyRecovery refuses if ReceiptBytes>SourceBytes, any selected row/credit/batch
limit fails, LedgerBytes>8GiB, RoundBytes>selectedRoundBytes,
LargestRoundBytes>2MiB, LargestCreditBytes>128KiB,
LargestBatchBytes>2MiB, ReportBytes>8MiB, C>32768, SigningBytes>640MiB,
ManagerCopyBytes>320MiB, orR*MaxMoney>MaxInt64.
Physical current work bytes are checked by the original Pool and are NOT erased
from its metadata by this pure receipt-only figure.

DailyEnvelope tests(n+1,n+1) and additionally
`16793600 + 4096*(n+1) <= SourceBytes`. Its upper search bound is4095 and input
limits can lower it. All terms above are nondecreasing in N,R with fixed G;
B,I,P,C are nondecreasing, fixed linear coefficients are nonnegative, and min
bounds cannot decrease. Thus each predicate is monotone. An initial n=0 fit check plus at most12
binary-search probes uses at most13 predicate calls, including all-fit and
none-fit outcomes. No expected result comes from iterating a product function.

At operation time original remaining source bytes must additionally cover
`16793600 + 4096*(max(maximum-currentSubmissions,0)+1)` before issuer effects.
An actual savedWork cannot exceed16793600 bytes. ManagerCopyBytes independently
bounds the existing manager calculation: each claim adds both64-byte identities,
its2490/2468-byte fraction and256 bytes, while512 per coin exceeds its actual
64+44+64+128 overhead. The raw input is at most400000 bytes. The producer must
assert this expression atC=32768 fits320MiB; no JSON-frame bound replaces it.
SigningBytes counts input and change claims separately: each<=32768,
never a single shared copy. Original request ownership still binds the exact
body; its conservative8MiB frame covers up to1464 original coin frames, raw
bytes and256 payments. The exact serialized-size and
source counter checks still run before sync. Ordinary Submit consumes one of
its reserved row slots; canonical-only recovery consumes one additional slot
only after its full N/R proof and actual current free bytes succeed.

## Required independent outputs (producer not yet run)

Output each classified case, exact reservation JSON, derived maximum and its
(n+1,n+1) reserve, next failing ordinary candidate, static frame lengths, LCM
value/digits and the separately checked2^8190 bound, total-case monotonicity
checks, and a SHA-bound input manifest. Positive and refusal cases are from
inputs/cases.json. This is conditional arithmetic, not source/PoW/canonicality,
funding, signing, storage-performance or public transport qualification.
