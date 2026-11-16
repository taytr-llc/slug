# SLUG set 8 PoW swap security audit (pre-fix baseline)

Fixes, verification, and the current residual-risk disposition are recorded
in `../../CHANGESET_8_NOTES.md`. This baseline preserves the original evidence.

Reviewed 2026-11-15 in `/Users/taytrrip/slug/slug`, beginning at the set 6
implementation `df655087` with set 7 report-only checkpoints in progress.
Set 7 subsequently landed as `6b3e9e90`; the main agent is applying fixes on
`slug-pow-security`. **This document describes the original findings, not a
claim that those later fixes have passed verification.** Line numbers refer
to the inspected baseline and may move. This agent made no production edits.

The two high-priority findings are (1) per-peer hashing before serialized
validation permits parallel 256 MiB allocations, including for unsolicited
empty invalid blocks; and (2) a repeated known header costs Argon2 again,
allowing one headers batch to monopolize the sync manager for minutes.
Existing limits bound some counts, but do not establish an acceptable total
CPU or active-hash-memory budget. The restart optimization has a defensible
trusted-local-database boundary, with specific corruption-handling gaps.
No attacker-selectable Argon2 memory/time parameter was found.

## Method and scope

Read AGENTS.md, HANDOVER.md set 8, the PoW implementation, P2P receipt and
logging, netsync message/request/timeout paths, header/block validation,
orphan handling, storage/restart/migration, target arithmetic, and mining/RPC
hash consumers. A second agent independently traced stored-index trust and
parameter handling; its detailed evidence is preserved in
`stored-index-review.md`.

The complete broad caller search is saved in
`block-hash-callers.txt` (224 lines, including transaction Hash
calls and method names that are not computations). The classified inventory
below accounts for production block/header computations and identifies
test/tool-only paths. There is no separate BlockHeader.Hash method.

One bounded benchmark computed **three** mainnet genesis hashes and checked
their exact configured identity. `/private/tmp/slug-set8-pow-measure.go` ran
on this arm64 Mac with Go 1.27.4 and GOMAXPROCS=4. It measured 650.086 ms total,
216.695 ms/hash, and 805,313,504 allocated bytes total (268,437,834/hash).
These are local observations, not universal performance promises. Attack
duration estimates below are arithmetic extrapolations, not load tests.
No adversarial traffic/load, production key access, deployment, or network
scan was performed.

## P1: unauthenticated peers can allocate concurrent Argon2 arenas

**Severity: high availability.** Successful P2P handshakes and enough peer
slots are the exploitation conditions; valid PoW is not required.

Before the fix, `serverPeer.OnBlock` (`server.go:724-747`) wraps the message,
then calls `block.Hash()` to mark peer inventory **before** QueueBlock and
its processing wait. Each peer invokes this on its own input goroutine.
`netsync.handleBlockMsg` only later checks the peer/requested-block maps
(`netsync/manager.go:692-714`), after the hash is already computed.

`wire.MsgBlock.BtcDecode` permits a zero transaction count
(`wire/msgblock.go:79-126`). An 80-byte header plus one zero count byte is a
syntactically decoded block message that reaches this hash, despite being an
invalid block. Including the 24-byte v1 network header
(`wire/message.go:17-20`), 105 bytes can trigger a 256 MiB arena.

The default max peers is 125 (`config.go:46`), with eight default outbound
targets (`server.go:55-56`). Thus 117 normally available inbound slots could
trigger 29.25 GiB of simultaneously active arenas; 125 hashes total would be
31.25 GiB, before GC and other memory. This is a concurrency upper scenario,
not a measurement. Fewer peers suffice on memory-limited nodes.

The pending-handshake prefix limit of eight and v2 responder token/concurrency
limits (`internal/inbound/admission.go:23-46`) do not protect established
peers: the reservation is released after handshake (`server.go:388-394,640`).
QueueBlock's per-peer wait and the single sync-manager goroutine serialize
later validation, not this prequeue hash.

