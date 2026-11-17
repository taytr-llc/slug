# Bounded, resumable candidate recovery (set 32)

This is an explicit wallet schema 11 for the two published D73 candidate
fixtures. It restores the same profile 2 file and root/gym identity in bounded,
restartable steps. Existing schemas 8/9/10 and their creation APIs keep their
previous behavior. Real issuance remains disabled until the permanent profile
and genesis are finalized. This library is not a new desktop onboarding flow.

## Operational contract

`Loader.BeginCandidateRecovery(ctx, publicPass, privatePass, export)` creates
an exclusive pending session. `BeginCandidateFixture` starts a named public
fixture with no issued addresses. `OpenCandidateRecovery` resumes an existing
session with public encryption access; it does not regenerate missing progress.
`Step` performs at most 128 raw address-child derivation attempts or scans one
bounded block. `Status` exposes only counts and checkpoint information. `Close`
cancels and joins accepted work, then releases storage. Pending wallets cannot
open through the ordinary loader, unlock, sign, or spend. After the session
reports ready, close it and use the ordinary loader.

An imported cursor, issued cursor, derived cursor and target cursor are
separate facts. Unused 20-address lookahead does not become issued history.
Discovering a used lookahead address raises the issued cursor permanently,
derives the extension in later batches, then scans earlier blocks again so an
older payment to a newly covered address is not missed. Reorg repair retains
address facts and rebuilds the private transaction view from genesis. Script
rows, transaction effects and progress commit together. An uncertain write
poisons the handle and requires reopening; a rejected notification never
makes an allocated address reusable.

The aggregate ceiling is 65,536 child attempts across all eight branches,
including lookahead. Ready allocation also reserves 20 future attempts per
branch so an accepted exported highwater remains restorable within the same
quota. Full nonhardened cursors remain valid profile data; exceeding the
runtime quota is an explicit resource refusal before creating a directory.
There is no quota override or silent truncation.

Schema 11 also checks stored master/cipher/sync field sizes before copying or
key derivation. Its scrypt parameters require a power-of-two N greater than 1 and no
larger than 2^18, r from 1 through 8, and p equal to 1. Creation and password
rotation enforce the same ceiling, including the existing default parameters;
older schemas retain their prior behavior. A recorded invalid child must be
independently proven invalid from the public account key at that exact index,
so a forged skip cannot hide an otherwise valid address.

The scanner additionally bounds retained relevant transactions to 16,384,
historical owned outputs to 16,384 and serialized relevant transactions to 32 MiB.
Limits are checked before a batch commits. The scanner audits persisted script,
ledger and progress crosslinks on reopen. Raw transaction/credit/debit/block
row sizes and counts are checked before decoding, and exact references and
cached balances are reconciled with the recovered transaction indices. Its completion check binds the exact
canonical height and hash immediately before marking ready. This is a trusted
full-node-assisted recovery operation, not an independent consensus verifier or
a permanent finality guarantee.

## Node source and live wallet boundary

`RecoveryRPCSource` is read-only. Each operation opens one WebSocket and verifies
network magic, genesis and the Phase-A activation 62 settings on that same
connection before reading data. It never reconnects under a previous socket's
identity result. Plaintext requires literal loopback; TLS verifies certificates
and hostnames. Requests have a 30-second deadline, bounded upgrade/frame sizes,
strict response IDs and cancellation that closes the underlying socket even
during TLS or HTTP upgrade. Configuration and errors redact credentials.

The session processes one normal 256 MiB Argon2id header hash at a time, checks
Merkle/witness binding and rechecks canonical identity around scans. Its source
cannot mine, submit transactions or select a claimant-provided chain identity.
The real integration uses a disposable peerless loopback node only.

A ready wallet still performs O(total supported inventory) startup and watch
work. Registration is chunked to 128 addresses; watched outputs are bounded to
16,384 and a rescan request must encode at most 8 MiB. Capacity/watch failures keep
schema 11 unsynced and prevent ordinary transaction creation and PSBT spending.
These limits do not promise arbitrary future history or constant-memory live
sync. Maximum-inventory acceptance records sampled wallet heap during ready address
enumeration and watch registration, after creation/open and garbage collection.
That sample excludes creation/KDF peaks and is not total application RAM.
Request sizes/calls and node RSS are measured separately; node RSS is not a
Go-heap measurement. The measured
watch workload uses a genesis-only disposable node with no mined payments; the
separate funded recovery acceptance supplies the ledger/spending evidence.

## Recovery and release boundaries

Exports retain the unchanged version 2 format and take current issued cursors
from one committed snapshot under the existing private-material lease. They
cover that snapshot, not addresses issued afterward or an arbitrarily stale
backup. The hosted fixture vault still stores an immutable legacy snapshot;
this set does not silently claim that it follows schema 11 cursors.

Independent public vectors cover all four payment types and both branches at
4096, 4097, 4116, 4128, 8191, 8192 and 65535 for both fixtures. Acceptance combines
cancellation, subprocess termination, storage corruption, chronological
lookahead expansion and reorgs with an actual high-index export/restore/spend
under production PoW. See `CHANGESET_32_NOTES.md` for the final measured evidence.

D74 still requires a thorough joint design review with Mike before a friends,
tester or public product release. Engineering validation and seed deployment
do not satisfy that review or authorize production-wallet issuance.
