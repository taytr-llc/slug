# SLUG Change Set 2: Chain Parameters and Genesis

Branch `slug-chain-params`, three commits on top of change set 1
(`slug-pow-swap`), which is on top of master (dd169fe). The bundle
`chain-params.bundle` (12,415 bytes) contains the full history from
master, both change sets included, and was verified: `git bundle
verify` passes and a scratch clone at plain master fetched it and
produced a tree identical to the branch.

This is Phase A: the chain defined here is a disposable test chain.
The permanent genesis comes later (Phase C), so choices are clear
and documented rather than agonized over.

## What changed, parameter by parameter (MainNetParams unless noted)

### Economics (spec D32, settled)
- `baseSubsidy` in `blockchain/validate.go`: 50 coins -> **10 SLUG**
  (10 * 1e8 base units; 1e8 base units per coin unchanged).
- `SubsidyReductionInterval`: 210,000 -> **1,050,000** blocks.
- `TargetTimePerBlock`: 10 minutes -> **2 minutes**.
- `TargetTimespan`: unchanged, 14 days (see the retarget flag below).
- `CoinbaseMaturity`: unchanged, 100.
- Fees to the block miner: unchanged (already btcd behavior).
- Supply math: 30 subsidy eras of 1,050,000 blocks each. Because the
  halving is an integer shift, the total issued is 20,999,999.8635
  SLUG, a hair under the spec's "exactly 21,000,000" (same truncation
  property Bitcoin has). Worth knowing before the permanent genesis;
  changing it would require a non-shift subsidy schedule.

### Difficulty for a slow hash
- `mainPowLimit`: 2^224 - 1 -> **2^255 - 1** (the regtest-class limit).
- `PowLimitBits`: 0x1d00ffff -> **0x207fffff** (about 2 expected
  Argon2id attempts per block at genesis).
- `ReduceMinDifficulty`: unchanged, false.

### **DECISION FLAG FOR MIKE: the retarget window**
Keeping `TargetTimespan` at 14 days with 2-minute blocks makes the
difficulty retarget window **10,080 blocks** (about 14 days of
blocks), instead of Bitcoin's 2,016. That is what this change set
ships. The alternative is a 2,016-block window (about 2.8 days),
which reacts faster but with a 340 ms hash and a small early network
it also amplifies noise and timewarp games. Either is defensible;
**confirm the 10,080-block window (or pick 2,016) before the real
genesis.** Related leftovers in the same area, also to decide before
the permanent genesis: `MinerConfirmationWindow` still says 2,016
and the BIP9 deployments (CSV/Segwit/Taproot) still carry Bitcoin's
2016-2021 voting windows, and `BIP0034Height`/`BIP0065Height`/
`BIP0066Height` still carry Bitcoin's activation heights and the
Bitcoin BIP0034 block hash. None of these bite a young chain (the
heights are far away and the deployment windows simply read as
expired), but they are Bitcoin history embedded in SLUG params and
should be replaced with activate-from-genesis values deliberately,
not inherited by default. `HDCoinType` also still says 0 (Bitcoin's
BIP44 coin type); SLUG should register its own before real wallets.

### The SLUG genesis block (mainnet)
- Coinbase message: "SLUG genesis, November 15 2026. Every fighter
  starts somewhere." (63 bytes, pushed after the bits push, same
  layout as Bitcoin's genesis).
- Coinbase output: the 10 SLUG genesis subsidy (1,000,000,000 base
  units) paid to an OP_RETURN script ("SLUG"), i.e. provably burned.
  A Phase A genesis has no treasury key to pay; the permanent
  genesis can revisit this.
- Timestamp: **2026-11-15 00:00:00 UTC** (Unix 1794700800).
- Bits: 0x207fffff. Nonce: **0** (Argon2id met the target on the
  first attempt; expected value was ~2 attempts).
- Merkle root: 7e88256720840d89bd12ea14616f212a59ac6ded579fbe99977530156833af30
- Genesis hash (Argon2id): **7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd**
- Regtest and simnet keep their inherited genesis blocks (rehashed
  by change set 1), as do testnet3/testnet4/signet. One trap found
  and fixed along the way: the other networks' merkle roots were
  defined as `= genesisMerkleRoot`, aliasing the mainnet var, so
  changing the mainnet root silently changed theirs. They now carry
  the inherited Bitcoin merkle root as explicit literals.

### Network identity (mainnet)
- **Magic**: `wire.MainNet` 0xd9b4bef9 -> **0x534c5547**, the ASCII
  bytes "SLUG" as a big-endian uint32. Note the bytes on the wire
  are the little-endian serialization, 47 55 4c 53 ("GULS"), the
  same convention as Bitcoin's (whose enum 0xd9b4bef9 serializes to
  f9 be b4 d9). SLUG nodes will not peer with Bitcoin nodes.
