# AGENTS.md — SLUG chain repo (fork of btcd)

This repo is **SLUG**, a fork of btcsuite/btcd (v0.27.0) being turned into a
standalone cryptocurrency: Argon2id proof-of-work, 2-minute blocks, 10 SLUG
block subsidy. It is the chain layer for a fight-league product (fighters are
derived from block hashes; that layer comes later — see HANDOVER.md and
BRAWLCHAIN_SPEC.md in the handover package).

Read HANDOVER.md before starting. It has the full state, the remaining work
spec, and the verification gates. This file is the short standing-rules
version.

## Branch discipline (do not break this)

- `master` is **pristine upstream btcd** and stays that way until Phase A
  (chain core) is declared done. Never commit to master.
- Work lands as numbered **change sets**, each on its own branch stacked on
  the previous one:
  `slug-pow-swap` → `slug-chain-params` → `slug-chain-fixes` → `slug-deploy`
  → `slug-rebrand` → `slug-module-rename` (set 6, `df655087`).
- Branch each next change set off the latest completed implementation tip,
  never the live seed branch by default. Use `slug-<topic>` names; set 7 is
  `slug-signed-checkpoints`. HANDOVER.md records the original takeover state.
- **Never open a pull request.** On a fork, GitHub's PR prompts can target
  btcsuite upstream. Push branches only.
- One change set = one branch = one (or a few) well-explained commits.
  Commit messages: short summary line, then a body that says what changed
  and why, including any deliberate non-changes.

## Build and test

- Go workspace: `go.work` is committed and resolves all 10 modules to local
  sources. Always build/test from the repo root in workspace mode.
- Full build: `go build ./...` at root covers the root module only. Build
  every module: root, address, btcec, btcutil, chaincfg, chainhash, psbt,
  txscript, v2transport, wire. A quick all-module check:
  `for d in . address btcec btcutil chaincfg chainhash psbt txscript v2transport wire; do (cd $d && go build ./...) || break; done`
- `gofmt -l .` must be clean (excluding nothing; upstream is formatted).
- Test suites are SLOW under Argon2id (256 MB per block hash): btcutil a
  minute or two, database ~4.5 minutes, netsync full package ~6 minutes.
  Slow is normal; a suite that is still hashing at timeout is not the same
  as a failing assertion. Report which.
- Known verification debt: the `blockchain` package suite has never run to
  completion under Argon2id (its test helper grinds blocks). Closing that
  is change set 9 (test-only reduced-cost PoW knob). Do not claim the
  blockchain suite passes until it has actually run end to end.

## Consensus facts (Phase A testnet, disposable but internally consistent)

- PoW: `wire/blockheader.go` `BlockHash()` is Argon2id over the 80-byte
  serialized header (header is both password and salt; time 1, memory
  256 MB, threads 1, 32-byte output). ONLY the header hash changed.
  Txids, merkle roots, checksums, and sighashes remain double-SHA256.
- Mainnet params were redefined in place as the SLUG network in
  `chaincfg`: subsidy 10 SLUG, halving every 1,050,000 blocks, 2-minute
  target spacing, retarget window 2,016 blocks, coinbase maturity 100,
  PowLimit 2^255−1, genesis bits 0x207fffff. Total issuance works out to
  20,999,999.8635 SLUG (integer-shift truncation, same as Bitcoin).
- Genesis: timestamp 2026-11-15 00:00:00 UTC, coinbase text "SLUG genesis,
  November 15 2026. Every fighter starts somewhere.", nonce 0, hash
  `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`,
  subsidy paid to an OP_RETURN burn output.
- Network: magic 0x534c5547 ("SLUG"), P2P port 29333, RPC port 29332,
  bech32 HRP "slug", P2PKH byte 0x53, P2SH 0x47, WIF 0xd3, HD private
  version 0x736c7567, HD public 0x534c5547. DNS seeds empty, Bitcoin
  checkpoints cleared.
- Do not change any consensus parameter or the genesis on an existing
  branch. Parameter changes are a new change set, and after the Phase C
  genesis they become coordinated upgrades, never silent edits.

## Test-vector rule (learned the hard way)

When adapting Bitcoin test vectors to SLUG encodings: decode the expected
string, swap ONLY the version bytes (or the bech32 HRP), re-encode with a
fresh checksum. Payloads (keys, hash160s, witness programs) stay
byte-identical to the canonical vectors. Never paste the code's own output
in as the expected value without independent verification (e.g. hash the
fixture header with `golang.org/x/crypto/argon2` directly in a throwaway
test). Exception already in tree: `TestCloneWithVersion` in hdkeychain
expects BITCOIN strings on purpose, because the test clones to explicitly
passed version bytes.

## Deliberate non-renames (change set 5)

Left as-is on purpose, do not "fix" without a change-set decision:
internal identifiers (btcdLog etc.), the "BTCD:" log subsystem tag, the
btcjson `"btcdconnected"` RPC notification method (wire protocol), and the
integration/rpctest harness's internal binary name. The Go module path
rename is change set 6 (HANDOVER.md has the spec).

## Deployment coupling (Railway seed node)

A public seed node runs this code on Railway (project
`successful-cooperation`, service `slug`). It tracks branch
**`slug-rebrand`**: any push to `slug-rebrand` auto-deploys. Its start
command is `slugd --notls --rpcuser=u --rpcpass=p --datadir=/data` with a
persistent volume at /data; public TCP endpoint
`junction.proxy.rlwy.net:16023` → port 29333 (RPC is not exposed).
Implications: never push a broken build to `slug-rebrand`; if a future
change set renames the binary again, the Railway start command must be
changed in the same deploy or the seed will not start. Deploying a
different branch means switching the service's source branch in Railway
settings.

## Signed checkpoint policy (set 7, Mike's decision)

- Signed checkpoints are report-only official league declarations. Conflicts
  are publicly reported; normal chain selection and validation continue.
  Never feed signed records into btcd's consensus checkpoints or fast-add
  shortcuts. Enforced finality requires a separate explicit decision.
- D62 burial means 60 descendant blocks, excluding the checkpoint block.
- Mike runs the offline key/signing tool himself. Never create, read, or
  request his operator private seed; only his public key is compiled into
  chaincfg. Test fixture keys are not operator keys. Until public-key
  enrollment, the client reports no configured authority and rejects signed
  checkpoint imports. Do not claim official testnet checkpoints exist yet.

## Style

- Keep diffs minimal and close to upstream btcd structure; this fork tracks
  upstream and every gratuitous divergence makes merges harder.
- Where behavior intentionally differs from Bitcoin, leave a short comment
  saying so and why.
- Write a `CHANGESET_<n>_NOTES.md` per change set (format: what changed,
  what was deliberately not changed, how it was verified, known debt).
