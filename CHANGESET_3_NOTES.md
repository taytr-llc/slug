# SLUG Change Set 3: D67 Retarget Window and the Restart Rehash Fix

Branch `slug-chain-fixes`, two commits on top of change set 2
(`slug-chain-params`), which is on top of change set 1 and master
(dd169fe). The bundle `chain-fixes.bundle` (17,692 bytes) contains
the full history from master, all three change sets included, and
was verified: `git bundle verify` passes and a scratch clone at
plain master fetched it and produced a tree identical to the
branch.

Commits:
- ec65be7 chaincfg: D67 retarget window of 2,016 blocks on mainnet
- 30acf83 blockchain: trust the stored hash when loading the block index

## Change 1: the D67 retarget window (mainnet only)

Spec decision D67 (made after change set 2 shipped): the difficulty
retarget window is 2,016 blocks, not the 10,080 blocks that
Bitcoin's 14-day timespan produces at 2-minute blocks.

- `chaincfg/params.go`, MainNetParams only: `TargetTimespan`
  14 days (1,209,600 s) -> **time.Minute * 2 * 2016 = 241,920 s**
  (2,016 blocks at 2 minutes, ~2.8 days). `BlocksPerRetarget()`
  (TargetTimespan / TargetTimePerBlock) therefore goes 10,080 ->
  **2,016**.
- Unchanged, as specified: `TargetTimePerBlock` 2 minutes, the 4x
  `RetargetAdjustmentFactor` clamp, `ReduceMinDifficulty` false.
- The BIP9 fields are untouched: `MinerConfirmationWindow` stays
  2,016 and `RuleChangeActivationThreshold` stays 1,916. Note the
  confirmation window now *coincides* with the retarget window at
  2,016 blocks; before this change the confirmation window was
  2,016 while the retarget window was 10,080. Nothing in btcd
  requires the two to be equal or different (the threshold state
  machine counts in confirmation windows, difficulty in retarget
  windows), but anyone reading the params should know they now
  move together numerically.
- Regtest and simnet params are untouched (regtest also sets
  PoWNoRetargeting, so it never retargets at all).

### Retarget test evidence

New `blockchain/difficulty_test.go`, `TestMainNetRetargetWindow`,
passing. It pins the window three ways on MainNetParams:

1. `TargetTimespan` is exactly 2,016 * 2 minutes (241,920 s) and
   `BlocksPerRetarget()` is exactly 2,016.
2. With synthetic timestamps 60 s apart (blocks arriving twice too
   fast), the required difficulty for block 2015 is the previous
   block's bits unchanged: no retarget one block short of the
   boundary.
3. At the boundary, the required difficulty for block 2016 *is*
   recomputed and the target drops below the genesis target
   (harder), as it must when blocks arrive at half the target
   spacing. Under the old 10,080-block window this call would have
   returned the previous bits unchanged.

The test uses fake HeaderCtx nodes with synthetic timestamps (the
style of the existing difficulty-adjacent tests), not real
blockNodes: a real 2,016-node chain would cost one Argon2id hash
per node (~340 ms each, ~11 minutes) just to build.

One finding while writing it: the existing `stubChainCtx` in
regtest_test.go returns Min/MaxRetargetTimespan in *nanoseconds*
(it divides the Duration without converting to seconds), while
BlockChain's own ChainCtx returns seconds. It never fires there
because regtest short-circuits retargeting, but the stub cannot be
used for a mainnet retarget test. The new test therefore carries a
small `difficultyTestChainCtx` that mirrors BlockChain's
seconds-based derivation in New. The stub was left untouched
(out of scope); flagging it here so nobody trips on it later.

## Change 2: the restart rehash fix (swap map risk 1)

### The problem

`initChainState` (blockchain/chainio.go) rebuilds the in-memory
block index at every startup by deserializing every stored header
and calling `initBlockNode`, which set the node's hash to
`header.BlockHash()`. Free with SHA256; with Argon2id that is
~340 ms per block of history on *every restart* (~25 hours per
year of chain). Nothing about the result was ever in doubt: the
node was recomputing a value it already had.