**Minimal fix:** remove Hash/AddKnownInventory from OnBlock; leave its
QueueBlock/wait backpressure intact. In serialized handleBlockMsg, after the
known-peer check, compute the identity once and call
`peer.AddKnownInventory(wire.NewInvVect(wire.InvTypeBlock, blockHash))` before
the existing requested-block check. AddKnownInventory is available on the
concrete peer (`peer/peer.go:547-551`) and already used by netsync
(`manager.go:1158`). Never hash again after rejection back in OnBlock.

Also remove the raw `MsgBlock.BlockHash()` from the debug summary
(`peer/log.go:178`). Both read and write logging lazily invoke that summary
on individual peer goroutines (`peer/peer.go:1138,1185`). Default INFO avoids
this second vector, but enabling DEBUG must not restore it. Log header
fields/transaction count instead.

**Independent invariants:** an empty/unrequested block never hashes in a
peer callback/log summary; rejected blocks retain the existing disconnect
rule; requested accepted blocks still mark known inventory; a peer cannot
queue a second block while its first is unfinished. This closes the observed
P2P parallel-hash path, not every authorized RPC/miner concurrent allocation.

## P2: replayed headers monopolize serialized processing

**Severity: high availability.** One connected peer can send known valid
headers, with no fresh mining cost.

`wire.MaxBlockHeadersPerMsg=2000` (`wire/msgheaders.go:14`); a full headers
payload is 3 + 2000*81 = 162,003 bytes (the declared maximum permits a
9-byte count, 162,009 bytes; `:135-138`). OnHeaders simply QueueHeaders
(`server.go:789-791` baseline), which enqueues without waiting
(`netsync/manager.go:1527-1535`). The manager channel capacity is MaxPeers*3
(`:1634`), 375 by default.

`handleHeadersMsg` accepts any tracked peer, loops the entire batch without
an adjacency check or incremental budget (`manager.go:956-1000`), and calls
ProcessBlockHeader. `maybeAcceptBlockHeader` first resolves a known valid
parent, then computes Argon2 **before** checking whether that header is
already indexed (`blockchain/accept.go:126-162`). Sending 2,000 copies of one
known non-genesis header therefore costs 2,000 hashes. The local measurement
extrapolates to 433.39 seconds (7.22 minutes), with 500 GiB of cumulative
arena allocations, sequential rather than peak. A 375-batch queue represents
750,000 hashes, about 45.14 hours at that measured rate; this is an estimate,
not a demonstrated sustained attack duration.

A newly accepted header is hashed at least four times: initial identity
(`accept.go:145`), proof (`validate.go:393`), checkpoint context
(`validate.go:814`, even with zero legacy checkpoints), and new blockNode
(`blockindex.go:120`, called at `accept.go:184`). Extra median-time consumers
can add work. Full new valid batches are consequently expensive even without
malice; repeated known data is the attacker-asymmetry case.

The manager's three-minute stall threshold and 30-second ticker
(`manager.go:43,47,452-477`) run in the same goroutine as the batch
(`:1300-1368`), so cannot interrupt it. Peer-level stall logic extends
deadlines while callbacks run (`peer/peer.go:1299-1407`). Existing ban-score
paths do not impose a header hash budget. Count-bounded channel memory does
not bound time-to-service for blocks, transactions, or disconnect messages.

**Fix recommendations and compatibility:**

- Add per-peer QueueHeaders completion backpressure, with shutdown/disconnect
  escape, so one peer cannot fill the entire shared channel. Alone, this
  limits queued messages, not per-batch CPU or active-header memory.
- Deduplicate by canonical serialized 80-byte header, with a bounded cache;
  reject duplicate/discontinuous sequences cheaply after each predecessor's
  identity is known. Do not use time.Time structural equality as a substitute
  for canonical wire bytes. Cache identity rather than a permanently valid
  decision: later body validation can mark an accepted header's ancestry
  invalid. Current invalid-ancestor/status checks must remain authoritative;
  ignored cached data must not falsely count as new progress/reset stalls.
