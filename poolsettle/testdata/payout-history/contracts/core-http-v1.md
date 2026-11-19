# Own pool payout history v1 — core/HTTP freeze proposal

**Status: ready for root review; not yet frozen.** November 19, 2026, on the clean Set67 stack from Set66 `247edc868a620dd39ac3bfdf678241066fa4f844`. No production implementation or expected vectors exist. Independent sizing only was executed under root's explicit request; its source, input lock and report accompany this proposal.

## 1. Meaning and authority

This family returns one bounded owned copy of **the authenticated gym's historical settlement projection** from one concrete configured `poolsettle.Store`. It performs no new settlement, node observation, signing, address allocation or broadcast. The original source and journal undergo the existing verification path; a caller-supplied history is never imported into a trusted store.

D58/D79's daily batching, one-percent fee, minimum, exact carry, maturity and identical-transaction recovery remain unchanged. D72 permits a newly authorized held account to read. An old token/view cannot disclose a result after revocation. Recorded batch state and the report timestamp do not prove current canonical-chain membership, current wallet observation, unspentness or sixty-block finality.

The outer pool source binding and authenticated GymID remain separately expected inputs. The native desktop additionally checks the exact Set58 recipient script. The general pool client knows its gym/source from its immutable ClientConfig; it must not treat the returned recipient as an instruction to change wallet enrollment.

## 2. Public API and type names

```go
// poolsettle
const MaxPayoutItems = 366
const MaxPayoutHistorySize = 131072
const MaxPayoutItemSize = 256
const MaxPayoutBalanceSize = 17408

type PayoutItem struct {
    Day string `json:"day"`
    CreatedAtMS int64 `json:"created_at_ms"`
    TxID string `json:"txid"`
    OutputIndex uint32 `json:"output_index"`
    Atoms int64 `json:"atoms"`
    RecordedState string `json:"recorded_state"`
}
type PayoutHistory struct {
    Version uint32 `json:"version"`
    RecipientScript string `json:"recipient_script"`
    Balance Balance `json:"balance"`
    SettlementObservedAtMS int64 `json:"settlement_observed_at_ms"`
    Items []PayoutItem `json:"items"`
}
func (s *Store) MinerPayoutHistory(ctx context.Context,
    sessions prospectservice.SessionAuthorizer, token string) (PayoutHistory,error)
func ValidatePayoutHistory(history PayoutHistory, expectedGym string) (PayoutHistory,error)

// poolservice
const MaxPayoutResponseSize = 131072 // Entire HTTP body, including server LF.
type PayoutHistoryResponse struct {
    Version uint32 `json:"version"`
    BindingID string `json:"binding_id"`
    History poolsettle.PayoutHistory `json:"history"`
}
type PayoutReader interface {
    Payouts(context.Context) (PayoutHistoryResponse,error)
}
func (c *Client) Payouts(ctx context.Context) (PayoutHistoryResponse,error)
```

`ValidatePayoutHistory` is a cheap shape/consistency check with an owned returned Items slice. It neither reads a source nor grants an authenticated history capability. It may remain profile-neutral; actual store/client/service operations refuse the test-PoW profile before source work. Existing `poolservice.Authority`, worker types, balance DTOs, old routes and old response limits do not change. `PoolSessionAuthority` may implement the separate PayoutReader interface; it retains its original account/generation/view and source/transport pin.

Internal parser/helper names are implementation choices. Public API names, DTO fields/order, limits and semantics above are proposed frozen contract. No standalone import, ID, signature or public archive format is added.

## 3. Closed value grammar

Every listed field is required, in listed canonical JSON order. No omitted/null/unknown/duplicate fields. Objects, scalars and the one `items` array have exactly their declared shape; empty items encodes `[]`. All strings are ASCII with no escapes, control characters or alternate lexical form. Hashes are exactly64 lowercase hexadecimal characters. No float/exponent/negative/leading-zero integer representations or booleans in number fields.

### PayoutItem