### The fix

The block index bucket is keyed by `blockIndexKey(height, hash)`
(4-byte big-endian height, then the 32-byte hash), so the loader
already holds the correct hash for every entry as part of the
cursor key.

- `blockchain/blockindex.go`: `initBlockNode` now delegates to a
  new `initBlockNodeWithHash`, which takes the hash from the
  caller. `initBlockNode` itself is unchanged in behavior (it
  computes `blockHeader.BlockHash()` and passes it in), so fresh
  sync, block connection, and every other construction path
  compute and validate hashes exactly as before.
- `blockchain/chainio.go`, the `initChainState` load loop: the
  hash is copied out of the cursor key (with a key-length check
  that returns a corruption error on a malformed key) and passed
  to `initBlockNodeWithHash`. The two other `header.BlockHash()`
  calls in the same loop now use the key hash as well: the
  first-entry genesis check, and the parent-not-found error
  message. A comment at the change carries the safety argument.
- `blockchain/upgrade.go`, `readBlockTree`: the same pattern in
  the one-time v1->v2 block index migration. It iterated the old
  bucket discarding the key (`func(_, blockRow)`) and recomputed
  every hash, even though the v1 bucket is keyed by the block
  hash. It now takes the hash from the key, with a pointer comment
  to initChainState. This path runs once per ancient database,
  not per restart; it is fixed because it is the identical
  pattern, and flagged here as a judgment call.

### Why trusting the key is safe

