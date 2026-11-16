# SLUG chain: handover brief for the coding agent taking over

Written 2026-11-15 by Ren (the previous assistant) at Mike Taylor's request.
You are taking over the build-out of the SLUG chain so it can proceed
without Mike relaying bundles and test runs by hand. Work directly in
Mike's clone: `~/slug/slug` on his Mac, origin `github.com/taytr-llc/slug`.

Start by reading, in this order:

1. `AGENTS.md` (in this package; Mike will place it at the repo root) —
   the standing rules. Everything in it is binding.
2. This brief — current state, remaining work spec, verification gates.
3. `BRAWLCHAIN_SPEC.md` — the product spec (67 settled decisions). The
   chain-relevant ones are summarized here; the fighter/league design
   matters when you reach the post-chain-core workstreams.
4. `BUILD_PLAN.md` — the overall build plan and workstream order.
5. `POW_SWAP_MAP.md` and `ARGON2ID_PARAMS.md` — how the PoW swap was done
   and why those parameters.
6. `CHANGESET_1_NOTES.md` … `CHANGESET_5_NOTES.md` — the per-set record.

## What SLUG is

A from-scratch cryptocurrency forked from btcd v0.27.0, built for a
fight-league product: miners run "gyms", block hashes deterministically
generate fighters, fighters compete in a weekly Saturday event. The chain
is product infrastructure, not a speculation vehicle: no premine, no ICO,
fighters (not coins) are the collectible. Coin and brand are both "SLUG".
The chain's job is to be a boring, correct, well-tested btcd with a
different PoW and parameters.

## Where the work stands (2026-11-15, evening)

Change sets 1–5 are built, verified, and pushed. The stacked branch tip is
`slug-rebrand` @ `0574b8b`. `master` is pristine upstream btcd @ `dd169fe`.

- **Set 1, PoW swap** (`slug-pow-swap`): `BlockHash()` in the wire module
  is Argon2id over the 80-byte header (header as password and salt,
  t=1, m=256 MB, p=1, 32-byte output). Proof test
  `TestCheckProofOfWorkArgon2id` mines nonce 1 to hash
  `5f42cf87f3acc67ff351adc85f0df64b4cc19e3e278fd46ce63c76511dc8d719`;
  it must produce this exact hash on any machine.
- **Set 2, params + genesis** (`slug-chain-params`): MainNetParams
  redefined in place as SLUG (full parameter list in AGENTS.md). Regtest
  and simnet keep inherited behavior; testnet3/4 and signet untouched.
- **Set 3, fixes** (`slug-chain-fixes`): retarget window set to 2,016
  blocks (`TestMainNetRetargetWindow`: no retarget at 2,015, retarget at
  2,016), and a startup fix: the block index loader builds nodes from the
  hash stored in each index bucket key instead of recomputing Argon2id
  per header (same pattern in `readBlockTree`). Restart on a 30-block
  chain went from 11.2 s to 162 ms.
- **Set 4, deploy** (`slug-deploy`): minimal multi-stage Dockerfile
  (golang:1.27-alpine build, alpine:3.24 runtime). Upstream's Dockerfile
  is rejected by Railway (VOLUME instruction); keep ours minimal.
- **Set 5, rebrand + test sweep** (`slug-rebrand`): daemon binary is
  `slugd`, control tool is `slugctl` (cmd/btcctl renamed), default home
  `~/.slug` with `slug.conf`/`slug.log`, `sample-slug.conf`, peer subver
  `/btcwire:0.5.0/slug:0.27.0/`, Dockerfile ENTRYPOINT slugd. All test
  suites that sets 1–2 left red were repaired with re-derived vectors and
  regenerated fixtures (ffldb block fixture retagged to SLUG magic;
  netsync header fixture and integration chaintips blocks regenerated as
  Argon2id chains). Green: btcutil (all), txscript, chaincfg, chainhash,
  wire, btcec, mempool, mining, peer, rpcclient, psbt, addrmgr, connmgr,
  btcjson, root package, database module, netsync full package (368 s),
  integration TestGetChainTips. NOT fully verified: the blockchain
  package suite (see set 9 below) and the rest of the integration module
  beyond TestGetChainTips.

A three-node network proof is done: Mike's Mac node, a Railway seed node,
and a VM node all agreed on the same first 3 blocks (tip
`42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188`). The
Railway seed now runs the rebranded slugd from branch `slug-rebrand`
(see AGENTS.md for the deployment coupling — read it before pushing
anything to that branch).

## Remaining chain-core work

### Change set 6: module path rename (next up)

Rename every module path `github.com/btcsuite/btcd` →
`github.com/taytr-llc/slug`. The 10 modules:

- root: `github.com/btcsuite/btcd`
- `github.com/btcsuite/btcd/address/v2`
- `github.com/btcsuite/btcd/btcec/v2`
- `github.com/btcsuite/btcd/btcutil/v2`
- `github.com/btcsuite/btcd/chaincfg/v2`
- `github.com/btcsuite/btcd/chainhash/v2`
- `github.com/btcsuite/btcd/psbt/v2`
- `github.com/btcsuite/btcd/txscript/v2`
- `github.com/btcsuite/btcd/v2transport`
- `github.com/btcsuite/btcd/wire/v2`

