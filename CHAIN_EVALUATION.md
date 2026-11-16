# SLUG Chain Codebase Evaluation

November 15, 2026. Decision input for D49's open half: the approach is
settled (fork a proven open-source chain, add the fighter layer); this
memo picks the codebase. Facts checked against live sources on the
date above; Argonmine facts are from our own project records.

**Outcome: Mike adopted the recommendation the same day as Decision
66. The chain is a btcd fork with an Argon2id proof-of-work swap.**

## The constraints that do the deciding

From the settled spec, in order of how much they cut:

1. **Argon2id proof of work, 256 MB per attempt** (D50 area). No major
   proven chain ships Argon2id natively, so every candidate needs a
   PoW swap or a parameter change. This is the one real surgery
   wherever we land.
2. **A public, transparent ledger.** Rankings, marketplace listings,
   and checkpoints must be publicly checkable. Privacy-by-default
   chains are a defect here, not a feature.
3. **21M cap, 10 SLUG start reward, halving every 1,050,000 blocks,
   2-minute blocks, fees to the miner** (D32). Bitcoin-shaped emission.
4. **The team is one engineer plus AI help, for a decade.** Language,
   codebase size, and upstream health dominate everything else. The
   Mac and Windows gym apps (settled Nov 15) also have to ship from
   this stack.
5. **Pool from day one, solo always available** (D55-D58). Stratum
   pool software must be adaptable; share validation needs an
   Argon2id hasher.
6. **The fighter layer needs little from the chain:** the block hash
   as fighter DNA (any chain gives this) and a place to anchor signed
   checkpoints and marketplace commitments. Bitcoin-family OP_RETURN
   covers it natively.

## Candidates

### btcd (Go) — recommended

- Full Bitcoin node in Go by btcsuite; production use since 2013,
  actively maintained in 2026 (health trackers score it 84/100,
  current releases, modern Go), ISC license.
- Bitcoin-faithful consensus and RPC surface, so pool software
  (Miningcore forks, NOMP), explorers, and tooling adapt instead of
  being invented. Wallet is the separate btcwallet (same suite, Go);
  the hosted wallet is league-operated anyway, and export is
  standard keys and seed.
- Team fit is the argument: Go is readable and modifiable by a tiny
  team, and one Go codebase cross-compiles the node, the miner, and
  the Mac and Windows tray apps. That is the whole client plan in a
  single stack.
- Costs: the PoW swap is ours to do and tune (see Risks), and btcd
  itself has never been the consensus base of a large standalone
  coin, so the "proven" claim rests on its Bitcoin-faithful code and
  production history in Lightning infrastructure, not on fork
  precedent.

### Litecoin Core (C++) — runner-up

- The most-forked lineage on earth (via Bitcoin Core), Scrypt is
  memory-hard heritage, actively maintained (v0.21.5.8, Sep 2026).
  Dogecoin Core 1.14.9 (Sep 2026) shows the other side: forks inherit
  upstream bugs and live on a patch treadmill forever.
- Loses on team fit: a decade of C++ maintenance by a one-engineer
  team, and the native apps become a second stack instead of the
  same one. If fork precedent ever outweighs team fit, this is the
  fallback.

### Kryptokrona (CryptoNote) — ruled out

- The one production Argon2id lineage (CryptoNight-Turtle, mined via
  XMRig, p2pool culture). But the Turtle variant is deliberately
  lightweight, a few MB per attempt, not our 256 MB profile, and
  changing that is consensus surgery anyway.
- Privacy by default (ring signatures, stealth addresses) fights
  the public league, and CryptoNote's smooth emission fights D32.
  Three surgeries instead of one, in a messier codebase.

### Argonmine 1.2.1 source (Rust) — ruled out

- Right PoW family, and Mike knows this code, he has PRs in it. But
  the 1.3.0 source was never published, the maintainer has been
  silent since November 13, 2026, and the network is down. Forking
  it repeats the exact dependency that just burned him, and it fails
  D49's "proven" bar outright.

### Tari — ruled out

- L1 (Minotari) live since May 2025, L2 (Ootle) launched Nov 11,
  2026. But its PoW is RandomX/SHA3X/Cuckaroo lanes, not Argon2id;
  Mimblewimble privacy by default fights the public league; supply
  is 21 billion, not 21M; and the asset layer is a BFT side network,
  not mined. It was vetted in November as a mining doorway for Mike,
  never as a base. Wrong on four settled constraints.

### Blocknet (2026) — a reference, not a base

- A new chain (genesis Feb 15, 2026, no relation to the old Blocknet
  DEX project): Bitcoin-shaped blocks, coinbase, stratum pools,
  LWMA difficulty, running Argon2id at 2 GB per attempt in
  production, with a gentle CPU miner on Mac, Windows, and Linux.
  It self-describes as experimental, and it is privacy-first, so it
  is not a fork base. Its value is proof: the exact pattern we need
  (Bitcoin shape plus Argon2id plus stratum pools that recompute
  each share's hash) works end to end in 2026. Study it during the
  PoW swap.

## Risks of the recommended route, stated plainly

- **Verification cost.** Every node verifies Argon2id for every
  block. At 2-minute blocks a year of chain is about 263,000 blocks;
  at even 0.25 s per verify, initial sync carries roughly 18 hours
  of pure PoW checking per year of history. Blocknet's planned
  Faraday upgrade exists largely to lighten exactly this. Our
  mitigations are already in the design: signed public checkpoints
  double as fast-sync anchors. This is the main engineering risk
  and it is a sync problem, not a safety problem.
- **Difficulty tuning for a slow hash.** Bitcoin's retarget assumes
  cheap hashes; share difficulty, pool vardiff, and the genesis
  difficulty all need setting against Argon2id attempt times. The
  sim's D30 prospect calibration gives us the target rates to tune
  to.
- **No turnkey pool or explorer for this exact combination.** Pool
  share validation needs an Argon2id hasher added to a Miningcore
  fork or NOMP (libargon2 exists; Blocknet pools prove the pattern),
  and the explorer is an adapted open-source Bitcoin explorer.
  Bounded work, but it is work.
- **Security review before genesis is non-optional.** A PoW swap is
  consensus surgery; it goes on the pre-launch list next to the
  trademark opinion.

## Recommendation

Fork **btcd**, swap the proof of work to Argon2id at 256 MB, set the
D32 parameters (21M cap, 10 SLUG, halving every 1,050,000 blocks,
2-minute blocks), keep the Bitcoin RPC and stratum surface so pool
and explorer tooling adapts, and build the fighter layer off-chain
with on-chain checkpoint anchors, as the spec already describes.
One stack, Go, for the node, the miner, the pool glue, and the Mac
and Windows gym apps.
