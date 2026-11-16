# SLUG PoW Swap Map: btcd Fork Inventory

Written November 15, 2026. A mapping job only: what must change in
the taytr-llc/slug fork (of btcsuite/btcd, master) to implement the
settled SLUG design, and where. Requirements: BUILD_PLAN.md
workstream 1, BRAWLCHAIN_SPEC.md (D32, D50, D66),
ARGON2ID_PARAMS.md. Settled values: Argon2id 256 MB, t=1, p=1;
21M cap; 10 SLUG starting reward; halving every 1,050,000 blocks;
2-minute blocks; fees to the block miner.

All paths are relative to the repo root. Line numbers are from the
fork's master as cloned Nov 15, 2026 (btcd v0.27.0 era) and will
drift; symbols are the durable reference.

## 0. Repo facts

- **Go version:** every go.mod in the repo says `go 1.26.10`.
- **Modules: 10, not one.** Root module `github.com/btcsuite/btcd`
  plus nine nested modules, each versioned `/v2` (except
  v2transport): `address`, `btcec`, `btcutil`, `chaincfg`,
  `chainhash`, `psbt`, `txscript`, `wire`, `v2transport`. The root
  module depends on the nested ones by version, so cross-module
  changes need version bumps or replace directives during dev.
- **Top-level layout:** the node daemon is the root package (`btcd.go`,
  `server.go`, `rpcserver.go`, `config.go`, `params.go`, all package
  main). Library packages: `addrmgr`, `blockchain`, `btcjson`,
  `chaincfg`, `chainhash`, `connmgr`, `database` (ffldb), `limits`,
  `mempool`, `mining` (+ `mining/cpuminer`), `netsync`, `peer`,
  `rpcclient`, `txscript`, `wire`, plus the nested-module packages
  above. Utility binaries under `cmd/`: `btcctl`, `addblock`,
  `findcheckpoint`, `gencerts`. `integration/rpctest` spins up real
  nodes for tests.

## 1. Proof of work: the Argon2id swap surface

The good news first: exactly one function in the codebase computes
the block header hash. Everything else that double-SHA256s (txids in
`btcutil/tx.go`, merkle roots in `blockchain/merkle.go`, base58 and
HD checksums, addrmgr bucketing) is not PoW and must NOT change;
the script/sighash machinery in `txscript` also stays SHA256.

### 1.1 The hash itself

- `wire/blockheader.go:47` `(*BlockHeader).BlockHash()` — currently
  `chainhash.DoubleHashRaw` over the 80-byte serialized header.
  This is the swap point: return the Argon2id digest (256 MB, t=1,
  p=1, 32-byte output) of the same 80-byte serialization instead.
  Every block ID in the system derives from this one function.
- `wire/msgblock.go:313` `(*MsgBlock).BlockHash()` — delegates to
  the header method; no separate change.
- `chainhash/hash.go` — `Hash` (32 bytes), `DoubleHashB/H/Raw`.
  Keep as is: Argon2id's 32-byte output fits the existing `Hash`
  type, so no ID format, DB key, or wire format changes size.
  Argon2id itself comes from `golang.org/x/crypto/argon2`
  (`argon2.IDKey`), already benchmarked in `argon2id-bench/`.

### 1.2 The check

- `blockchain/validate.go:372` `checkProofOfWork` — compares
  `HashToBig(header.BlockHash())` against the target from the
  header's Bits; rejects above `powLimit`. Hash-agnostic once
  BlockHash changes, but it runs once per block per node, so this
  is where the 340 ms verify cost lands.
- `blockchain/validate.go:407` `CheckProofOfWork` (exported
  wrapper) and `:501` `checkBlockHeader` / `:534` `checkBlockSanity`
  / `process.go:169` — the call chain every received block takes.
- `blockchain/difficulty.go:17` `HashToBig`, `:45` `CompactToBig`,
  `:53` `BigToCompact`, `:67` `CalcWork` — target/work arithmetic;
  no hash function inside, no change needed, but `CalcWork` feeds
  `workSum` in `blockchain/blockindex.go:122`, so chain selection
  by cumulative work keeps working unchanged.