Mechanics: rewrite `module` and intra-repo `require` lines in all 10
go.mod files, every import path in every .go file, and references in
go.work comments if any. Then `go work sync` and per-module
`go mod tidy` as needed; expect go.sum churn for external deps to be
nil (paths of external deps don't change) — if go.sum changes beyond
removing btcd self-references, look twice. Watch for: string literals
that are not imports (test fixtures, the rpctest harness, doc examples);
`go generate` directives; the Dockerfile (it builds with explicit `-o`
names, so binary names are safe). Deliberate non-renames from set 5 stay
(AGENTS.md lists them).

Why it was deferred from set 5: it is a huge mechanical diff that makes
future upstream merges harder. Mike has now explicitly approved it. Do it
as one clean mechanical commit (plus a fixup commit if the build finds
stragglers), not mixed with any functional change.

Verification gate: all-module build clean, gofmt clean, and the same
green suites as set 5 (btcutil, txscript, database spot suites, netsync
fixture tests) plus the smoke test below.

### Change set 7: signed checkpoints

btcd supports checkpoints (chaincfg) and the "assume valid" pattern.
SLUG's design (spec decisions D62/D63): block results become final under
a signed public checkpoint; upgrades are opt-in. For the chain core, the
deliverable is: a checkpoint signing scheme for the league operator
(an Ed25519 key held by Mike, public key compiled into chaincfg),
checkpoints for the testnet chain at sensible heights, and node support
for verifying and reporting checkpoint state via RPC. Keep it minimal
and close to btcd's existing checkpoint machinery; the fighter layer
will anchor its own checkpoints on top later. Design decisions you are
not sure about go to Mike, not into the code by default.

### Change set 8: pre-genesis security review of the PoW swap

This is a review set, not a feature set. The swap touched consensus:
`BlockHash()` feeds block identity, the block index, merkle-adjacent
checks, and every "is this hash above target" comparison. Audit at
minimum: (a) every caller of BlockHash and Hash on headers for places
that assumed SHA256d cost or properties; (b) the difficulty/target
comparison path under a 256 MB hash (DoS surface: header-first sync and
orphan handling now cost 256 MB per untrusted header — quantify it and
check btcd's existing rate limits actually bound it); (c) the restart
rehash fix's assumption that stored index keys are trustworthy;
(d) Argon2id parameter handling (no path may accept attacker-controlled
memory/time parameters). Write findings as a notes file, fix what is
fixable in-scope, and list residual risks plainly. Do not mark this set
done with unexamined areas; an honest "not yet reviewed" beats a
rubber stamp. This review is the gate for the Phase C genesis.

### Change set 9: full blockchain suite pass

Add a test-only reduced-cost PoW configuration (build tag or test
helper override) that lowers Argon2id memory for test block grinding
WITHOUT touching production consensus code paths, then run the
`blockchain` package suite (and the rest of the integration module)
to completion and fix what surfaces. The knob must be impossible to
enable in a production build by accident: a build tag that the release
Dockerfile never sets is the expected shape. Success criterion: the
suite finishes and passes, and the notes record the wall-clock time
and any test that had to be adapted and why.

### Phase A closeout (after set 9, with Mike's go-ahead)

- Merge the stacked branch into master in one merge. Master stops being
  pristine btcd at that point, by design.
- Re-confirm Phase C genesis parameters with Mike before anyone mines
  the production genesis (the current genesis is a disposable Phase A
  testnet; the production genesis timestamp and coinbase text are
  Mike's call).
- Point the Railway seed at master (or its successor branch) and the
  production genesis when that day comes; until then it tracks
  `slug-rebrand` running the testnet.

## Smoke test (run after every change set)

1. Build: `go build -o /tmp/slugd .` and
   `go build -o /tmp/slugctl ./cmd/slugctl` from the repo root.
2. Fresh node: `/tmp/slugd --notls --rpcuser=u --rpcpass=p
   --datadir=/tmp/slug-smoke --rpclisten=127.0.0.1:29432
   --miningaddr=<any valid slug address>` (generate one with a throwaway
   wallet flow or reuse `amkchND9pP6STAzbb4M6U4SzU2anReC4R4`).
   `/tmp/slugd --version` prints "slugd version 0.27.0".
3. Mine 2 blocks via slugctl `generate 2`; confirm block 1 pays exactly
   10 SLUG.
4. Peer with the public seed: slugctl `addnode
   junction.proxy.rlwy.net:16023 add`, then `getpeerinfo` should show
   subver `/btcwire:0.5.0/slug:0.27.0/` and `getblockchaininfo` should
   converge to the seed's height (3 on the testnet as of handover;
   it only grows if someone mines to the seed's chain).
5. Stop cleanly: slugctl `stop` returns "slugd stopping."

## Beyond the chain core (context, not your first task)

BUILD_PLAN.md workstreams 2–9 build the product on this chain: the
fighter layer (fighter identity derived from block hashes, with
on-chain checkpoint anchors), the official mining pool (1% fee, daily
payouts, 1 SLUG minimum), a btcwallet adaptation, the gym client apps
(the miner reframed as a gym), league services (rankings, Saturday
fight-night scheduling), the marketplace, presentation, and pre-launch
legal. The settled design for all of it is in BRAWLCHAIN_SPEC.md.
Sequence them with Mike; do not start the fighter layer until the
chain core sets above are done or Mike explicitly reprioritizes.

## Working agreements with Mike

- Mike is terse and decisive; his one-line instructions are decisions,
  not conversation starters. When he delegates a judgment call, make it
  and record the reasoning in the notes file.
- Report results plainly: what passed, what failed, what is still
  unverified. Never report a suite as passing if it timed out.
- Purchases, payments, and anything touching wallet keys or seed phrases
  are out of scope for this repo's work entirely.
- The repo and the Railway project are Mike's; push branches, never
  force-push shared branches, never open PRs.