- `day`: exactly10 bytes, valid Gregorian UTC date `YYYY-MM-DD`; equals the UTC date of CreatedAtMS. Supported epoch is positive milliseconds, so stored actual dates cannot precede1970.
- `created_at_ms`: integer1..253402300799999, and no later than SettlementObservedAtMS. This is original preparation time, not confirmation time. The representable candidate range ends at9999-12-31T23:59:59.999Z; a legacy journal timestamp outside this declared range is unavailable through this family rather than rounded, truncated or silently relabeled.
- `txid`: lowercase64hex. It is the exact retained transaction ID.
- `output_index`: integer0..255, the original index in sorted Batch.Payments. Optional operator change follows all payments and is never returned. Do not renumber after filtering to one gym.
- `atoms`: integer100000000..2100000000000000, exact value of that original payment output.
- `recorded_state`: exactly `prepared`, `broadcast` or `confirmed`.
- Encoded item ≤256 bytes. The preserved independent conservative bound for these fields is200 bytes; the256 cap does not permit extra fields or wider strings.

### Inherited Balance (exact order preserved)

```text
{gym_id, confirmed:{numerator,denominator},
 available:{numerator,denominator}, reserved_atoms, paid_atoms, reason}
```

GymID equals the separately expected authenticated gym. Each decimal-string numerator/denominator is1..4096 digits, canonical nonnegative numerator/strictly positive denominator, with no sign/leading zeros; each fraction is in lowest terms, including zero as0/1. ReservedAtoms/PaidAtoms are integers0..2100000000000000. Available is nonnegative and ≤Confirmed−Reserved; Confirmed−Reserved is nonnegative. When Confirmed=0, Reason is empty. Preserve the inherited interpretation: Confirmed is confirmed **unpaid** entitlement including reservations; PaidAtoms is retained confirmed payment total. Encoded Balance ≤17408 bytes.

Reason vocabulary is exactly:

```text
"", payout-pending, broadcast-unknown, transaction-unavailable,
payout-conflict, funding-unavailable, funding-conflict, funding-changed,
immature, awaiting-confirmation, below-minimum, destination-unavailable,
daily-batch-complete, fee-unavailable, fee-budget, change-dust,
signing-unavailable, capacity
```

No raw backend error or fee surcharge becomes a Reason.

### PayoutHistory

- Version is1.
- RecipientScript is either empty for a gym absent from the configured destination table, or exactly44 lowercase hex characters encoding `00 14` followed by20 bytes. A configured gym returns its existing script even before its first payment; a history with any item requires the configured nonempty script. Do not expose any other destination.
- Items count0..366. Core configuration and the shared runtime both allow **366 batches** (`MaxRounds=256` is a different limit). This family must not truncate at256 or silently enlarge the existing journal limit.
- Newest-first strict descending Day **and** CreatedAtMS; all TxIDs are unique, hence outpoints unique. The projection cannot create a duplicate payment for one gym from one batch.
- At most one own item has state other than confirmed. If present it is item0; all later items are confirmed. The existing global journal permits an unfinished state only on its newest global batch, so an own filtered list cannot contain an older unfinished payment.
- Checked sum of confirmed own item atoms equals Balance.PaidAtoms exactly; checked sum of other own item atoms equals Balance.ReservedAtoms exactly. Each sum and their checked combined total stay within the inherited money bound. Neither value is taken from a separately refreshed balance request.
- An empty recipient implies zero Items, PaidAtoms and ReservedAtoms. If this gym nevertheless has unpaid Confirmed entitlement, Reason must be destination-unavailable (the existing report rule). No placeholder/zero payment row is emitted for a deferral or a day without a batch.
- SettlementObservedAtMS is integer0..253402300799999. Zero requires no items, zero fractions, zero paid/reserved and empty Reason. A configured script may still exist in that initial state. Nonzero is the captured Report.ObservedAtMS, not the current request clock.
- The captured report timestamp must cover every item's original preparation time. It describes the most recent retained settlement attempt/report. It must **never be presented as each batch's confirmation time**: old batches do not retain an individual state-observation timestamp, and an early failed attempt may not have reobserved every later row.
- A historical `confirmed` state may coexist with a current retained payout-conflict Reason. Never rewrite a recorded state, erase locked entitlements or assert fresh confirmation to eliminate that distinction.
- Encoded canonical history ≤131072 bytes. The preserved conservative upper bound for its closed grammar is90363 bytes; the larger cap is a buffer limit, not permission for additional fields.