- **Ports**: P2P default 8333 -> **29333** (`DefaultPort`); RPC
  default 8334 -> **29332** (root `params.go` rpcPort, btcctl's
  mainnet default for both node and wallet, and the --listen /
  --rpclisten help strings and doc.go). Testnet/regtest/simnet/
  signet ports are unchanged.
- **DNS seeds**: the 8 Bitcoin seeds -> **empty**. No SLUG seeds
  exist yet; public seed nodes are a later workstream 1 item.
- **Checkpoints**: Bitcoin's 36 checkpoints -> **nil**. They
  referenced Bitcoin hashes. SLUG's signed checkpoints / fast-sync
  anchors are separate work.
- **Bech32 HRP**: "bc" -> **"slug"**.
- **Address version bytes** (all fresh, none collide with the other
  registered networks in this codebase):
  - PubKeyHashAddrID: 0x00 -> **0x53** (ASCII 'S'; addresses start
    with 'a', e.g. aQ44j1juvyTisyaC2peTFQbJEsPJ6xNJsd)
  - ScriptHashAddrID: 0x05 -> **0x47** (ASCII 'G'; start with 'V')
  - PrivateKeyID (WIF): 0x80 -> **0xd3** (WIF starts with 'Y')
  - WitnessPubKeyHashAddrID: 0x06 -> **0x54** (ASCII 'T')
  - WitnessScriptHashAddrID: 0x0A -> **0x55** (ASCII 'U')
  - HDPrivateKeyID: 0488ade4 ("xprv") -> **736c7567**, ASCII "slug"
  - HDPublicKeyID: 0488b21e ("xpub") -> **534c5547**, ASCII "SLUG"
- **GenerateSupported**: false -> **true** on mainnet params. The
  `generate` RPC refuses to run without it (rpcserver.go), and the
  Phase A test chain's only miner is the node itself. Documented in
  the params as a Phase A choice to revisit before the permanent
  genesis.
- Not changed: the network `Name` stays "mainnet", the Go module
  paths and binary names stay btcd/btcctl (rebrand change set), and
  testnet3/testnet4/signet params are entirely untouched.

### Tests
- `chaincfg/genesis_test.go`: `genesisBlockBytes` regenerated for
  the SLUG genesis (218 bytes); `TestGenesisBlock` passes against
  the new hash constant.
- `blockchain/subsidy_test.go` (new): `TestSlugBlockSubsidy` pins
  the interval at 1,050,000, subsidy at height 0 and 1,049,999 at
  10 SLUG, the first halving at block 1,050,000 (5 SLUG), and the
  tail (1 base unit in the last era, 0 from block 31,500,000).
- `blockchain/validate_test.go`: `TestCheckBlockSanity` and
  `TestCheckConnectBlockTemplate` are **skipped with an explanatory
  comment**, not deleted. Both feed SHA256-mined Bitcoin mainnet
  fixture blocks, which change set 2 retired; they can be restored
  if SLUG-native fixtures are generated.
- `wire/common_test.go`: the writeElement expectation for the
  mainnet magic updated to the new little-endian bytes.
- The change set 1 proof test (`TestCheckProofOfWorkArgon2id`)
  still passes.

## Verification, what actually ran

Environment: this VM, Go 1.27.4.

1. **Builds: pass.** `go build ./...` in wire, chaincfg, and the
   root module. `go vet` clean on wire, chaincfg, and the root
   module. `gofmt` clean.
2. **chaincfg suite: full pass** (3.0 s), including all six genesis
   block tests and the registration tests.
3. **wire suite: full pass** (after the magic expectation update
   above).
4. **Blockchain named subset: pass.** TestCheckProofOfWorkArgon2id,
   TestSlugBlockSubsidy, and the four regtest validation tests
   (TestRegtestBlockVersions, TestRegtestBuriedDeploymentsAlwaysActive,
   TestRegtestRejectsCoinbaseMissingHeight,
   TestRegtestAcceptsCoinbaseHeight). The two retired fixture tests
   report SKIP.
5. **Broader blockchain suite: did not finish, no failures seen.**
   A full run hit the 9-minute go test timeout (540 s) with the
   goroutine dump showing Argon2id block grinding in progress, the
   same behavior as change set 1. A verbose 4-minute probe got
   further into specifics: the eight top-level tests that completed
   (TestBip0030CheckNeeded* x6, TestFlushToDB,
   TestMaybeAcceptBlockReusesHeaderNode; 15 PASS lines including
   subtests) all passed with zero failures, and the probe then
   stalled inside TestAncestor (3m32s elapsed at the timeout), which
   builds a long chain block by block through the Argon2id-grinding
   test helper. Everything past TestAncestor (the rest of the chain,
   chainio, UTXO, and fullblocks tests) was not reached and remains
   unrun, exactly as in change set 1; expect more slow grinding
   there rather than new rule failures, but that is a prediction,
   not a result. The named subset in item 4 was verified separately.