- Reuse a locally computed header identity through internal validation/node
  construction rather than independently computing it four times. Never
  accept a peer-supplied hash as the cache/validation result.
- Preserve legitimate 2,000-header getheaders replies. The protocol request
  has a locator and stop hash, **no requested count** (`peer/peer.go:979`), so
  lowering a local accepted batch count breaks normal peers. If bounding a
  processing turn, process a small chunk and retain its remainder for fair
  scheduling, rather than reject an otherwise valid long response.
- `startSync` sends getheaders at `manager.go:288`; continuation sends at
  `:1001`. Track outstanding requests if enforcing request state, but preserve
  spontaneous headers after IBD (explicitly documented at `:953-955`). During
  IBD, restricting bulk replies to the selected requested sync peer avoids
  arbitrary peers doing bulk work; outside IBD, an unrequested short announcement
  may trigger a requested continuation. Count policy must allow legitimate
  multi-header reorg announcements and must be documented/tested rather than
  silently assuming every unrequested batch is malicious.

A small cache alone is not a total CPU bound: a peer can cycle through more
known history than fits, or supply new valid headers. Queue backpressure alone
does not bound one admitted batch. Fair scheduling/rate or work admission
must be addressed explicitly, or listed as a remaining availability risk.

## Target, parameter, orphan, and request bounds

`checkProofOfWork` (`blockchain/validate.go:374-405`) decodes Bits, requires a
positive target no greater than PowLimit, then computes and compares the
header hash. HashToBig reverses the little-endian hash consistently
(`blockchain/internal/workmath/difficulty.go:25-37`). Compact exponent is an
8-bit field, so worst target arithmetic is only about 2,039 bits; it cannot
request an enormous integer allocation. However, the initial identity hash
in header acceptance happens before these cheap target checks. Header sanity
also checks timestamps after proof (`validate.go:498-536`). Reordering cheap
rejections must preserve consensus error/flag behavior and be tested.

The sole production Argon2 call is `wire/blockheader.go:91-92`. Time=1,
memory=256*1024 KiB, lanes=1, output=32 are unexported compile-time constants
(`:53-67`); password and salt are the same fixed 80-byte canonical header
(`:84-89`). Header Bits/nonce/version/time do not alter cost parameters. The
selected x/crypto implementation allocates 262,144 blocks of [128]uint64,
exactly 268,435,456 bytes for each main arena. No network/RPC/config path to
select memory, passes, lanes, or salt length was found. This bounds one call,
not concurrent calls or cumulative work.

The orphan block pool caps at 100 with one-hour lazy expiration
(`blockchain/chain.go:23-26,295-337`). MaxBlockPayload is 4,000,000 bytes
(`wire/msgblock.go:27`), so 100 bodies alone can represent 400,000,000 bytes
(381.5 MiB), before decoded objects/cached serialization. ProcessBlock holds
the chain lock, hashes and sanity-checks before orphan insertion
(`blockchain/process.go:143-170,214-229`). Thus the count cap protects retained
bodies, not hash admission; valid requested orphan bodies require PoW, which
is materially different from the free invalid prequeue vector above. Fresh
wrappers recompute their cached identity; cache scope is only one block object.

Requested-block/transaction maps use the 50,000 inventory limit
(`netsync/manager.go:33-39`). `minInFlightBlocks=10` (`:27`) is a refill
threshold, not a ten-block hard cap: buildBlockRequest can request up to
50,000 (`:887-949`). These bounds do not establish hash CPU/memory budgets.
An attacker cannot use unknown-parent headers to cheaply fill the header
index: acceptance rejects absent parents before hashing, and novel nodes
require proof/context checks. The accepted header index itself has no total
node cap; work-backed side branches are retained, with easier Phase A initial
difficulty relevant to cost. No unbounded header-index stress test was run.

## Restart trust and corruption handling