The response version is1 and BindingID exactly equals the client's independently configured existing pool BindingID. The HTTP client checks the entire history against its independently derived gym. The desktop separately checks its retained source and recipient. Pure shape validation is not authentication of arbitrary claimant JSON.

## 4. Core operation and lifetime

1. Normal profile and nonnil context/source/session validation precedes work. Acquire the existing fail-fast settlement operation permit; a busy caller does not queue additional work.
2. Capture original token/key using existing mininglease behavior and derive the gym under the immutable source policy. No replacement token is substituted after work begins.
3. Execute the existing `Store.verify` under the operation permit, **outside** an account lease. It audits disk/memory identity, source SettlementSnapshot/consumed proof history, exact retained signed transactions through liveCoins and the current Report. Do not call SourceConfig immediately before this just to revalidate the same source twice. Do not skip source verification to meet a deadline.
4. From this one verified state, filter exact Payments to the original gym, derive each output index from its original Payments position, copy the Report's exact own Balance/default zero and configured recipient, and build the bounded newest-first projection. Validate it without allocating proportional to an unchecked slice/string. A corrupt source/state gives no partial history.
5. Recheck the original credential and context before returning an owned value. No account lease spans proof work, journal gate wait, network I/O or owned serialization. Cancellation joins an entered proof; Close waits for the operation permit. No detached hash task or success after cancellation.

This is a read-only application operation: no settlement commit, new clock highwater, admission, signature, funding observation or broadcast. Existing authorization internals may retain their established authentication audit behavior. Returned strings are immutable values and Items is copied; caller mutation cannot alter live state.

Local desktop pages consume the retained owned capture and **do not call this method, the source or normal-PoW verification again**. An explicit refresh obtains a new bounded capture; a later scheduler transition does not mutate the old one. The exact private wire/capture lifetime and aggregate resource budget are frozen separately by the desktop contract.

## 5. HTTP request and framing

Only additive `POST /pool/v1/payouts`. Request canonical value `{}`; no gym, address, cursor, amount or transaction selector. Entire request body limit remains1024 bytes. Canonical JSON interior is required; decoders may accept surrounding ASCII JSON whitespace (`space`, `tab`, `CR`, `LF`) within the total cap. Non-ASCII padding, multiple JSON values, internal whitespace/alternate ordering, duplicate keys, malformed/oversized bodies and any other value are rejected. Existing routes retain their existing behavior.

Success is HTTP200 with canonical `PayoutHistoryResponse` JSON followed by exactly one LF. **MaxPayoutResponseSize131072 counts that LF and all other received bytes.** A decoder accepts canonical interior with only the bounded surrounding ASCII JSON whitespace above. No trailing second value or internal whitespace is accepted. Header, compression, redirects, content-type, no-store and no-sniff behavior follows the current pinned pool transport.

Preflight must bound the raw body and closed structure before decoding Items: at most366 items, four fraction strings at most4096 digits each, fixed bounded item/scalar strings and container nesting at most4 (response object → history → array/item or balance/fraction). No other array path is allowed. Validate the full closed grammar/canonical re-encoding after decode. This ensures a128-KiB body cannot force an unrelated giant slice of tiny objects before the item bound is checked.

Request routing retains literal-loopback Host/local-address checks, no Origin, no query/absolute/escaped route, no content encoding, original bearer, read rate4/s burst8 and the existing one active service operation. Read-capacity limits remain per original gym/key, not per token rotation. The30-second operation/client lifetime and existing HTTP server bounds are unchanged. Before writing success, serialize within the family cap and reauthenticate the same original key/token outside core work. Release the service operation permit before a slow response writer occupies it.

