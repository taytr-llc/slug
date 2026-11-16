# Prospect work proofs, format 1

Set 14 adds an offline verifier for work committed to one gym identifier.
It is an off-chain prerequisite for registration, not a registration service,
ownership ledger, account authenticator or full block validator. A supplied
context file is a trust input: the CLI does not independently authenticate
its chain, target or time information. There is no default production
prospect target and no public verification endpoint.

## Trust and format

The trusted `Context` JSON contains exactly these required fields:

```text
version (uint32, 1)
network (uint32, nonzero network magic)
genesis (64 hex characters, displayed hash order, nonzero)
dna_version (uint32, currently 1)
prospect_target (64 hex characters, unsigned big-endian integer, nonzero)
parent_hash (64 hex characters, displayed hash order, nonzero)
parent_height (uint32, at most 2147483646)
expected_bits (uint32, nonzero)
min_timestamp (uint32, inclusive)
max_timestamp (uint32, inclusive, at least min_timestamp)
```

The node/league adapter must resolve the canonical parent, correct next-block
bits, accepted target policy and timestamp interval independently of the
claimant. This format checks equality against those supplied values; it does
not fetch chain state, establish freshness or enforce a policy schedule.
The prospect target is independent of header bits and pool variable
difficulty. It may be easier or harder than the block target. Calibration to
D30's discovery cadence is a later deployment prerequisite.

Set 16 implements the current-parent adapter in `prospectnode`, with online
CLI commands and guarded registry admission. Its supported network, explicit
policy, transport rules and observation limits are documented in
`TRUSTED_PROSPECT_CONTEXT.md`. The offline verifier's contract is unchanged.

The untrusted `Proof` JSON contains exactly these required fields:

```text
version (uint32, 1)
gym_id (64 hex characters, nonzero opaque identifier)
header (160 hex characters: exact 80-byte serialized block header)
coinbase (hex: canonical stripped transaction, at most 16384 decoded bytes)
merkle_branch (array of at most 20 displayed-order sibling hashes)
```

The coinbase is always transaction index zero. Every supplied sibling is on
the right. Empty branches are valid for a coinbase-only template. There is no
caller-controlled index or asserted transaction count. Duplicate fields,
unknown fields, nulls, missing fields and trailing JSON values are rejected.
Context files are limited to 4096 bytes; proof files to 49152 bytes including
whitespace. Hex accepts either case and uses no prefix or whitespace; output
hashes and identifiers are lowercase.

## Commitment bytes

`LE32` is a four-byte unsigned little-endian integer. `displayBytes` means
decode the 64-character displayed hash directly without reversal. Targets
and gym IDs are also decoded directly. SHA256 below is a single SHA-256.

```text
policy_id = SHA256(
  ASCII("SLUG prospect policy") || 00 || ASCII("v1") || 00 ||
  LE32(network) || displayBytes(genesis) || LE32(dna_version) ||
  bigEndianTargetBytes)

commitment = SHA256(
  ASCII("SLUG prospect gym") || 00 || ASCII("v1") || 00 ||
  policy_id_bytes || gym_id_bytes)
```

The coinbase must contain exactly one reserved commitment output, value zero,
with this exact script: `6a 28` followed by ASCII `SLUGPR01` followed by the
32 commitment bytes. This is OP_RETURN, a direct push of 40 bytes, and the
marked payload. The `SLUGPR` prefix reserves the family; detect that prefix
in the first pushed data after OP_RETURN even for nonminimal push encodings.
Reject duplicate family outputs, unknown family versions, extra script data,
noncanonical pushes and any mismatch. Other OP_RETURN families, such as the
witness commitment, are not prospect commitments.

Requiring one family output prevents one winning header from naming several
gyms. Changing the gym, DNA version, target or chain identity changes the
commitment and therefore the work. Anyone may relay an existing proof, but
it can only name the identifier already committed before hashing. The
identifier's account or wallet control is not authenticated here.

## Validation order and costs

1. Refuse verification when built with `slugtestpow`. Validate the complete
   trusted context and all proof sizes/encodings before expensive hashing.
