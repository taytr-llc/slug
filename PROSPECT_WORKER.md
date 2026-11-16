# Automatic prospect worker

`prospectworker` connects complete payable templates, production Argon2id,
automatic gym authorization, D68 job renewal and D69 block delivery. It is a Go
library for the desktop/account integration. It neither reads private keys nor
provides a hosted wallet, login or full wallet export.

## Explicit setup

Provide a version-1 worker `Config` with the exact prospect-node policy,
compressed public gym key, network-encoded payout address, authority origin and
positive retained-record/byte limits. The authority must use service protocol
v2 and a separately initialized schema-4 registry. The worker verifies the
actual client's protocol and normalized origin against its configuration.
RPC and service credentials remain in their separate clients, outside the
public outbox. No target, payout, authority or key is silently defaulted.

Create a new outbox with `CreateOutbox`, or explicitly reopen it with
`OpenOutbox`. There is no automatic creation, migration, repair or deletion.
The store binds chain, policy, gym public key, payout and authority. It uses an
exclusive LevelDB lock and synchronous writes. Capacity increases are explicit;
they do not change identity. Stop the worker and close the store before copying
its complete directory for backup.

Pass the clients, in-memory `Signer`, and outbox to `New`. Run `Run(ctx)` once;
cancel its context to stop and await its return before closing dependencies.
A worker instance cannot restart; a new instance can reuse the open outbox.
`Status()` is safe to read concurrently and separates recovery errors from the
current mining operation. `Outbox.List(after, limit)` pages public evidence,
at most 100 entries and 32 MiB of encoded entries per call. Continue after the
last returned sequence until an empty page. It is not a wallet export.

## Mining and delivery

The worker obtains a five-minute job, resolves the complete template, builds a
payable candidate and asks the node to validate its proposal. It checks the
parent periodically and renews early to leave delivery time. The authority's
final clock controls admission; local timing cannot extend the job. Nonce and
extra-nonce rollover are checked. Default builds use the production 256 MiB
Argon2id hash; `slugtestpow` cannot initialize or run this worker.

One sequential loop owns hashing and delivery. Synchronize the full scan intent
once, then advance a small durable cursor while hashing. Atomically retain
qualifying work with its cursor advance. A crash
can repeat an unfinished attempt, but cannot turn it into a younger job. A stop
requested during an uncancellable hash waits until found evidence is durable.

For each found result:

1. Preserve its public job, exact block/proof and computed work hash.
2. Broadcast a block-target win before waiting for the signer or authority.
3. Observe the exact canonical block separately; a submission reply alone does
   not establish canonicality. A block can qualify even if it misses a stricter
   prospect target.
4. For qualifying prospects, request the automatic signature, verify it, and
   synchronize the exact public authorization before submitting admission.
5. Preserve the complete validated receipt or the unresolved delivery state.

Block and prospect delivery have separate state. Retries use the same work,
job and authorization, including historical recovery after expiry. They never
request a replacement signature to reset age. Uncertain delivery remains
recoverable. Reconciliation is scheduled with bounded backoff in the same
loop so an unresolved block does not indefinitely prevent fresh mining.

## Storage and operational limits

The outbox stores public evidence only: no API token, RPC credential, wallet
private key or operator checkpoint seed. Records and logical encoded bytes are
bounded, including capacity reserved for future authorizations and receipts.
LevelDB journal/compaction overhead is additional filesystem usage. Exhaustion
pauses new mining while preserving recovery evidence. Accepted receipts count
toward retained capacity; there is no automatic pruning.

An uncertain synchronous write poisons the handle. Stop, close and explicitly
reopen before retrying; the worker never assumes the write rolled back. Open
checks marker/schema, indexes and structural linkage. Sending recovered work
requires production hash verification. Stored canonicality is an observation,
not permanent finality, and later chain changes do not rewrite original birth.

This worker does not start on the Railway seed just because its code is shipped.
The pilot still needs the desktop lifecycle/account adapter and a calibrated
prospect policy. Public fixture keys and easiest-target tests are for disposable
verification only; they are not production wallet or policy defaults.