The entry only exists because this node stored it under that hash
after the block passed full validation (the store path,
`dbStoreBlockNode`, keys the row by the node's own hash). The key
is also how the node itself looks everything up: the in-memory
index map, the stored best chain state, and block fetches all key
off the same hash, and the loader still cross-checks the first
entry against the params genesis hash and resolves each header's
PrevBlock against already-loaded nodes. All other on-disk chain
state loaded at startup (best chain state, UTXO set, spend
journal) is trusted in exactly the same way; the old code was not
a verification step, it was a recomputation whose result was
guaranteed to equal the key in any database this node wrote.
Databases written by an attacker are out of scope for the same
reason they are for the UTXO set: local disk is trusted storage.

### Rest of the startup path, audited

Grep of the load/startup path for other header-hash
recomputations found nothing else to fix:

- `createChainState` (chainio.go) computes the genesis hash via
  newBlockNode, but only on a fresh, uninitialized database,
  once, and there is no stored key to trust yet. Correct as is.
- `checkProofOfWork` and the header/block acceptance paths
  (validate.go, accept.go, thresholdstate.go) recompute hashes
  for *new* blocks and headers. That is validation, and it is
  unchanged by design.
- The indexers and RPC paths consume stored hashes; they do not
  rebuild nodes from headers at load.

## Verification, what actually ran

Environment: this VM, Go 1.27.4. This VM is slower than a modern
laptop (see the mining rate below); absolute times will improve
roughly 1.5-2x on current hardware, the ratios are the point.

1. **Builds: pass.** `go build ./...` in the root module,
   chaincfg, and wire; `go vet` clean on all three; `gofmt` clean.
2. **chaincfg suite: full pass** (2.6 s). **wire suite: full
   pass.**
3. **Blockchain named subset: full pass** (4.9 s):
   TestMainNetRetargetWindow (new), TestCheckProofOfWorkArgon2id
   (set 1 proof test), TestSlugBlockSubsidy (set 2), and the four
   regtest validation tests (TestRegtestBlockVersions,
   TestRegtestBuriedDeploymentsAlwaysActive,
   TestRegtestRejectsCoinbaseMissingHeight,
   TestRegtestAcceptsCoinbaseHeight).
4. **Broader blockchain suite: did not finish, no failures seen.**
   A full run was started and lost at ~5.5 minutes when the build
   VM restarted mid-run (its output was buffered and lost with
   it; nothing about the suite itself caused that). A 4-minute
   verbose probe was then run as the standing evidence, matching
   the set 1/2 pattern: 8 top-level tests completed, **zero
   failures** (TestMaybeAcceptBlockReusesHeaderNode, the six
   TestBip0030CheckNeeded* tests, TestFlushToDB), then the run
   stalled inside TestAncestor with the goroutine dump in
   testhelper.SolveBlock grinding Argon2id blocks, and hit the
   4-minute test timeout. Everything past TestAncestor remains
   unrun, exactly as in change sets 1 and 2; the stall is the
   known test-helper grinding cost, not a new failure mode. The
   named subset in item 3 was verified separately and passes.
5. **Live restart proof on SLUG mainnet params: pass, with a
   measured before/after.** Two nodes were built, this branch
   (fixed) and `slug-chain-params` (pre-fix), each on its own
   throwaway datadir, no peers:
   - Each mined 30 blocks at genesis difficulty (~92-94 s per 30
     blocks on this VM) and was stopped cleanly via the stop RPC.
   - **Pre-fix restart:** "Loading block index..." to "Chain
     state" in the log took **11.0 s** for 31 index entries
     (~355 ms per block, the Argon2id hash), and the process took
     **11,158 ms** from start to answering getblockcount.
   - **Post-fix restart:** the same log segment took **~1 ms**,
     and start-to-RPC was **162 ms**.
   - Functional assertion, both branches: the restarted node
     returned to the exact pre-shutdown tip (fixed: height 30,
     hash 31de689042ac2b383d5f22237653c32727b15b5f751afa7e9835c9e26
     adf9c32; pre-fix: height 30, hash 1b6b77ec41eb099678f23ecc077
     48899528798cd45b84e921e86b79fef09d415; the two chains differ
     because each mined its own blocks). The reloaded index is
     therefore not merely fast, it is the same index.
   - Datadirs and binaries were under /tmp and were wiped when
     the build VM restarted; the pre-fix worktree was pruned.
     Nothing persists.

## For Mike: fetch and build it on the Mac

The bundle contains all three change sets and applies on plain
master; it applies just as well on top of slug-chain-params (the
commits are the same). Commits carry the repo-local identity "Ren
<ren@localhost>" from the build VM, as with sets 1 and 2.

```sh
# put chain-fixes.bundle somewhere on the Mac, then:
cd ~/slug/slug
git fetch ~/slug/chain-fixes.bundle slug-chain-fixes
git checkout slug-chain-fixes
go build ./...

# the named tests: D67 retarget, set 1 proof, subsidy, regtest set
go test -run 'TestMainNetRetargetWindow|TestCheckProofOfWorkArgon2id|TestSlugBlockSubsidy|TestRegtest' -v ./blockchain/
# chaincfg and wire suites:
(cd chaincfg && go test ./...)
(cd wire && go test ./...)

# optional, the live check: mine 30 blocks, restart, watch the load
go build -o /tmp/slugd .
/tmp/slugd --notls --rpcuser=u --rpcpass=p \
  --datadir=/tmp/slugfix-home/data --listen=127.0.0.1:29333 \
  --rpclisten=127.0.0.1:29332 \
  --miningaddr=aQ44j1juvyTisyaC2peTFQbJEsPJ6xNJsd
# in another terminal:
go build -o /tmp/slugctl ./cmd/btcctl
/tmp/slugctl --notls --rpcuser=u --rpcpass=p \
  --rpcserver=127.0.0.1:29332 generate 30
/tmp/slugctl --notls --rpcuser=u --rpcpass=p \
  --rpcserver=127.0.0.1:29332 getbestblockhash
/tmp/slugctl --notls --rpcuser=u --rpcpass=p \
  --rpcserver=127.0.0.1:29332 stop
# start slugd again with the same command as above; in its log the
# gap between "Loading block index..." and "Chain state" should be
# ~instant instead of ~340 ms per block, and getbestblockhash must
# print the same hash as before the restart.

# when satisfied:
git push -u origin slug-chain-fixes
```

The mining address is the same throwaway from set 2 (hash160 all
zeros, nobody has the key). The coins are test-chain coins and
worthless.
