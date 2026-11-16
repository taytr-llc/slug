# Payable job-bound block candidates (set 20)

`prospectwork` prepares a full block candidate for a D68 job and extracts its
matching proof-v2 evidence. `prospectnode.ResolveTemplate` obtains the complete
template and trusted context from one bracketed node snapshot. The
`prepare-work-node` command additionally asks the node to check the candidate
through GBT proposal mode. It never starts mining or broadcasts a block.

## Inputs and trust

The caller supplies an existing canonical job, an explicit prospect policy,
trusted-node RPC configuration, a SLUG payout address and an extra nonce.
Payout comes from local wallet/operator configuration, never from the node,
authority, gym ID or a default address. The library accepts standard mainnet
P2PKH, P2SH, P2WPKH, P2WSH and P2TR addresses. Raw scripts, raw public keys and
anyone-can-spend payout forms are refused. Address validity does not establish
that a wallet controls it; actual wallet custody/export remains separate work.

The job must be canonical and match the template's chain, policy, parent,
height, difficulty and minimum time. A current template may widen the maximum
time; the chosen timestamp must remain in both the job and template intervals.
The block uses the node's version/current time and nonce zero. Extra nonce is
an explicit 64-bit counter, not an authority timestamp or random challenge.

A parsed job does not independently prove durable issuance. The authority still
requires its own stored job on admission. The builder never assigns birth or
changes the five-minute lifetime. CLI expiry checks are local convenience
checks; authoritative expiry remains the final registry check after proof
verification and node recheck.

## Complete template validation

GBT JSON is bounded to 16 MiB. Required mining fields, optional known metadata
and nested transaction/auxiliary fields use strict schemas. Duplicate decoded
keys, unknown fields, null fields, malformed UTF-8 and trailing input
are rejected. Typed library inputs are also bounded and revalidated.

The client requests `coinbasevalue` and declares `rules: ["segwit"]`.
The pinned SLUG endpoint does not negotiate that rules field; deployment state
governs witness inclusion. This declaration does not broaden the accepted
response schema or add a Bitcoin Core backend.

Full transaction bytes retain witness data. Each transaction is decoded
canonically without trailing bytes; txid, wtxid and weight are recomputed.
Coinbase transactions, duplicate txids and inconsistent or out-of-order
dependencies are rejected. Reported fees and signature-operation costs are
bounded with checked arithmetic; their full truth requires UTXO context and is
subsequently checked by the trusted node's proposal validation. Coinbase value
must equal the chain's height-dependent subsidy plus reported fees.

The local GBT endpoint returns `default_witness_commitment` as **32 raw bytes
in hex**, not as a complete output script. The builder independently recomputes
the witness tree using a zero coinbase leaf and a zero 32-byte reserved value,
checks the returned commitment, then constructs the output script. Transaction
witness bytes are retained in the full block but omitted from the prospect
proof's coinbase encoding, as required by the existing proof format.

## Candidate construction

The coinbase uses version 2, the null previous output, final sequence and
locktime zero. Its scriptSig is the minimal BIP34 height, followed by an
8-byte little-endian extra-nonce data push and the exact node auxiliary flags.
The complete scriptSig must satisfy the 2–100 byte coinbase bounds.

Outputs are ordered as the requested subsidy/fee payout, a zero-value
`SLUGPR02` job commitment, and—when witness transactions require it—a zero-value
witness commitment. The coinbase witness contains the zero 32-byte reserved
value when needed. No template transaction is silently omitted to make room.

The builder recomputes the txid Merkle root and the coinbase's index-zero
right-sibling branch, then emits owned serialized bytes. Final base size,
wire size, weight and signature-operation cost must satisfy both chain limits
and the template's advertised limits, including the newly added outputs.
An otherwise full template can therefore be refused; reserving headroom in
the node's template generator is later integration work.

`Candidate` contains version, job ID, full block hex, the matching proof,
payout address, coinbase value, transaction count, weight and signature-operation
cost. It makes no qualifying-work, account ownership, birth, permanent
canonicality or accepted-block claim. Construction never hashes the header.
Production candidate APIs reject the incompatible `slugtestpow` profile.

## Node proposal and command

```text
slugprospect prepare-work-node --job JOB.json --policy POLICY.json --rpc-config RPC.json --payout-address SLUG_ADDRESS --extra-nonce 0
```

`ResolveTemplate` retains the full GBT result from the same identity/tip-bracketed
snapshot used for its context; it never attaches a second unbracketed template.
The existing context-only API remains available.

`Proposal` checks bounded canonical candidate bytes, expected parent/difficulty
and timestamp before sending them. It checks the node identity/current parent
before and after `getblocktemplate` proposal mode. Only a successful JSON
`null` result means the node accepted the proposal. This null handling is
specific to proposals; other RPC calls retain their required-result checks.
Node errors and rejection text are sanitized. Transport retains the existing
verified TLS/loopback, no-proxy, no-redirect, no-retry and time bounds.

Proposal validation exercises current UTXO, transaction/script, subsidy,
Merkle and witness rules while skipping proof-of-work acceptance. It neither
adds the block to the chain nor proves the candidate will still be current
later. CLI output labels the proposal observation and explicitly leaves work
verification and admission false.

## Next integration boundary

The automatic worker still needs job renewal, bounded nonce scanning, automatic
signing, submission/retry persistence and separate handling of prospect shares
versus block-target wins. Broadcasting a winning block first changes the current
parent and can make its prospect admission fail. Admission first delays block
propagation and can still race another miner. Before automatic broadcasting,
freeze an explicit ordering and failure policy; do not silently broaden
current-parent eligibility or claim atomicity across the node and authority.

Full templates can exceed the service v1 request/response limits. Set 20 leaves
those limits and endpoints unchanged; an eventual remote mining transport
needs its own bounded versioned contract. The local integration can obtain
templates directly from its explicitly configured node.