2. Decode the exact header and require the supplied parent hash, exact bits
   and inclusive timestamp bounds. Header time is miner-controlled; it is
   not an authenticated discovery time or fighter birth date.
3. Preflight stripped transaction lengths without allocating declared script
   sizes. Require canonical CompactSize integers, one input, scriptSig length
   2–100, 1–256 outputs, bounded scripts fitting remaining bytes, locktime and
   EOF. Then decode with the existing wire decoder, require no witness and
   exact stripped reserialization. Require a null previous outpoint with
   index 0xffffffff. Output values and their sum must be nonnegative and at
   most 21,000,000 * 100,000,000. Require the canonical BIP34 encoding of
   `parent_height + 1` at the start of the scriptSig. This is a format rule
   for new prospect templates, not a change to chain consensus.
4. Verify exactly one canonical commitment output. Compute the ordinary
   SHA256d txid. For each sibling, convert displayed hash text to internal
   wire byte order and calculate SHA256d(current || sibling). Reject equal
   current/sibling hashes; index zero never needs an odd final-node duplicate.
   Require the resulting root to match the header.
5. Compute one production header hash through `wire.BlockHeader.BlockHash`:
   Argon2id, t=1, m=256*1024 KiB, p=1, 32 bytes. Interpret its displayed hash
   as an unsigned big-endian integer and require it to be <= prospect_target.
   The package serializes its Argon calls to bound concurrent active work.
   Callers still need admission/queue limits; Argon cannot be cancelled once
   started, and this is not a global node resource quota.
6. Generate DNA using `fighter.ParseSeed(workHash.String())` and the trusted
   DNA version. Do not pass internal reversed hash bytes to the DNA generator.

The branch proves the coinbase commitment and header work. It cannot check
other transactions, hidden duplicate subtrees, spends, rewards, template
provenance, pool acceptance or full block validity. Bad supplied proofs must
not poison a persistent header-validity cache.

## Verified record and identity

```text
prospect_id = SHA256(
  ASCII("SLUG prospect id") || 00 || ASCII("v1") || 00 ||
  LE32(network) || displayBytes(genesis) || displayBytes(work_hash))
```

The lowercase ID excludes claimant, policy, gym, proof encoding and receipt
metadata so repeated representations of the same header on the same chain
cannot mint additional IDs. The verified JSON records format version,
`verification: "supplied-context"`, chain identity, prospect ID, policy ID,
gym ID, work hash, parent hash/height and DNA. It is returned only on success.

Set 15's local evidence registry (`PROSPECT_REGISTRY.md`) persists IDs and
first evidence across restarts, with no deletion or reset operation. Future
status handling must preserve these IDs across reorgs. Actual registration
still must authenticate gym/account control and transfer authority, establish
freshness and an immutable birth anchor, and publish records with checkpoint
anchors. Delaying submission must never reset aging. These rules are not
implied by this verifier, and the league signing key remains report-only for
chain selection.

The height prefix follows [BIP 34](https://github.com/bitcoin/bips/blob/master/bip-0034.mediawiki),
including OP_1 through OP_16 for the first sixteen heights and the separate
two-byte minimum scriptSig length. The limits of partial-tree evidence and
duplicate-root ambiguity follow the behavior documented in
[Bitcoin Core's Merkle implementation](https://github.com/bitcoin/bitcoin/blob/master/src/consensus/merkle.cpp).
The verifier does not treat a malformed proof as proof that its header is
permanently invalid.

## Tools and independent fixtures

`slugprospect commitment --context trusted.json --gym-id HEX` prepares the
public commitment script for a mining template. It does not sign or mine.
`slugprospect verify --context trusted.json --proof share.json` verifies one
proof conditionally against that separately supplied context.

Fixtures are derived without project encoders: Python constructs commitment,
transaction, Merkle and header bytes, while a standalone oracle imports only
`golang.org/x/crypto/argon2` and uses literal production parameters. Production
and cheap-test identities are never interchangeable.
