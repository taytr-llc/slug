# Gym-key authorization, version 1

Set 17 binds a self-certifying gym ID to a secp256k1 public key and authorizes
one action: admit a specific prospect proof. The client signs automatically;
there is no player-facing signing ceremony or extra prospect-selection step.
This is a protocol component for the future wallet/account client, not an
implemented login service. Key possession alone proves neither a person's
identity nor account existence, exclusive ownership, team affiliation, or
control of coins. D25/D48 wallet integration and key recovery must retain the
same dedicated key or its derivation metadata; no HD path is invented here.

## Exact bytes and scope

Use the existing secp256k1 ECDSA implementation. Public keys are canonical
33-byte compressed points. Signatures are strict DER, low-S, and must exactly
equal their parser's canonical serialization; trailing bytes are rejected.
All hexadecimal inputs decode to bytes, so hex case does not change identity.

The gym ID is SHA256 of the following concatenation:

```text
ASCII "SLUG gym identity\0v1\0"
network: unsigned 32-bit little endian
genesis: 32 decoded display-order bytes
public key: 33 canonical compressed bytes
```

The admission digest is SHA256 of:

```text
ASCII "SLUG prospect admission\0v1\0"
policy ID: 32 bytes, as defined in PROSPECT_PROOFS.md
derived gym ID: 32 bytes
public key: 33 canonical compressed bytes
proof version: unsigned 32-bit little endian
header: 80 serialized bytes
coinbase byte length: unsigned 32-bit little endian
coinbase: exact stripped transaction bytes
Merkle sibling count: unsigned 32-bit little endian
each sibling: 32 decoded display-order bytes, in supplied order
```

Policy ID already binds network, genesis, DNA version, and prospect target.
The proof gym ID must equal the derived ID. The header commits parent, bits,
timestamp, and coinbase/Merkle root; the coinbase commits the gym and policy.
Changing any signed proof field invalidates the authorization. The current
node's moving timestamp bounds are verifier assertions, not signed metadata.
Signatures authorize no transfer, payout change, branding, general account
action, or future proof. Identical authorized submissions may be relayed;
stable prospect-ID deduplication prevents allocating a second receipt.

An authorization JSON object contains exactly `version`, `public_key`, and
`signature`. The parser rejects unknown, duplicate, missing, null, trailing,
oversized, or malformed input. `gymauth.SignAdmission` accepts an in-memory
client key and creates this action-specific authorization; it performs no
file or network I/O. It is not a work verifier. `VerifyAuthorization` checks
the key binding and signature before the production work verifier runs.
Full coinbase, Merkle, and Argon2id checks still belong to `prospect.Verify`.

## Durable authenticated receipts

`registry.CreateAuthenticated` / `slugregistry init-auth` create a new,
explicit **schema-2** directory. Every record has receipt `version: 2` plus
the original `authorization` object. Raw proof, context, verified result,
signature, index, and metadata enter one synchronous atomic batch. Reads
reverify the signature and production work; opening checks signatures and
structure without rehashing all history. Replays preserve the first receipt,
including its signature and context. Uncertain writes require close/reopen
before retry, as in schema 1.

Evidence-only admission methods cannot write schema 2. Authorized methods
cannot write schema 1. There is no automatic migration, key enrollment onto
old opaque gym IDs, or retrospective authorization. Schema-1 bytes and
semantics are unchanged; older binaries reject a schema-2 marker. Copy a
closed complete directory for backup. A local administrator can still roll
back storage; signatures do not authenticate local sequence numbers, supplied
node context, or completeness of an export.

Public-input commands are available for gym-ID derivation, digest preparation,
signature verification, and authorized admission. `verify-auth-node` and
`record-auth-node` resolve trusted context and recheck the node after work,
before success or persistence. Their `authentication: "gym-key-control"`
label describes the verified action only. Stored proof verification remains
`supplied-context`; no signature implies permanent chain canonicality.

CLI tools never request or read private keys. Tests use public, deterministic
fixture keys, independent Python secp256k1/RFC6979 and byte encodings, the
standalone Argon2id oracle, and independent DNA derivation. Such keys and the
all-ones test target must never be used for real accounts or production policy.

Birth/freshness rules, actual wallet/login integration, ownership and transfer
state, reorg history, target calibration, and publication remain separate
work. Deploying the commands does not activate a public admission service,
create a live registry, or configure a production target.