### 1.3 Difficulty calculation and retarget

- `blockchain/difficulty.go:136` `calcNextRequiredDifficulty` —
  the retarget: every `BlocksPerRetarget()` blocks, scale by
  actual vs `TargetTimespan`, clamped by
  `RetargetAdjustmentFactor` (4) and `PowLimit`. Entry points:
  `:238` `(*BlockChain).CalcNextRequiredDifficulty`,
  `blockchain/validate.go:1438` `BlocksPerRetarget()` =
  TargetTimespan / TargetTimePerBlock.
- With 2-minute blocks and Bitcoin's 14-day timespan kept, the
  window becomes 10,080 blocks (from 2,016). The spec leaves
  difficulty specifics open (spec section 17), so timespan is an
  open decision, but something must be chosen: the window size,
  the clamp, and the genesis difficulty all interact with a
  340 ms hash (see Risks).
- Testnet-only machinery in the same file that assumes Bitcoin
  timing: `:75` `calcEasiestDifficulty`, `:111`
  `findPrevTestNetDifficulty`, and the `ReduceMinDifficulty` /
  `MinDiffReductionTime` path (Bitcoin: 20 minutes = 2 blocks;
  at 2-minute blocks the semantics change). BIP94 timewarp
  handling hangs off `Params.EnforceBIP94`.

### 1.4 Mining and templates

