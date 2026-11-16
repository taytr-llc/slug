# Prospect jobs and birth, D68 rule version 1

Mike approved D68 during change set 18: jobs last exactly five minutes, age
starts at job issuance, and new admissions at or after expiry are rejected
without grace. The reasoning is recorded in `BRAWLCHAIN_SPEC.md`, section 10.
A header timestamp or first submission cannot prove the exact discovery time.
An unpredictable challenge committed before hashing establishes an interval.
Starting age at issuance prevents withholding from making a fighter younger,
at the cost of starting age up to five minutes before discovery.

The APIs and public-input commands implement the rule for an explicitly
configured local authority. The future gym miner obtains/renews jobs and signs
automatically, with no player signing or prospect-selection step. This set
does not ship the end-user miner, wallet/login service, public admission server,
or calibrated target. Jobs support solo and pool work; the official pool is
not a dependency. None of this changes chain consensus or checkpoint policy.

## Immutable job bytes

`prospectjob.Job` has required JSON fields `version: 1`, `rule_version: 1`,
`id`, complete version-1 `context`, `public_key`, `gym_id`, `challenge`,
`issued_at_ms`, and `expires_at_ms`. Hex is canonical lowercase; the public
key is compressed secp256k1. The challenge is 32 random bytes. Time is positive
Unix milliseconds; expiry is exactly issuance plus 300000 without overflow.
Job ID is SHA256 of this concatenation, without JSON or implicit separators:

```text
ASCII "SLUG mining job\0v1\0"
version LE32, rule_version LE32, context.version LE32
network LE32, genesis display32, DNA version LE32, prospect target BE32
parent hash display32, parent height LE32, expected bits LE32
minimum header timestamp LE32, maximum header timestamp LE32
public key33, gym ID32, challenge32
issued_at_ms LE64, expires_at_ms LE64
```

Positive signed 64-bit times encode as unsigned LE64. Job IDs/challenges may
contain zero bytes, including all-zero values. IDs commit every other field.
Validation recomputes the canonical job. Strict JSON rejects duplicate,
unknown, missing or null fields, including nested context; input is bounded
to 8192 bytes. `prospectjob.New` is a deterministic public byte helper, with
no issuer authority, clock, randomness, persistence or private-key access.

`registry.IssueJob` obtains time and `crypto/rand` internally, runs a mandatory
trusted-node context guard, then synchronizes the job before returning it.
Callers cannot supply a time, lifetime, challenge or chosen ID to this storage
API. The CLI resolves context from its configured node. Admission uses the
same authority's stored job, never claimant-provided job fields. Unsigned job
exports are issuer time assertions, not independently authenticated timestamps
or signed league publications. No operator checkpoint key is involved.

## Proof and authorization version 2

Context, gym identity, policy ID, DNA, and prospect-ID derivation are unchanged.
Proof v2 requires `job_id`; v1 JSON forbids it even when empty. The single
reserved zero-value coinbase output is:

```text
OP_RETURN, direct push 40, ASCII "SLUGPR02", commitment32
commitment = SHA256("SLUG prospect job gym\0v2\0" || policy ID32 || gym ID32 || job ID32)
```

Mixed, duplicate, nonminimal or unknown `SLUGPR` outputs still fail. Changing
jobs changes the coinbase and header Merkle root before Argon2id; attaching a
fresh job or signature to old work cannot renew it. All ordinary coinbase,
Merkle, header, target and production-work checks remain. `Verified.Version`
follows proof version; stable prospect IDs remain chain plus work hash.

Authorization v2 uses the set-17 encoding with domain
`"SLUG prospect admission\0v2\0"`, inserting job ID32 immediately after the
proof-version LE32. Authorization and proof versions must match. V1 bytes and
golden vectors remain unchanged. Work/signature verification alone does not
establish issuance or birth; conditional v2 CLI output says `birth_verified:
false`. Only the birth registry enforces the job's provenance and deadline.

## Schema 3 and admission

`CreateBirth` / `init-birth` require a new dedicated directory. No older
registry migrates; schema-1/2 admission remains limited to proof v1. Every
schema-3 receipt contains original authorization, complete job, and `birth`
with `rule_version: 1`, `born_at_ms: job.issued_at_ms`, and `accepted_at_ms`.
Job keys are `job:<lowercase ID>`. Metadata adds `job_count` and `last_clock_ms`
to existing schema fields. Open checks hashes, job/receipt linkage, birth
intervals, counts and indexes; reads reverify signatures and production work.

New admission verifies the stored job, signature, production work, and current
node/policy eligibility. It requires issuance <= final acceptance < expiry.
The final time check runs after hashing and the node guard, immediately before
encoding the bounded atomic receipt batch. Queue/network delays may make a
genuinely mined proof too late. There is no claimant arrival-time override or
offline exception. A synchronous batch stores receipt, index, and metadata;
job issuance is already durable in the same database. Multiple distinct
qualifying proofs may share one job and its birth anchor; jobs are not a cap.

Duplicates are historical receipt recovery. Signature and work are reverified,
but current clock/node checks are skipped; original birth survives expiry,
reorg, restart, policy change and node outage. Output reports
`current_node_checked: false` and `context_source: "historical-receipt"`.
New admissions report true after the guard. Historical output grants no
renewed eligibility or canonicality. No API deletes, re-ages or updates a birth.

For a new valid proof, clock observations are synchronized before expiry/node
rejection; late final observations are also synchronized before expiry failure.
A rejected admission may therefore advance clock metadata only, creating no
birth, receipt or sequence. Reopening cannot forget that later observation.
Observed backward clock movement fails new issuance/admission until the clock
catches up. Historical reads remain available. Uncertain clock/job/receipt
writes poison the handle until close/reopen; errors never prove no commit.
Retrying an accepted proof recovers its original receipt, even after output
failure. Lost issuance output can strand an unused job; issue a fresh one.

This trusts the authority's clock and durable filesystem. It cannot detect an
administrator rewriting or rolling back the entire store, prove global time,
or make a node observation atomic with the write. Reorgs can follow acceptance.
Back up the complete closed directory, retaining jobs and receipts. Exports
contain full job/birth evidence but are unsigned and are not database backups.
Existing platform/synchronization limitations remain. Public service deployment
still needs authentication, TLS, resource limits and bounded admission queues.

## Commands and checks

```sh
slugregistry init-birth --db /path/to/new-registry --network 1397511495 --genesis HEX
slugregistry issue-job-node --db /path/to/registry --policy policy.json --rpc-config rpc.json --public-key HEX
slugregistry get-job --db /path/to/registry --id HEX
slugprospect commitment-job --job issued-job.json
slugprospect auth-payload --context context.json --proof proof-v2.json --public-key HEX
slugregistry record-job-node --db /path/to/registry --policy policy.json --rpc-config rpc.json --proof proof-v2.json --authorization authorization-v2.json
```

No command accepts private keys, `--now`, issue times, lifetime overrides, or a
production default target. Existing `get`, `list` and `export` are read-only.
Deploying binaries creates no live job authority or public service.

Independent vectors cover every job byte, v2 commitment/signature, full 256 MiB
Argon work and DNA. Behavioral tests cover exact/final-check expiry, historical
birth, clock rollback/reopen, multiple proofs, old-work rebinding even with a
fresh valid signature, schema isolation, uncertain writes, process death,
corrupt links and cheap-PoW refusal.
