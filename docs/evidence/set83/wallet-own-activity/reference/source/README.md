# Selected DAILY own-wallet reference

Unexecuted independent Python source for the additive WIRE2 contract
`20ae3033647feb93ddd5c9ae7af123cd8034f2fa582c212b9b476a03163a347b`.
No product serializer, Go executable, wallet, database, RPC, live account,
private fixture or expected output is used. Root alone approves a separate
execution lock and runs this source under two interpreters.

The inherited public authorities are unchanged:

- Set70 activity wire: `fd0f28ff06b4cb0502e7822ad401330941cdb71e9a4d4b5e31d5a017f911dc2b`,
  1,705,410 bytes. Qualified incoming/outgoing/self-payment/unknown-fee and
  bounded-output payloads are reused; their transactions are not regenerated.
- Set67 payout wire: `4b3eafc010d515557097357a968bd007571c7126f7c0f15a2cd4184bcc88f802`,
  488,661 bytes. Wallet-reported observation states retain their old semantics.
- First selected DAILY WIRE1: `b62572b6f1966acad3845348feb446ebb24cb5d1d3c043498a6f95e5b6645f1a`,
  895,972 bytes. It supplies the exact public selection/RemotePoolView/binding,
  old payloads, retained payment windows and qualified old frame-width premise.

The catalog copies 19 explicit public inputs and records each original path,
length and SHA256. Source declarations document the unchanged payload and
wallet bounds. The reference never imports those declarations or either old
producer. Original source paths are provenance; execution reads only the bound
copies. The complete core transaction/storage corpora are intentionally not
duplicated because this slice adds no transaction or ownership rules.

The authored frame inventory is 165 cases: 90 valid and 75 invalid. It includes
all eight original commands in the new envelope, all six new commands,
separate activity sequence correlation, retained old captures, zero/end/tail
pages, payload integer arithmetic, output totals, selected observation states,
genesis height zero with a nonzero wallet tip, exact null/result arms and
canonical framing. The generator checks this hand tally and every authored
decision; a mismatch fails before output rather than relabeling the case.

Five unchanged qualified WIRE1 payloads also have both request and response
controls in the new envelope: paid values at 2^53-1, 2^53 and MaxInt64,
MaxInt64 reserved atoms, and four 4,096-digit rational components. These are
quoted cumulative/rational fields with their original semantics; unquoted
activity amounts still use the signed-safe integer bounds below. This narrow
pre-execution successor preserves the entire prior unexecuted source tree at
`../reference-before-wide-string-controls`. No inherited input, payload,
classification or frame cap changes.

Separate tables have 20 local cursor decisions, 19 decimal spellings and 14
amount displays. Activity amounts remain safe JSON integers, including signed
NetAtoms; only the selected ID/sequence fields are decimal strings. Decimal
money spellings are display/reference arithmetic, not a new wire encoding.
Never sum activity to reconstruct the current wallet balance.

Eight retained-payment context cases distinguish identical First/Current
copies from conflicts, absent/evicted rows and replaced capture/tuple selectors.
They take already validated capture data as a premise; no native capability is
manufactured. Ten original-operation cases are declarative native test
expectations (admission/repeat/cancel/refresh/Stop/Close), not executable owner or
lifetime proofs. Consumers must run contextual cases against the real owner
seam. A stateless request parser cannot know the current View, cache contents
or original captured positions. Shape-valid payment changes are not mislabeled
as parser refusals merely because a particular native capture would reject them.

New activity payload/address examples remain conditional on the qualified old
payloads. The address grammar in this producer accepts only those inherited
public addresses plus the explicitly allowed empty address, rather than
inventing an independent address decoder. The old command result payloads
remain exact qualified premises; WIRE1 consumers retain their full corpus.
Rebinding a historical payload to the selected View is a codec example and
does not establish that the selected wallet owns the historical transaction.

The finite width derivation separately bounds request, old command response,
new small response and page/detail response. It counts all key punctuation,
null arms, full integer widths, original binding fields (generation widened to
20 digits), eight items and 256 outputs. Its deliberately conservative widths
are not jointly attainable financial states. Original caps stay 2,048 request,
65,536 old replies, 16,384 new small replies and 98,304 new page/detail replies,
including LF. The inherited 56,738-byte old response bound is widened only by
the five added null arms, not by raising a command's cap.

Run interface, root only:

```
python3 generate.py --lock /absolute/root-approved-lock.json --output /fresh/external/reference-v2.json
```

The proposed lock has `execution_authorized:false`. Root's separate approved
copy must set `execution_authorized:true` and `approved_by:"root"`; the source
requires both. Bound file paths are absolute under the original source root.
The output is one exclusive 0600 file outside that root, at most 16 MiB. Root
should use a 60-second process limit, a 1 MiB diagnostic capture and the same
approved lock for both Python 3.14 and 3.9 runs. Before/after checks bind every
source and copied input; partial output on a failure is preserved. There are no
network calls, child processes or product calls. AST parsing/hashing of the
source is not a producer execution or a qualification.