- `mining/mining.go:446` `(*BlkTmplGenerator).NewBlockTemplate` —
  builds the template; `:815` sets header Bits from
  `CalcNextRequiredDifficulty`; `:254` `createCoinbaseTx` pays
  `blockchain.CalcBlockSubsidy` (`:286`) plus fees to the miner
  (D32's fees-to-miner is already the behavior); `:242`
  `standardCoinbaseScript`; `:868` `AddWitnessCommitment` (segwit,
  stays if segwit stays).
- `mining/cpuminer/cpuminer.go:208` `solveBlock` — the nonce loop;
  `:278` calls `header.BlockHash()` per attempt. With Argon2id
  this loop IS the miner (about 3 attempts/s/thread); the
  extra-nonce and template-refresh cadence around it was designed
  for gigahash rates and needs retuning, not replacing. `:548`
  `GenerateNBlocks` backs the `generate` RPC (`rpcserver.go:892`
  `handleGenerate`), which the gym clients and tests will use.
- `rpcserver.go` `getblocktemplate` / `submitblock` handlers — the
  stratum-facing surface the official pool adapts (workstream 3);
  no PoW logic inside, they inherit BlockHash.

## 2. Chain parameters (chaincfg)

One struct, six network definitions, one genesis file. This is the
highest-density change area.

### 2.1 The struct and the networks

- `chaincfg/params.go:179` `type Params struct` — fields to set per
  network: `Name`, `Net` (magic), `DefaultPort`, `DNSSeeds`,
  `GenesisBlock`, `GenesisHash`, `PowLimit`, `PowLimitBits`,
  `CoinbaseMaturity`, `SubsidyReductionInterval`, `TargetTimespan`,
  `TargetTimePerBlock`, `RetargetAdjustmentFactor`,
  `ReduceMinDifficulty`, `MinDiffReductionTime`,
  `Checkpoints`, BIP9 `Deployments` + `MinerConfirmationWindow` +
  `RuleChangeActivationThreshold`, `Bech32HRPSegwit`, the five
  address ID bytes, `HDPrivateKeyID` / `HDPublicKeyID`,
  `HDCoinType`, and the softfork heights `BIP0034Height`
  (+`BIP0034Hash`), `BIP0065Height`, `BIP0066Height`.
- Network definitions in the same file: `MainNetParams` (:305),
  `RegressionNetParams` (:471), `TestNet3Params` (:593),
  `TestNet4Params` (:739), `SimNetParams` (:869), `SigNetParams`
  (:985, built by `CustomSignetParams` with a signet challenge).
  SLUG needs mainnet plus its own testnet/regtest/simnet set;
  testnet4 and signet are Bitcoin-specific extras to drop or
  repurpose.
- PowLimit values are package vars at `chaincfg/params.go:29-46`
  (`mainPowLimit` = 2^224-1, `PowLimitBits` 0x1d00ffff, etc.).
  These must be re-derived for Argon2id: at Bitcoin's mainnet
  target, one expected block is ~2^32 attempts, which at 340 ms
  per attempt is decades per thread. Genesis and early-chain
  difficulty have to start near regtest levels (0x207fffff,
  ~2 attempts) and let the retarget climb.
- Current values that change under D32, per network today:
  `SubsidyReductionInterval` 210,000 (150 on regtest),
  `TargetTimePerBlock` 10 minutes, `TargetTimespan` 14 days,
  `CoinbaseMaturity` 100 (no settled SLUG change; located here).
- Root `params.go` (package main, :30-69) maps CLI network flags
  to these Params; `chaincfg/params.go:1161` `Register`, `:1231`
  `RegisterHDKeyID`, `:1280` `init()` register all networks for
  address/HD lookups. New networks must be registered the same way.

### 2.2 Genesis

- `chaincfg/genesis.go` — `genesisCoinbaseTx` (:16, contains the
  Times 03/Jan/2009 headline bytes), and per-network
  `genesisBlock` / `genesisHash` / `genesisMerkleRoot` vars
  (mainnet :60-87, regtest :92, testnet3 :120, testnet4 :147-205
  with its own genesis tx, simnet :219, signet :247). SLUG needs
  a new genesis block per network: new coinbase message, new
  timestamp/bits, a nonce actually mined with Argon2id, and the
  `genesisHash` constants set to the Argon2id digest of the new
  header (these constants are asserted by `chaincfg/genesis_test.go`
  and by `GenesisHash` checks at chain init).
- `blockchain/checkpoints.go` — `Checkpoints()`,
  `LatestCheckpoint`, `verifyCheckpoint`, `findPreviousCheckpoint`
  consume `Params.Checkpoints`. The Bitcoin mainnet list (36
  entries in MainNetParams) must be emptied for SLUG; the signed
  checkpoint / fast-sync anchor design (BUILD_PLAN workstream 1)
  builds on this mechanism.

### 2.3 Network identity on the wire

- `wire/protocol.go:180-196` — magic values: `MainNet`
  0xd9b4bef9, `TestNet` (regtest) 0xdab5bffa, `TestNet3`
  0x0709110b, `TestNet4` 0x283f161c, `SigNet` 0x40CF030A,
  `SimNet` 0x12141c16. SLUG needs fresh magics so its nodes never
  peer with Bitcoin's.
- Default ports live in Params (mainnet "8333", testnet3 "18333",
  regtest "18444", testnet4 "48333", simnet "18555"); RPC ports
  are config defaults in `config.go` / `cmd/btcctl`.
- DNS seeds: `Params.DNSSeeds` (mainnet lists 8 Bitcoin seeds),
  consumed by `connmgr/seed.go:36` and `config.go:593`
  (signet seeds). Replace with SLUG seed hosts; the design also
  wants public seed nodes (workstream 1).

## 3. Address and key formats

All values come from `chaincfg.Params`; the encoding code reads
them, so changing the params is most of the work. SLUG needs fresh
values throughout, or SLUG and Bitcoin addresses are
interchangeable and funds can be sent to the wrong chain.

- Base58 prefixes, `chaincfg/params.go` MainNetParams (:452-458):
  `PubKeyHashAddrID` 0x00 ("1..."), `ScriptHashAddrID` 0x05
  ("3..."), `PrivateKeyID` 0x80 (WIF), `WitnessPubKeyHashAddrID`
  0x06, `WitnessScriptHashAddrID` 0x0A. Same fields per network.
- Bech32: `Bech32HRPSegwit` — "bc" mainnet, "tb" testnet,
  "bcrt" regtest. Encoding in the `address` module
  (`address/bech32`), consumed by `address/address.go`
  (`NewAddressPubKeyHash` :324, `NewAddressScriptHash` :384,
  `NewAddressWitnessPubKeyHash` :633, and `DecodeAddress`, which
  resolves the network by matching ID bytes against registered
  params).
- WIF: `btcutil/wif.go:53` `NewWIF` stamps `net.PrivateKeyID`;
  checksums stay double-SHA256.
- BIP32: `HDPrivateKeyID` 0x0488ade4 ("xprv"), `HDPublicKeyID`
  0x0488b21e ("xpub") in Params; consumed by
  `btcutil/hdkeychain/extendedkey.go` (:643-653 version checks,
  :715 master key creation). `HDCoinType` 0 (BIP44) in Params;
  SLUG should register its own coin type.
- Amounts: `btcutil/const.go:12` `SatoshiPerBitcoin` = 1e8 and
  `:15` `MaxSatoshi` = 21e6 x 1e8. Eight decimals and a 21M cap
  carry over to SLUG unchanged in value, but the names are
  branding surface (section 4).

## 4. Identity and branding surface (inventory only, nothing renamed)

- **Go module paths:** all 10 go.mod files declare
  `github.com/btcsuite/btcd[/...]` (see section 0), and every Go
  file in the repo imports by those paths. A module rename is a
  mechanical sweep of every import line in the repo plus go.mod /
  go.sum in each module.
- **Binaries:** root daemon (built from the root package, binary
  name `btcd`), `cmd/btcctl` (RPC CLI), `cmd/addblock`,
  `cmd/findcheckpoint`, `cmd/gencerts`. Build/release naming in
  `Makefile`, `Dockerfile`, and `release/`.
- **Config and data dirs:** `config.go:41` `defaultConfigFilename`
  = "btcd.conf", `:69` `sampleConfigFilename` = "sample-btcd.conf"
  (file at repo root), `:76` `defaultHomeDir` =
  `btcutil.AppDataDir("btcd", false)` (~/.btcd), data subdir
  "data". `cmd/btcctl/config.go:30-33`: AppDataDir("btcd"),
  AppDataDir("btcctl"), AppDataDir("btcwallet"), default config
  "btcctl.conf". RPC cert/key default filenames live in the same
  two config files.
- **Names in code:** root `version.go` (app version constants,
  currently 0.27.0), log subsystem tags in `log.go` files across
  packages (e.g. "BTCD"), the default user agent / protocol
  service strings in `wire` and `peer`, and `sample-btcd.conf`
  option names (many are `btcd`-prefixed in docs only; the flags
  themselves are mostly generic).
- **Upstream sync note:** the fork already has `upstream`
  (btcsuite/btcd) wired. Every branding change increases the
  merge surface for pulling upstream fixes; renaming the module
  path is the single most expensive one because it touches every
  file. Worth deciding once, early, and doing it before other work
  stacks on top.

## 5. Bitcoin-specific assumptions to re-decide

- **Subsidy is half hardcoded:** `blockchain/validate.go:44`
  `baseSubsidy = 50 * btcutil.SatoshiPerBitcoin` is a const in
  code, not a Params field. SLUG's 10 SLUG start means editing
  this const to 10, and setting `SubsidyReductionInterval` to
  1,050,000 per network. `CalcBlockSubsidy` (:259) computes
  `baseSubsidy >> (height / interval)`; the shift math truncates
  per halving, so the true total lands a hair under 21,000,000
  (same property as Bitcoin), despite the spec's "exactly"
  wording. Tests in `blockchain` and `fullblocktests` assert
  Bitcoin subsidy values and will fail until updated.
- **BIP activation state:** MainNetParams carries Bitcoin's
  history: `BIP0034Height` 227931 with a specific Bitcoin
  `BIP0034Hash`, BIP65/66 heights, and BIP9 `Deployments` for
  CSV, Segwit, and Taproot with 2016-2021 start/end times and a
  taproot `MinActivationHeight` of 709,632. On a fresh chain
  these need an explicit decision (the usual fork answer:
  activate from genesis, heights 0, deployments removed or
  always-active). Enforcement code: `blockchain/validate.go`,
  `blockchain/thresholdstate.go`, `txscript`.
  `MinerConfirmationWindow` 2016 is defined as timespan/spacing,
  so it must move with the difficulty decision in 1.3.
- **Block size/weight:** `blockchain/weight.go:23`
  `MaxBlockWeight` = 4,000,000 (consensus), `wire/msgblock.go:27`
  `MaxBlockPayload` = 4,000,000 (p2p), template defaults in
  `mining/policy.go` (`Policy.BlockMaxWeight` etc.), mempool
  standardness limits in `mempool/policy.go`. No settled SLUG
  change; located in case the design wants one.
- **Coinbase maturity:** `Params.CoinbaseMaturity` = 100 on every
  network; enforced in `blockchain/validate.go` and the UTXO
  cache. No settled change.
- **Test networks' extras:** testnet4's BIP94 block-storm rules
  (`EnforceBIP94`, its own genesis tx) and signet's challenge
  script (`CustomSignetParams`, `chaincfg/params.go:1000`) are
  Bitcoin-2024 specifics SLUG almost certainly drops.
- **The genesis coinbase headline** (section 2.2) and the
  checkpoint list (2.2) are Bitcoin history embedded in code;
  both must go for a clean SLUG genesis.

## 6. What fights the Argon2id swap (risks)

1. **Startup rehashes the whole chain.** `blockchain/chainio.go:1079`
   (`initChainState` / block index load) rebuilds every block node
   from stored headers via `newBlockNode`
   (`blockchain/blockindex.go:121`), which calls
   `header.BlockHash()`. With SHA256 that is free; with Argon2id
   every node restart replays one 340 ms hash per block of history
   (~25 h per year of chain, the ARGON2ID_PARAMS sync tax, but on
   every start, not just first sync). The loader needs to store
   and trust hashes (or verify lazily) before this is survivable;
   the signed-checkpoint fast-sync design only helps if the index
   loader is taught to use it.
2. **Difficulty bootstrap at a 340 ms hash.** Bitcoin's PowLimit
   and genesis bits assume ~2^32 expected attempts is cheap. At
   3 attempts/s/thread, mainnet-bits genesis is effectively
   unminable and the early chain cannot bootstrap. Genesis bits,
   PowLimit, the retarget window (10,080 blocks if the 14-day
   timespan is kept), and the testnet min-difficulty rules all
   have to be designed for a slow hash up front (BUILD_PLAN
   already flags "difficulty tuning for a slow hash"; this is
   where it bites). Until difficulty climbs, `CalcWork` totals
   are tiny, which also affects checkpoint candidate logic
   (`blockchain/checkpoints.go:200` `IsCheckpointCandidate`).
3. **BlockHash is the universal ID, and everyone recomputes it.**
   34 non-test `.BlockHash()` call sites outside wire: block index,
   chain IO, indexers (`blockchain/indexers`: tx/addr/cf indexes
   keyed by block hash), GCS filter construction
   (`btcutil/gcs/builder`), netsync locators, RPC lookups,
   `fullblocktests/generate.go`. The swap is one function, but
   any hot path that casually recomputes a block's hash now pays
   340 ms: the test generators (`fullblocktests`,
   `integration/rpctest/blockgen.go`, cpuminer `GenerateNBlocks`)
   grind nonces and will slow the whole test suite, and any
   future explorer or pool code that assumes "hash is cheap"
   (the entire Bitcoin tooling ecosystem does) will feel it.
   Mitigation is discipline: compute once, store, pass hashes
   around; the codebase mostly does this via `blockNode.hash`,
   but the index-load path (risk 1) is the counterexample that
   ships in the box.
