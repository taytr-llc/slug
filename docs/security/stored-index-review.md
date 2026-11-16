# SLUG set 8: stored-index trust and Argon2 parameter audit

Source inspected: `/Users/taytrrip/slug/slug`, set 7 worktree stacked on
`df655087`, 2026-11-15. This was a read-only source review. No production file
was modified, no production key was accessed, and no adversarial load was run.
Line numbers refer to the inspected tree and may move with set 7 formatting.

## Conclusion

The restart optimization does not expose an observed remote hash-injection
path: ordinary block-index insertion computes the hash locally from the block
header, and only the local-database loader bypasses that computation. It is
reasonable to trust authenticated local database state as btcd already trusts
its UTXO set and best-chain state. This is not an integrity guarantee for an
untrusted database snapshot or a database modified by another process.

The comment claiming every stored index entry has passed full validation is
incorrect. Side-chain blocks and blocks that fail transaction connection can
be stored. The actual justification is that the stored identity was locally
computed from the header, while the validation status is stored separately.

There are concrete local-corruption error-handling gaps worth fixing in the
touched startup paths. No network-reachable way to manufacture those malformed
metadata keys/rows was found in this review.

## Insertion-to-restart trust trace

1. Network/RPC block parsing creates a `btcutil.Block` from block bytes; its
   cached hash is private and initially unset. `btcutil/block.go:96-105`
   derives that cached identity through `wire.MsgBlock.BlockHash`, which
   delegates directly to its header (`wire/msgblock.go:312-314`). There is no
   exported `SetHash` setter.
2. `blockchain.ProcessBlock` computes `block.Hash`, rejects duplicates, and
   checks header PoW and block sanity before orphan handling
   (`blockchain/process.go:143-170`, `214-229`). Header-only acceptance first
   resolves a known, noninvalid parent, then checks header sanity/context
   (`blockchain/accept.go:126-175`).
3. Fresh node construction uses `newBlockNode` -> `initBlockNode` ->
   `header.BlockHash` (`blockchain/blockindex.go:119-120`, `154-157`). The only
   production call of `initBlockNodeWithHash` outside that wrapper is the
   local index loader (`blockchain/chainio.go:1311`).
4. `dbStoreBlockNode` serializes `node.Header()` and `node.status`, then keys
   the row by the same locally derived `node.hash` and height
   (`blockchain/chainio.go:1452-1469`). The block database likewise keys stored
   block bytes by `block.Hash()` (`database/ffldb/db.go:1166-1189`).
5. Importantly, `maybeAcceptBlock` stores the body and index at
   `blockchain/accept.go:57-81`, **before** transaction/script connection at
   `:89`. A failed connection or nonwinning side branch may persist. Loading
   a hash is not asserting `statusValid`; that bit is separate.
6. Header-only nodes are **not currently persisted**:
   `blockchain/blockindex.go:524-539` avoids all-header writes and `:544-553`
   skips individual header-only records. Restart discards header-only sync
   progress and sets bestHeader to the stored full-block best tip
   (`blockchain/chainio.go:1319-1326`). This is a potential expensive resync
   cost, not a stored-header validation bypass.
7. The loader checks key length = 36, first key hash = configured genesis,
   each nonfirst header's parent exists, and stored best-tip hash resolves
   (`blockchain/chainio.go:1277-1305`, `1319-1324`). It then uses stored hashes
   and statuses. Ancestors of the persisted best tip are upgraded to valid
   at `:1338-1354`, relying on the trusted persisted chain state as upstream
   already does.
8. Ordinary storage bit corruption has lower-layer checks: ffldb block
   records carry CRC32C and network magic (`database/ffldb/blockio.go:557-575`),
   and metadata LevelDB opens with `opt.DefaultStrict`
   (`database/ffldb/db.go:2135-2142`), whose selected dependency enables
   journal/block checksums. These are corruption checks, not cryptographic
   authentication against a local writer.

## Findings

### ST-1: migration errors are swallowed during startup

**Severity:** medium operational reliability; low security impact under the
trusted-local-disk threat model. Pre-existing upstream defect.

`blockchain/chainio.go:1222-1226` calls `migrateBlockIndex` but returns `nil`
when that operation returns an error. An initialized old-format database with
missing buckets, malformed data, or an I/O failure can therefore appear to
have initialized successfully without creating `bestChain`. Later `New`
continues into threshold/UTXO initialization and dereferences the best node
(`blockchain/chain.go:2325-2337`).

**Fix:** return the migration error. Add a small mocked/temporary database
test that forces migration failure and asserts a returned error rather than
successful initialization/panic. This does not require PoW grinding.

### ST-2: legacy migration trusts lengths and graph shape before checking them

**Severity:** low security, medium recoverability for local corruption.
Legacy migration only; no P2P route to malformed metadata identified.

- `readBlockTree` slices row bytes `[12:92:92]` before checking length
  (`blockchain/upgrade.go:156-157`) and key bytes `[0:32]` without a length
  check (`:168-169`). Short rows or short keys panic. The second migration
  pass repeats both unchecked slices (`:105-110`). Long keys are silently
  truncated to 32 bytes.
