# Set79 independent frame bound

Source-only arithmetic cross-check; no Go, reference-vector generation or network.
All counts below are UTF-8 bytes of compact ordered JSON. Identifiers, decimal
strings, dates and closed reasons need no JSON escapes. HTTP response bounds
include the existing single final LF; embedded component bounds do not.

For an object with n fields, define
`O = 2 + (n-1) + sum(len(key)+3+encoded_value_length)`.
A quoted ASCII value of length k occupies k+2 bytes. This counts every brace,
comma, quote and colon; field order does not change its size.

## Scalar and component arithmetic

- A fraction is `O(numerator:4098, denominator:4098) = 8225`.
  The two components are independently bounded at 4096 canonical decimal digits.
- The daily balance is
  `O(gym_id:66, confirmed:8225, available:8225, reserved_atoms:21,
  paid_atoms:21, reason:25) = 16662`.
  Each int64 decimal string has at most 19 digits plus quotes. The longest
  allowed reasons, `transaction-unavailable` and `destination-unavailable`, have
  23 ASCII bytes. This is below the 17408-byte balance cap by 746 bytes.
- The unchanged PayoutItem is
  `O(day:12, created_at_ms:15, txid:66, output_index:3, atoms:16,
  recorded_state:11) = 199`.
  Existing item validation bounds time at 253402300799999, output index at255,
  amount at2100000000000000, date at10 bytes, and recorded state at9 bytes.
  It fits the existing256-byte item cap with57 bytes left.
- The wrapper is `O(batch_sequence:7, payment:199) = 236` because selected
  batch sequence<=32767 has at most5 digits. Eight items take
  `2 + 7 + 8*236 = 1897` array bytes. This counts eight globally selected slots;
  filtering out unmatched gyms only reduces the result.

These are exact arithmetic upper bounds over the permitted field lengths,
not a claim that all maxima coexist in an authenticated financial journal.
Reduced fractions, balance consistency, captured revisions and full ledger audit
remain mandatory and may reduce attainable sizes.

## Whole frames

| Frame | Bound, bytes | Existing cap | Margin |
| --- | ---: | ---: | ---: |
| Daily balance response, including LF | 16960 | 65536 | 48576 |
| Daily payout page, including LF | 18979 | 131072 | 112093 |
| Selected continuation request | 79 | 1024 | 945 |
| Work response, including LF | 16785686 | 16842752 | 57066 |
| Submission response, including LF | 4360 | 65536 | 61176 |

Balance/page rows above conservatively allow a19-digit nonnegative int64
observed_at_ms. If the daily page retains the existing payout-reader maximum
253402300799999 explicitly, its exact bound is18975 rather than18979. Thus
neither timestamp interpretation needs a cap change. The ordinary balance path
currently permits the full nonnegative int64 range; do not silently assume a
15-digit timestamp there.

The balance envelope, excluding its balance, is297 bytes with a19-digit time,
so `16662 + 297 + LF = 16960`.
The page envelope, excluding balance and items, is419 bytes with a19-digit time:
`16662 + 1897 + 419 + LF = 18979`. Its revision uses20 digits, high_batch,
before_batch and next_before use5 digits, and done uses the longer literal
`false` (5 bytes). An empty or final page is smaller.

The selected request is
`O(revision:22, high_batch:7, before_batch:7) = 79`.
Even three generic uint64 string maxima would occupy only109 bytes, but high and
before must still pass the selected32767/32768 semantic bounds. Requests are
canonical without LF; allowing one for a transport measurement adds only1 byte.

The new settlement_config_id and recipient_script fields add exactly156 bytes
to each old work/submission success envelope: two commas plus89+65 field bytes.
The complete daily envelope overhead is257 bytes for work and263 for submission.
Embedded data remains an object, not a quoted/escaped JSON string.

The retained template's exact marshalled bytes are at most16777216 and the
existing ManifestJob is at most8192. MiningWork contributes20 object bytes:
`work_response <= 16777216 + 8192 + 20 + 257 + LF = 16785686`.
The existing cap is `MaxTemplateSize + 65536 = 16842752`.
For submission, the original retained receipt cap4096 gives
`4096 + 263 + LF = 4360`. No component bound, normal PoW check, immutable receipt
or response validator is weakened by these new fields.

The embedded prospectjob.ManifestJob is Version2, while admission transport is
v3. An earlier contract naming typo is corrected in the pinned6f1e664b successor;
original job bytes and the8192-byte bound are unchanged.

## Source anchors

The proof uses the frozen selected field inventories and existing:
`poolsettle/payout_history.go:17` (item/balance caps and time bound),
`:25` (item fields), `:85` (item domains), `poolsettle/types.go:20` (MaxMoney),
`:51`/`:120` (fraction/balance fields), `poolsettle/report.go:9` (closed reasons),
`poolservice/client.go:255`/`:277` (fraction/time bounds),
`poolservice/types.go:19` (response caps), `poolshare/config.go:28` (receipt cap),
`poolshare/pool.go:175` (exact marshalled template bound),
`poolshare/mining_work.go:15` (embedded object fields), and
`prospectjob/manifest_json.go:111` (canonical8192-byte job bound).
Source files and the reviewed contract are pinned in the adjacent input record.
