# Pool work validation — set 52

This first pool component is an additive immutable adapter in `prospectwork`.
It prepares candidate manifest work using the existing `BuildManifest` and
checks submitted nonces against three separately retained targets. It does not
change the miner, node, desktop, job format or D68/D69.

## API and ownership

`PrepareManifestWork(job prospectjob.ManifestJob, template Template,
payoutAddress string, extraNonce uint64, poolTarget string)` returns
`(*ManifestWork, error)`. The pool target is a nonzero, exactly 64-character
lowercase hexadecimal integer, at most 256 bits. It never replaces the job's
prospect target or the block target encoded in `Bits`. Targets are classified
independently; a block win must remain visible even if another target is missed.

Preparation calls the unchanged `BuildManifest`, retaining its owned,
nonce-zero candidate and original job. It performs no header PoW. The caller
must obtain the job and template from its trusted issuer/node and choose the
payout destination from its own configuration. Successful construction proves
their existing byte relationships, not that they were issued or are current.

`Job()`, `PoolTarget()` and `Candidate()` expose the retained preparation.
Returned candidate slices are independent copies; mutating inputs or outputs
cannot alter later checks. A zero or nil work object exposes only zero values.
There is no caller-supplied Candidate, proof, hash, target, timestamp, gym or
coinbase argument at checking time.

`CheckNonce(ctx context.Context, nonce uint32) (ShareCheck, error)` changes only
the four little-endian nonce bytes at offsets 76–79 of the retained 80-byte
header. It uses normal `wire.BlockHeader.BlockHash` and interprets the digest
with the chain's existing little-endian numeric convention. Each target check
is inclusive (`hash <= target`). `ShareCheck` contains `JobID`, `GymID`, `Nonce`,
`Header` (hex), `WorkHash` (the conventional displayed block hash),
`DuplicateKey`, and the independent `Pool`, `Prospect`, `Block` booleans.
If at least one target is met, `Candidate` points to an owned nonce-adjusted
candidate, including the corresponding full block and stripped-coinbase proof;
otherwise it is nil. No transaction, payout, merkle path or other header field
changes. An error returns a completely zero `ShareCheck`.

The duplicate key is lowercase hex SHA256 of the exact concatenation:

```
ASCII "SLUG pool submission" || 00 || ASCII "v1" || 00 || header[80]
```

It excludes pool target, session and receipt time. Reassigning a pool target
cannot give identical work a new duplicate identity. This is a stable key for
the future durable ledger, not a claim that duplicate acceptance is prevented
by this stateless component.

## Resource and authority boundaries

All adapter checks share one nonblocking hash slot. Contention returns
`ErrShareBusy`, bounding this adapter to one active normal Argon2id invocation;
other node/miner consumers of `BlockHash` are outside this bound. Check caller
cancellation before admission and before/after hashing. The noninterruptible
hash finishes synchronously before a cancelled call returns. There is no
background hash, accepted queue or hidden retry. Existing exact-header cache
hits may avoid the Argon computation, but never replace target checks.

Reject the `slugtestpow` profile in both preparation and checking. Nil contexts
and unprepared work are invalid. The helper's errors are ordinary Go errors,
not a new network protocol. The implementation introduces no clock, database,
listener, wallet, framework dependency, signing operation or payment.

Successful checking means only that these exact bytes meet these retained
targets. The eventual pool service must independently enforce original-session
authority, durable issuance and duplicate handling, D68's strict expiry, current
parent or exact canonical-winning-block eligibility, and durable acceptance.
It must broadcast block wins before fighter admission under D69. This adapter
does not claim fighter birth, ownership, node acceptance, spendability, account
credit or payout. D79 accounting and its separate confirmation/maturity gates
remain in `POOL_ACCOUNTING_POLICY.md`.

Before runtime implementation, freeze small independent expected vectors from
the existing manifest-candidate reference, changing only nonce bytes. Derive
normal Argon2id and duplicate hashes independently, and cover target equality
and either side of it. Then check ownership, profile refusal, cancellation and
hash contention without generating expected values from the adapter itself.
