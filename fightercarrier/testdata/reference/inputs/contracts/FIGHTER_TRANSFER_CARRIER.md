# Direct fighter-transfer carrier v1

Frozen Set73 engineering contract, November 19, 2026. This specifies the Phase-A
carrier codec, fixed writer and recovery boundary under D83. Public ownership
replay, publication/cutoff authority, wallet schemas and consumer APIs are separate
contracts; parsing these bytes alone grants none of their authority. The normal
mechanical probe passed all six cases on109 blocks with clean node shutdown.
Its temporary probe domains are not this protocol. Implementation and full
transfer/restoration qualification remain required.

## Purpose and cryptographic bytes

One transfer consumes one authenticated effective ownership StateID. The state
resolves the fighter, source, current gym public key and prior finality evidence.
It gives the fighter to one distinct valid compressed secp256k1 gym public key.
An external recipient need not have a hosted account. Self-transfers are invalid.
This is a direct gift/off-market transfer, with no marketplace commission.

Hex identifiers below decode to their literal bytes; hashes in transaction
outpoints use the existing wire/internal 32-byte order. The fixed binary intent
is `PinID[32] || PriorStateID[32] || RecipientGym[33] || FundingDigest[32]`
(129 bytes). Every ID is nonzero; individual zero bytes are permitted. Recipient
is a valid canonical compressed point. All integer fields below are unsigned
little endian unless specified otherwise.

Funding tuple bytes are `inputCount:u8 || (prevTxHash[32] || prevIndex:u32)*`.
Count is 1..16, outpoints are distinct and lexicographically sorted by their
36-byte wire encoding. They are exactly the commit transaction inputs in that
order, not selected locators or a claimant's assertion. FundingDigest is
`SHA256("SLUG fighter transfer funding\x00v1\x00" || tuple)`.
Gym authorization digest is
`SHA256("SLUG fighter transfer authorization\x00v1\x00" || intent)`.

Gym signature encoding is `r[32] || s[32]`, big endian scalar components, each
in 1..N-1 and s<=N/2. Use secp256k1 ECDSA and verify against the predecessor gym.
The SLUG writer uses the inherited deterministic RFC6979 signer and low-S
normalization. No arbitrary digest or scalar getter is exposed by either wallet
signing owner. The signer accepts the exact typed, independently checked intent.
Public validation enforces valid low-S ECDSA, not an unverifiable claim about a
remote wallet's nonce generation. Our commit-only recovery guarantee covers the
specified deterministic writer; a nonconforming external commitment may fail
reconstruction and is reported unavailable rather than guessed.

Envelope is `ASCII("SLUGFTR1")[8] || 01 || intent[129] || gymSignature[64]`,
exactly202 bytes. TransferID is
`SHA256("SLUG fighter transfer id\x00v1\x00" || intent)`.
It excludes gym-signature representation, outer funding signature, transaction
ID, local RequestID and native ActionID. Different valid representations of an
identical intent are one semantic send; distinct funding tuples are competing
sends. Reacquisition has a new predecessor StateID, so old intents cannot revive.

Redeem script is exactly `PUSHDATA1 ca <envelope202> DROP PUSH33 <fundingPub33>
CHECKSIG`,240 bytes. Funding public key is the compressed public key in the first
native84 commit input's witness. Its HASH160 must match that actual prevout's
native P2WPKH program; the wallet also proves its concrete covered path. Other
commit inputs must be ordinary native P2WPKH, with valid compressed-key witness
and SIGHASH_ALL signatures. Commit inputs have empty scriptSig. Actual chain
validation establishes their spend validity; ownership parsing does not trust
claimed input values or signatures. Commit P2SH program is HASH160(redeem).

Commit locator payload is `ASCII("SLUGFCM1")[8] || 01 || PriorStateID[32] ||
RecipientGym[33]`,74 bytes. Reveal payload is `ASCII("SLUGFRV1")[8] || 01 ||
TransferID[32]`,41 bytes. Nulldata scripts are the single minimal direct push
after OP_RETURN. Exactly one appropriate locator is allowed in each template.

## Fixed candidate transactions and review economics

These fixed values apply only to this explicit Phase-A candidate writer. They
are engineering fee limits, not a permanent monetary or marketplace policy.
One SLUG is100,000,000 atoms. Both legs have a10,000-atom fee. The commit locks
500,000 atoms, and the reveal or refund returns490,000 atoms. Total network debit
is20,000 atoms (0.0002 SLUG); temporary locked value is0.005 SLUG. A native review
shows each fee, their sum, locked/returned amounts and the full destination gym.

All authored transactions use version1, locktime0 and max uint32 sequence on
every input. Commit has1..16 native84 inputs and exactly these outputs:

1. index0:500,000 atoms to the exact P2SH program;
2. index1:zero atoms to the74-byte locator nulldata script;
3. index2:total inputs minus510,000 atoms to one newly covered native84 internal
   P2WPKH path, at least1,000 atoms. Insufficient selection refuses; no dust
   change is silently absorbed as a higher fee.

Reveal has one input spending actual commit index0 and exactly two outputs:
index0 returns490,000 atoms to the exact commit output2 script; index1 is zero
atoms with the41-byte reveal payload. Its standard writer scriptSig minimally
pushes the canonical DER SIGHASH_ALL funding signature and then the redeem
script. Refund has the same input/return amount/script but only the return
output, with no reveal discriminator. Refund reveals the redeem preimage but
does not transfer a fighter. Its10,000-atom fee is also shown in a fresh review.