The new response cap is selected **only** for payouts; work/balance/submission/error limits stay unchanged. Bad HTTP/media/frame/source/gym/history/cancellation yields no payload. Neither a response nor its checksum is a public signature or full-pool audit proof.

## 6. Errors

Store read errors retain existing sentinels (ErrInvalid, ErrProfile, ErrBusy, ErrClosed, ErrPoisoned, ErrCorrupt, ErrSource, ErrCapacity/clock where inherited), original authorization errors and context cancellation. Pure ValidatePayoutHistory returns ErrInvalid for rejected supplied data. No new economic state or outcome-unknown mutation is introduced.

This HTTP family permits only the existing error envelope:

```text
{version:1,error:{code:<below>,outcome_unknown:false}}
```

Codes/statuses: invalid_request400, unauthorized401, forbidden403, too_large413, rate_limited429, busy503, capacity503, unavailable503. Bad/revoked original authentication maps to unauthorized; malformed request to invalid_request; size before decode to too_large; actual occupied permit to busy; rate-table/resource admission to capacity; exhausted rate to rate_limited; closed/corrupt/poisoned/source/profile/backend/cancelled validation to unavailable. No raw errors, source bytes or partial history are returned.

`not_found`, `expired`, `rejected` and `outcome_unknown` are not payout-history results. There is no missing-history error: an authenticated verified no-payment account receives a valid empty history. A malformed or unexpected error code/flag/status in the new client is unavailable. Transport failure is unavailable with OutcomeUnknown=false; it cannot have submitted a payment. Error bodies retain the existing65536-byte transport bound, not the new success cap.

## 7. Independent byte bound (sizing only)

`set67-prep/sizing/size_bounds.py` was source-locked before execution (`f42341a159ef9639986b4e8bda093ac26f9323e218f9b5770b1bf49ab07ff536`). Its hand-counted field-length arithmetic agrees with Python canonical encoding. It imports no SLUG code, reads no journal, runs no work and produces no evidence vectors.

Conservative syntactic maxima include all366 rows, all four4096-digit fraction strings, maximum atom/index lengths and the longest Reason. The preserved sizing producer uses a longer16-digit timestamp ceiling8640000000000000; the final candidate grammar uses the narrower15-digit ceiling253402300799999. Keeping the original longer lengths is intentionally conservative, so no sizing result was replaced after narrowing the contract:

| Value | Bytes |
|---|---:|
| PayoutItem | 200 |
| Inherited Balance | 16,652 |
| Items array, including delimiters | 73,567 |
| PayoutHistory | 90,363 |
| PayoutHistoryResponse JSON | 90,467 |
| HTTP body including one LF | 90,468 |
| Headroom below131,072 | 40,604 |

These extrema deliberately do not form a valid history (for example all rows at maximum money would violate sums, and equal max fractions are unreduced). Their combined length is a rigorous **upper bound**, not a claimed valid corpus case or economic outcome. Maximum legitimate strings/counts are not shortened to make the cap pass. The2048/8192 bounds from Set66's smaller metadata family are not reused here.

The retained capture and incoming decode/validation/serialization copies need separate accounted owners; this byte result is **not a Go heap measurement**. The desktop proposal owns that aggregate accounting and one fail-fast slot, retaining the previous capture safely while refresh prepares a replacement. No hidden mining stop or app-layout change is authorized by this contract.

## 8. Freeze and oracle sequencing

Root reviews this exact contract, sizing inputs/report and independent-input/vector plan. Only after approval are frozen copies and hashes recorded. Original actual inputs are extracted by the separately frozen read-only old-journal extractor; no current app is opened on the historical Set61 root. Producer source/recipes and input hashes freeze before expected outcomes. Core implementation output never supplies expectations. Private wire/wallet/lifecycle grammar freezes separately and must refer to this exact core definition rather than duplicate an incompatible Balance or timestamp meaning.