The intended trust boundary is the local database, which already controls
UTXOs, validation statuses, and the best-tip pointer. Fresh blockNode creation
hashes locally (`blockchain/blockindex.go:119-120,154-157`), and persistence
keys are that node identity (`chainio.go:1452-1469`). The only special
production initBlockNodeWithHash caller is the loader (`chainio.go:1311`).
The btcutil hash cache is private with no exported hash setter
(`btcutil/block.go:96-105`). No remote arbitrary stored-hash injection path
was identified.

Correct the claim that every stored index entry has passed full validation:
`maybeAcceptBlock` stores the block/index **before** transaction/script
connection (`blockchain/accept.go:57-89`). Losing side-chain and failed-connect
records may persist. The trustworthy property is **locally computed header
identity**, with validation status tracked separately.

The loader checks 36-byte keys, first key's genesis identity, parent existence,
and stored tip existence (`chainio.go:1277-1305,1319-1324`). It does not
authenticate an imported or locally modified database. CRC32C/network-magic
and LevelDB strict checks catch ordinary corruption, not hostile local writers
(`database/ffldb/blockio.go:557-575`, `db.go:2135-2142`). Header-only nodes are
not persisted (`blockindex.go:524-553`), so restart discards their sync work;
this can cause costly revalidation but is not a bypass.

Specific local-corruption findings (detailed in the companion report):

- **ST-1, medium operational/low security:** chainio.go:1222-1226 swallows a
  migrateBlockIndex error by returning nil; New later dereferences incomplete
  state. Return the error and test a forced migration failure without mining.
- **ST-2, medium recoverability/low security:** upgrade.go:105-110,156-169
  slices legacy row/key bytes without length checks; graph traversals at
  :203-230 assume nodes/parents exist and have no cycles. Malformed local
  metadata can panic or loop. Reject malformed lengths/missing links/cycles
  with corruption errors; no network route to these keys was found.
- **ST-3, low hardening:** loader ignores encoded key height, does not compare
  genesis row's header to configured genesis, and does not cross-check stored
  best height/work with the reconstructed tip (chainio.go:1277-1326). Row
  decoding accepts trailing bytes/unknown status bits (:1385-1399). Add cheap
  consistency checks where backward compatibility permits; they do not turn
  local storage into authenticated input.
- **ST-4, low documentation:** verifychain (rpcserver.go:3723-3755) rechecks
  block sanity/PoW but does not compare computed header identity with the
  index key or copied index header and ancestry. Do not call it a complete
  integrity audit of the optimized restart boundary. Consider an explicit
  offline/maintenance audit, not mandatory Argon2 rehash on every startup.

## Classified hash caller inventory and remaining cost assumptions

Direct production header/block computations (baseline):

| Site | Purpose and assessment |
| --- | --- |
| wire/blockheader.go:84,91 | Fixed-parameter consensus digest; no attacker cost selector. |
| wire/msgblock.go:314 | Raw block delegates to header; no cache. |
| btcutil/block.go:103 | Lazy private per-wrapper cache; repeated callers on the same immutable wrapper reuse it. |
| blockchain/accept.go:145 | Initial header identity; repeated-known-header finding P2. |
| blockchain/validate.go:393 | Proof target check, cost fixed but repeated identity work. |
| blockchain/validate.go:814 | Legacy checkpoint context hashes even with no checkpoints; report-only signed records never enter this path. |
| blockchain/blockindex.go:120 | Node identity constructor; called by accept.go:77,184, chainio.go:1079 (genesis), thresholdstate.go:159, validate.go:1425. |
| blockchain/thresholdstate.go:156 | Unknown-parent error formatting hashes; normal PastMedianTime builds a hash-computing node at :159 merely to obtain timestamps/ancestry. Avoidable computation. |
| btcutil/gcs/builder/builder.go:301 | Correct compact-filter hash key, but rehashes raw block despite caller's cached identity (cfindex.go:220); avoidable extra hash per filter. |
| peer/log.go:178 | Read/write DEBUG summary on peer goroutines; P1. |
| mining/cpuminer/cpuminer.go:278 | Intended nonce loop; one 256 MiB arena per worker. Default runtime.NumCPU workers (:48,377-388), operator-controlled. |