The public recognizer requires these value/output/version/sequence templates
and the complete actual prior transaction, prevouts and full canonical block
position. Native84 path control is a local wallet responsibility; ownership
validity does not assume all fee funding keys belong to the gym signer. A
third-party payer still cannot change the signed prior state or destination.

The actual sixtieth descendant establishing predecessor finality must strictly precede the commit's
inclusion height. Initial state requires its authenticated admission. Commit
may be earlier in the reveal's same block; require strict transaction order
when same-height. A commit alone never reserves ownership order. The reveal's
canonical height/transaction index is the D83 competing-send position.

## Public parsing, canonicality and unavailable evidence

The standard writer emits one scriptSig. The public reader accepts every
consensus-valid outer push-only scriptSig up to10,000 bytes that reveals the
exact redeem script and satisfies the actual chain spend: nonminimal outer
pushes, a high-S outer funding signature and harmless preceding stack elements
do not invalidate the embedded canonical low-S gym authorization. Take the
last pushed element as redeem, support opcode pushes exactly as consensus does,
and retain actual raw bytes/transaction ID alongside the semantic TransferID.
Never normalize or replace the on-chain transaction when calculating its Merkle
inclusion. A locally retained transaction's ID is not the public scanner's
search filter. Spend/redeem/envelope limits are distinct.

Scan all transactions in every block of the declared frontier. A malformed
recognized carrier is invalid with an explicit reason; a missing block,
commit/prevout, source row or predecessor-finality dependency is unavailable,
not a proved-invalid earlier competitor. Capacity refuses completeness rather
than truncating history. No hash-only external payload is needed: the complete
intent and signature are present in the public reveal. Standard relay and
normal consensus remain unchanged.

Own-inclusion-block signed checkpoint plus60 descendants finalizes the first
valid canonical send. The core chapter selects the earliest qualifying signed
publication entry and incorporates that entry and the actual60th descendant in
the next StateID. Pending sends leave the previous owner effective. Later
competing sends are void, not queued successor sends. Pre-finality canonical
reorganization recomputes the candidate; finalized conflict retains official
history and reports the community decision boundary.

## Wallet durability and independent reconstruction

Use a dedicated carrier owner and closed journal. Local RequestID precedes
selection; semantic TransferID comes after the exact funding tuple. ActionID is
process/session local. One unresolved carrier request per wallet,128 retained
requests maximum; exhaustion is visible. Original-view/native review is never
inherited by a later session. Relevant effective state and funded prevouts are
rechecked before committing reviewed bytes; unrelated new chain blocks do not
invalidate an otherwise unchanged review by themselves.

Reserve the one internal change/return path in the explicitly versioned hosted
inventory before deriving, consuming or exposing it. Keep both-way exclusions
with ordinary withdrawals. One path serves commit change and reveal/refund
return; the exported recovery frontier covers it before either use. Persist
both exact signed commit and reveal plus permanent selected-input exclusions
before broadcasting the commit. Native SegWit funding means its TxID is known
before its funding witness is exposed. Review fixes the economic/funding intent
and return path; the final commit TxID is computed only after the approved typed
gym signature exists. Sign only the fixed pair matching every reviewed field.
Store dispatch attempts/acknowledgments
separately from canonical observations. Retry only retained bytes after fresh
explicit authorization. A close/cancel does not erase signed/public obligations.

Hosted transfer requires the original FighterTransfer capability and D72 hold
check, through a purpose-limited HostedVault signer. No Mining substitute,
recovery-envelope unwrap, arbitrary script/sign RPC or private-key export.
File-only restored use is an explicit guarded owner after complete schema12
recovery; it cannot be opened by backup Verify or recreate a lost hosted
withdrawal/payment journal. The wallet chapter fixes new manager/allocator
versions and atomic activation before implementation.

After commit-only device/journal loss, scan public commits using the independently
pinned source and restored gym. Resolve prior state and recipient from the
locator, compute the actual funding tuple, reproduce the typed deterministic gym
signature, find/match the first funding key in the covered native84 inventory,
and recompute the exact P2SH program. Verify the actual unspent canonical commit
and its recorded return script is controlled within the recovered inventory.
Only then prepare a fresh reviewed reveal or refund in a new local journal.
No private previous draft, expired native approval or original app journal is a
reconstruction input. A copied envelope cannot match another input tuple. A
reconstruction mismatch is explicit unavailable evidence; never brute-force
private signing alternatives or widen the exported derivation frontier.

## Qualification required before integration closes

Freeze literal inputs and independently derive all canonical bytes, signatures,
scripts, transaction encodings/fees/IDs and refusal cases before product outputs.
Keep the existing mechanical probe domains distinct. Test exact scalar bounds,
malformed point/ID/count/order, all-zero versus individual-zero IDs, cross-pin
and altered tuple, wrong return/output/refund discrimination, full520 boundary
separately from this240-byte script, and alternate consensus-valid outer forms.
Then prove actual normal relay, both legs, exact-byte retry, competing canonical
order, checkpoint/depth boundaries, full exported-key restore and genuine
commit-only journal loss under the combined acceptance contract.