6. **Adjacent suites (spot check, not in the required list):**
   - address module: FAILS on mainnet vectors: TestAddresses,
     TestAddressPayToAnchor/mainnet, TestDecodeAddressP2A (mainnet
     cases). These hardcode Bitcoin mainnet address strings against
     the old version bytes and "bc" HRP.
   - btcutil: FAILS: TestBlock (a Bitcoin fixture block's SHA256
     hash expectation; this is change set 1 Argon2id fallout
     surfacing, not change set 2), and
     TestEncodeDecodeWIF/encodeValidUncompressedMainNetWif (a WIF
     vector with the 0x80 prefix).
   - btcutil/hdkeychain: FAILS: TestBIP0032Vectors,
     TestExtendedKeyAPI, TestNet, TestZero,
     Example_defaultWalletLayout, Example_audits (canonical
     xprv/xpub vectors asserted against mainnet's HD IDs).
   These suites were left red deliberately: their vectors are the
   encoding machinery's cross-checks, and rewriting them is the
   rebrand/test-sweep work, not a params change. They are enumerated
   here so nobody mistakes them for new breakage later.
   btcutil/txsort, address/base58, and address/bech32 pass.
7. **Live proof on SLUG mainnet params: pass.** Built the node and
   ran it with no network flags (so MainNetParams) on a throwaway
   datadir, P2P on 127.0.0.1:29333, RPC on 127.0.0.1:29332, no peers:
   - Startup log: "Chain state (height 0, hash
     7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd,
     totaltx 1, work 2)" — the node initialized at the new genesis
     hash, matching the chaincfg constant exactly.
   - `generate 2` mined two blocks through cpuminer (the
     GenerateSupported=true params flag is what allows `generate`
     on mainnet params; no peer/connectivity workaround was needed,
     the node mined with 0 connections).
   - `getblockcount` = 2; `getblockhash 0` =
     7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd.
   - Block 1 (3c052adc2796ce8b8baa4709ab72823e0438f2e4a5f261e9eacd961ea5c966ea)
     read back over RPC at verbosity 2: bits 207fffff,
     previousblockhash = the SLUG genesis hash, and its coinbase
     pays **value 10** (the raw hex carries 0x3b9aca00 =
     1,000,000,000 base units) to the SLUG address
     aQ44j1juvyTisyaC2peTFQbJEsPJ6xNJsd, which the node itself
     decoded and reported as a valid mainnet pubkeyhash address.
   - Datadir and binaries deleted afterward.

## For Mike: fetch and build it on the Mac

The bundle contains both change sets and applies on plain master;
if slug-pow-swap is already checked out it applies just as well
(the commits are the same). Commits carry the repo-local identity
"Ren <ren@localhost>" from the build VM, as with set 1.

```sh
# put chain-params.bundle somewhere on the Mac, then:
cd ~/slug/slug
git fetch ~/slug/chain-params.bundle slug-chain-params
git checkout slug-chain-params
go build ./...

# the subsidy test and the set 1 proof test (a few seconds):
go test -run 'TestSlugBlockSubsidy|TestCheckProofOfWorkArgon2id' -v ./blockchain/
# chaincfg suite, genesis included:
(cd chaincfg && go test ./...)

# optional, the live check: run a SLUG mainnet node and mine 2 blocks
go build -o /tmp/slugd .
/tmp/slugd --notls --rpcuser=u --rpcpass=p \
  --datadir=/tmp/slugmain-home/data --listen=127.0.0.1:29333 \
  --rpclisten=127.0.0.1:29332 \
  --miningaddr=aQ44j1juvyTisyaC2peTFQbJEsPJ6xNJsd
# in another terminal:
go build -o /tmp/slugctl ./cmd/btcctl
/tmp/slugctl --notls --rpcuser=u --rpcpass=p \
  --rpcserver=127.0.0.1:29332 generate 2
/tmp/slugctl --notls --rpcuser=u --rpcpass=p \
  --rpcserver=127.0.0.1:29332 getblockhash 0
# expect 7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd

# when satisfied:
git push -u origin slug-chain-params
```

The mining address above is a throwaway (its hash160 is all zeros;
nobody has the key). The coins are test-chain coins and worthless.