- `determineMainChainBlocks` indexes the graph and dereferences each parent
  without checking missing keys/parents or revisiting nodes (`:227-230`).
  A nonexistent stored tip panics; a corrupt cyclic tip ancestry can loop
  indefinitely. `determineBlockHeights` also lacks a visited/cycle guard
  (`:203-218`), although valid hash-keyed blocks cannot form a cycle without
  breaking the assumed header-to-hash relationship.

The short-row and second-pass short-key issues predate the PoW swap. The new
readBlockTree reliance on keys makes its own key-length validation necessary
at the same trust boundary. The migration is not expected for new SLUG
databases, but remains executable code and should fail cleanly.

**Fix:** require exactly 32-byte keys and at least the documented row length
before slicing; reject missing graph nodes/parents and cyclic traversal with
explicit corruption errors; propagate those errors. Do not hash every header
just to detect malformed lengths. Unit tests can use an in-memory fake Bucket
and constructed rows without mining.

### ST-3: cheap consistency checks are absent from the optimized loader

**Severity:** low security under trusted disk; hardening opportunity relevant
to the changed trust assumption.

- The loader checks key length but ignores its first four encoded-height
  bytes (`blockchain/chainio.go:1277-1283`). Node height is derived from parent
  links (`blockchain/blockindex.go:143-147`), so a key that misstates height
  is not directly rejected if ordering still permits parent resolution.
- The first row is accepted as genesis based only on the key hash; its
  serialized header and zero parent are not compared to the configured
  genesis header (`blockchain/chainio.go:1289-1295`).
- Deserialized `bestChainState.height` and `workSum` (`:1013`, `:1028-1029`)
  are not compared with the reconstructed tip (`:1319-1326`). The loader
  trusts the best-tip hash and rederives chain state from the linked nodes.
- `deserializeBlockRow` accepts any bytes after the 80-byte header and status
  byte (`:1385-1399`) and does not reject unknown status bits.

These gaps do not give a network peer authority to choose database keys. A
local writer already controls UTXOs, statuses, and the best-chain pointer.
However, the old header rehash happened to detect some header/key mismatches;
the optimized loader intentionally no longer provides that incidental check.

**Fix:** cheaply compare key height against derived height, genesis serialized
header against configured genesis, and persisted best height/work against
reconstructed values. Reject malformed row lengths/status combinations where
compatibility permits. Preserve the explicit trusted-local-disk assumption;
do not claim these checks authenticate an arbitrary imported database.

### ST-4: verifychain is not a complete stored-index hash audit

**Severity:** low, documentation/recovery limitation.

`rpcserver.go:3723-3755` loads blocks by current index height and optionally
runs `CheckBlockSanity`, which recomputes PoW and transaction sanity. It does
not compare the recomputed header hash to the index hash used to fetch that
block, compare the raw block's header to the index's copied header, or verify
the full ancestry/transaction state. Consequently it should not be presented
as validating the restart trust boundary or authenticating a database snapshot.

**Suggestion:** a future explicit database-integrity check could compare those
identities and linkages. Full-header rehash should remain an opt-in offline
or maintenance check rather than being silently restored on every startup.

## Argon2 parameter handling

**Reviewed; no attacker-controlled cost parameter path found.**

- The sole production Argon2 invocation is `wire/blockheader.go:91-92`.
- Time=1, memory=256*1024 KiB, lanes=1, output length=32 are unexported compile
  time constants (`wire/blockheader.go:53-67`). No RPC/config/network field
  alters them. Header `Bits` affects only target arithmetic/comparison, not
  the Argon2 memory/time parameters.
- Both password and salt are the fixed canonical 80-byte serialized header
  (`:84-89`); a peer controls field values, not their serialized lengths.
  The header wire decoder only reads fixed fields (`:177-203`).
- The selected `golang.org/x/crypto/argon2` implementation normalizes memory
  to a multiple of 4*lanes and allocates `memory` elements of
  `[128]uint64` (argon2.go:114-128,163). For these constants this is exactly
  268,435,456 bytes (256 MiB) for the main arena per active invocation, plus
  small overhead. The constants meet the implementation's nonzero time/lanes
  constraints, so peer header values cannot trigger its parameter panics.
- `wire.MsgBlock.BlockHash` and `btcutil.Block.Hash` delegate/cache this same
  algorithm; no alternate production header digest API was found in the
  scoped search. Transaction/merkle hashes continue through chainhash APIs.

Fixed per-call memory is **not** a global memory bound: simultaneous hashing
from peer/RPC/mining callers can allocate multiple arenas. The parent audit
owns concurrency/admission/rate-limit findings and should combine those with
this fixed 256 MiB figure.

## Limits of this subreview

Reviewed by source inspection: insertion/index persistence, header-only
status persistence, current index restart, v1 index migration, best-state
loading, block storage checksums, `verifychain` scope, Argon2 invocation and
fixed wire serialization. Findings above were not dynamically reproduced.

Not reviewed here: the entire UTXO/spend-journal upgrade history, crash/fault
injection under fsync failures, full LevelDB correctness, hostile snapshot
ingestion tooling, Argon2 cryptanalysis, mining or P2P rate-limit behavior,
and all possible application misuse of mutable exported `btcutil.Block`
internals. Those are not represented as passing this audit.