Cached block.Hash consumers were traced by category:

- Network admission: server.go:730 and manager.go:701 are P1. Later manager
  logging, block notifications/inventory/fee rollback (manager.go:841,861,
  1404,1475) use already processed wrappers.
- Validation and connection: process.go:114,149; accept.go:72;
  scriptval.go:309; chain.go:268,278,336,669,676,775,806,1045,1049,1254;
  chainio.go:463,473,1475; utxoviewpoint.go:333. Most use the same cached
  wrapper. Reorg detach at chain.go:1045 intentionally verifies loaded block
  identity against the selected index node and must preserve that check.
- Index/database consumers: indexers/{manager,txindex,addrindex,cfindex}.go
  (all sites retained in the raw inventory); ffldb/db.go:1166;
  mempool/estimatefee.go:252. DB fetch creates fresh wrappers
  (chainio.go:1175-1199), so a later Hash can recompute despite a known lookup
  key. Copying a stored key into a new unverified block's hash cache would
  expand the trust boundary and must not bypass detach/integrity checks.
- RPC: accepted submitblock logging rpcserver.go:3666 and notification :4902
  use processed blocks. verifychain :3748 may hash freshly fetched wrappers;
  RPC WebSocket connected/disconnected/filtered block/rescan paths
  rpcwebsocket.go:672,696,948,2793 also use cached Hash, with fresh DB wrappers
  possible during rescans. These authorized operations may run concurrently.
- Miner acceptance/output: cpuminer.go:195,615 uses block wrappers; the nonce
  loop itself uses raw Header.BlockHash. Miner telemetry still counts two
  hashes per nonce and says SHA256 twice (:275-279), overreporting Argon2 hash
  throughput by 2x. Low priority, no consensus effect.
- Local tools: cmd/addblock/import.go:107,
  cmd/findcheckpoint/findcheckpoint.go:105, blockchain/checkpoints.go:205,
  database/cmd/dbtool/insecureimport.go:123. Their fresh-wrapper work is
  operator initiated rather than concurrent unauthenticated P2P work.

Fullblocktests/generate.go's 17 raw hash sites, internal/testhelper/common.go,
integration/rpctest/blockgen.go, and rpcclient/examples/btcdwebsockets/main.go
are test generators/examples, not daemon receive paths. They retain expensive
grinding/rehashing, which belongs to set 9 debt. Remaining broad-search Hash
matches are transaction/coin hashes, chainhash value constructors, or method
names such as GetBlockHash/FilterByBlockHash. Txids, merkle trees, sighashes,
and address checksums remain separate chainhash algorithms.

## Residual risks and unreviewed areas

- Closing P1's peer path does not impose a global memory budget on operator
  mining, authenticated verifychain/rescan RPC, and P2P processing together.
  Default HTTP clients=10, WebSocket clients=25 (config.go:50-51), with 20
  concurrent request slots per websocket (rpcwebsocket.go:2036); these are
  connection/request counts, not hash-memory budgets. RPC authentication is
  required, and the documented Railway seed does not expose RPC. Template
  checks use chainLock.Lock (validate.go:1391-1426), so are serialized with
  chain validation; BFNoPoWCheck still leaves identity work elsewhere.
- Novel valid headers and bodies necessarily cost 256 MiB per proof. An
  explicit fair admission/throughput policy remains a deployment concern if
  replay fixes/backpressure are the only mitigations. Header-index growth and
  restart revalidation under sustained work-backed forks were not load tested.
- This source review does not prove Argon2 cryptographic security, difficulty
  economic adequacy, all consensus/script correctness, or whole-project
  absence of vulnerabilities. It did not review full UTXO/spend-journal
  migration history, LevelDB internals, disk/fsync fault injection, hostile
  snapshot tooling, or arbitrary applications mutating exported block data.
- No full blockchain/integration suite pass is claimed. Set 9 remains open.
  Fixes to set 8 findings require their own tests/review and smoke verification;
  this baseline report alone is not permission to mark the pre-genesis gate
  complete.
