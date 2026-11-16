# SLUG Change Set 1: Argon2id Proof-of-Work Swap

Branch `slug-pow-swap`, two commits on top of master (dd169fe).
Bundle: `pow-swap.bundle` (5.4 KB, requires only master as its base).
Written November 15, 2026.

## The hash construction

The block header hash is now Argon2id over the standard 80-byte
serialized header:

```
BlockHash = argon2.IDKey(password = serializedHeader,
                         salt     = serializedHeader,
                         time = 1, memory = 256*1024 KiB (= 256 MB),
                         threads = 1, keyLen = 32)
```

The serialized header is both the password and the salt. There is no
per-block or per-network salt to store or transmit: any node can
recompute the digest from the header alone, the same property the old
double SHA256 construction had. The 32-byte output fits the existing
`chainhash.Hash` type, so no block ID, database key, or wire format
changed size.

Only the block header hash changed. Transaction IDs, merkle roots,
address checksums, and signature hashes remain double SHA256, per the
swap map.

## Files changed, per module

**wire** (the swap itself)
- `wire/blockheader.go`: `(*BlockHeader).BlockHash()` returns the
  Argon2id digest instead of `chainhash.DoubleHashRaw`. The Argon2id
  parameters are named constants at the swap point with the
  construction documented in the comment. No new dependency:
  `golang.org/x/crypto` was already in wire's go.mod.
- `wire/msgblock_test.go`: `TestBlockHash` expected value updated to
  the Argon2id digest of block 1
  (caf7d42adeeca5564c08f79ff213e0c267c5f4e0ab66f36327e63de2cfeb059c).

**chaincfg** (consequential, not a genesis redesign)
- `chaincfg/genesis.go`: the six genesis hash constants (mainnet,
  regtest, testnet3, testnet4, simnet, signet) recomputed as Argon2id
  digests of the **unchanged** inherited genesis headers. The genesis
  blocks themselves are untouched; SLUG's own genesis is a later
  change set. The chain will not initialize self-consistently without
  this, which is why it is in this change set.
- `chaincfg/genesis_test.go`: testnet4's extra hardcoded hash string
  updated to match.

**root module**
- `blockchain/pow_argon2id_test.go` (new): the proof test, below.
- `go.work` (new): Go workspace file using all ten modules. Without
  it, the modules pin each other by released version and a wire
  change is invisible to blockchain/mining. It is development
  scaffolding for the build-out phase and can be dropped once the
  modules carry their own version tags.

**Not touched:** chainhash (the Hash type needed no change), the
difficulty/target arithmetic (hash-agnostic), mining and cpuminer
(they call `header.BlockHash()` and inherit the swap), the rebrand,
mainnet chain parameters, and the block-index rehash issue from the
swap map (risk 1). Those are later change sets, as scoped.

**Difficulty:** no regtest or simnet adjustment was needed. Both
already sit at bits 0x207fffff (target ~2^255, about 2 expected
hashes per block), which is feasible at ~340 ms per Argon2id hash.

## Verification, what actually ran

Environment: this VM, Go 1.27.4 (repo asks 1.26.10; newer toolchain,
no download needed).

1. **Builds: pass.** `go build ./...` in the wire module, the
   chaincfg module, and the root module (which covers blockchain,
   mining, cpuminer, the node daemon, and resolves every nested
   module locally through go.work). `go vet` clean on wire and
   blockchain. `gofmt` clean.
2. **Proof test: pass.** `TestCheckProofOfWorkArgon2id`
   (blockchain/pow_argon2id_test.go) grinds a regtest-bits header
   through the new BlockHash (found nonce 1, hash 5f42cf87...),
   asserts `CheckProofOfWork` accepts it, asserts a tampered nonce
   is rejected, and asserts BlockHash no longer equals the double
   SHA256 of the same header. Runs in about 3 seconds.
3. **Existing suites:**
   - wire: **full suite passes** (2.4 s).
   - chaincfg: **full suite passes** (2.2 s), genesis tests included.
   - mining: passes (cpuminer has no test files upstream).
   - blockchain: **full suite did not finish** in the ~10-minute
     timebox (killed at the 9-minute go test timeout; the goroutine
     dump shows the shared test helper, `testhelper.SolveBlock`,
     grinding blocks under Argon2id). A named subset ran instead:
     the proof test and all four regtest validation tests
     (TestRegtestBlockVersions, TestRegtestBuriedDeploymentsAlwaysActive,
     TestRegtestRejectsCoinbaseMissingHeight,
     TestRegtestAcceptsCoinbaseHeight) **pass**; these drive fresh
     Argon2id-mined blocks through the full chain validation path.
   - Two subset failures, both the expected kind:
     `TestCheckBlockSanity` and `TestCheckConnectBlockTemplate`
     feed real Bitcoin mainnet blocks (SHA256-mined fixtures) to the
     PoW check; under Argon2id their digests are effectively random
     and land above the mainnet target ("block hash ... is higher
     than expected max"). This is the swap working as intended on
     foreign-chain fixtures. Fixing them belongs to the chain
     params / genesis change sets that retire the Bitcoin mainnet
     fixtures, not to this one.
   - Everything else in the blockchain suite beyond the named subset
     was **skipped** (not run to completion) because of the timebox:
     the fullblocks, chain, chainio, and UTXO suites are the bulk of
     it. Expect more Bitcoin-fixture fallout of the same kind there,
     plus genuinely slow block grinding; budget a dedicated pass
     when the params change set lands.
4. **Live node on regtest: pass.** Built the node binary and ran it
   on a throwaway datadir. It initialized at the new Argon2id
   regtest genesis hash (e1bdb13f4c817dba...), then
   `generate 3` via RPC mined three blocks through the real
   cpuminer path (block 1: bits 207fffff, nonce 3, hash
   6d32456551e77b61...), `getblockcount` reported 3. A restart came
   back at height 0 (btcd wipes the regtest database on start by
   upstream design) and mined another block. Datadir and binaries
   were deleted afterward.

## For Mike: fetch and build it on the Mac

The commits were made with a repo-local identity "Ren
<ren@localhost>" on the build VM; amend the author when landing them
if you want them under your name.

```sh
# put pow-swap.bundle somewhere on the Mac, then:
cd ~/slug/slug
git fetch ~/slug/pow-swap.bundle slug-pow-swap
git checkout slug-pow-swap
go build ./...

# the proof test (about 3 s of grinding):
go test -run TestCheckProofOfWorkArgon2id -v ./blockchain/

# optional, the live check: build the node and mine regtest blocks
go build -o /tmp/slugd .
/tmp/slugd --regtest --notls --rpcuser=u --rpcpass=p \
  --datadir=/tmp/slugtest-home/data --listen=127.0.0.1:18444 \
  --rpclisten=127.0.0.1:18334 \
  --miningaddr=mr3Fvx5uGA7tBHoC8mQW1grM1ihzcEssFi
# in another terminal:
go build -o /tmp/slugctl ./cmd/btcctl
/tmp/slugctl --regtest --notls --rpcuser=u --rpcpass=p \
  --rpcserver=127.0.0.1:18334 generate 3
```

The mining address above is a throwaway regtest address generated
for the verification run; the coins are regtest-only and worthless.
