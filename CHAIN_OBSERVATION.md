# Atomic chain observation v1

Set73's read-only boundary observation is separate from consensus, signed
checkpoints and ownership replay. It lets a caller distinguish a stable node tip
from an A→B→A sequence with the same endpoint hash. It does not establish global
network agreement, wall-clock truth, publication time or final ownership.

`getchainobservation` takes no parameters. Its ordered result is
`{"version":1,"instance_id":"<64 lowercase hex>","revision":"<decimal>","hash":"<display hash>","height":0}`.
The public instance ID is a fresh nonzero32-byte random value for each concrete
RPC server/chain lifetime, never a key. Revision is canonical unsigned decimal
text in1..18446744073709551615, without leading zeros. Height is a nonnegative
int32. Hash uses existing chainhash display order. Revision text avoids losing
precision in JavaScript consumers. Normal RPC error framing remains unchanged.

The chain initializes revision1 after startup. Each successful best-state
replacement during connection or disconnection advances it under the same state
lock, including every detach/attach of a reorganization. Invalid, duplicate or
side-chain-only acceptance does not change it. No revision is persisted or
inserted into a block, database schema, fork choice or checkpoint rule. A
restart has a fresh RPC instance even at an identical tip/revision.

The getter uses fail-fast `chainLock.TryRLock` then `stateLock.TryRLock`, copies
hash/height/revision, and releases both before returning. It never waits for
normal hashing, DB work or a caller's deadline. Contention returns unavailable;
no fallback to an older snapshot is allowed. The chain lock excludes the gap
between the canonical tip update and snapshot publication. The copied result
does not expose a mutable internal BestState pointer.

When the uint64 revision cannot advance, mark this observation epoch permanently
unavailable. Continue accepting consensus-valid blocks and updating BestState;
never wrap the revision or make observation exhaustion a consensus error.

A noon coordinator separately obtains the original source publication/admission
permit, then two observations from its original owned node. The first response
must finish before noon and the second request begin at/after noon; require the
same instance, revision, hash and height, no original-owner disconnect/change,
monotonic clock ordering, each call within5seconds, and the whole bracket within
10seconds. Ten seconds is a failure ceiling; schedule reads close to noon.
Different revision detects an intervening A→B→A. Advancing tip, missed boundary,
contention, restart or missing durable intent means unavailable, not an invented
historical owner. Mining is never paused. Persist the exact bracket and source
frontiers before delayed replay; those higher-level bytes belong to the separate
ownership contract. Node observations alone grant no cutoff or transfer authority.

Qualification must cover stable repeats, real connect/disconnect/reconsider,
same-hash return with changed revision, concurrent reads, overflow while normal
chain state continues, fresh RPC lifetime IDs, no-parameter command/ordered JSON
and existing help generation. Actual isolated-node acceptance and normal smoke
remain required before any rollout. No production checkpoint key is involved.
