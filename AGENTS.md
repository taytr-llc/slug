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

- Mike explicitly authorized the Phase A master merge and seed deployment
  on November 15, 2026. The closeout merge ends master's pristine-upstream
  role; upstream btcd remains available at `dd169fe0c32a56087d28b92fff3b8cf4fb43477a`.
- `master` is the release/deployment branch after closeout. Develop changes
  on topic branches and validate them before promotion. Treat any push to
  master as a live deployment; obtain authorization for a new rollout.
- On November 16 Mike authorized continued work toward the desktop mining
  pilot and deployment/verification of completed sets as needed, without
  per-set rollout questions. That authorization persists: continue until a
  decision needs him, and raise an explicit blocking question in the chat.
  Do not claim push-notification delivery; notification controls are unavailable
  to this session. Existing private-key and consensus rules remain in force.
- Work lands as numbered **change sets**, each on its own branch stacked on
  the previous one:
  `slug-pow-swap` → `slug-chain-params` → `slug-chain-fixes` → `slug-deploy`
  → `slug-rebrand` → `slug-module-rename` (set 6, `df655087`)
  → `slug-signed-checkpoints` (set 7, `6b3e9e90`)
  → `slug-pow-security` (set 8, `c8176de5`)
  → `slug-test-pow` (set 9, `273b5e17`)
  → `slug-checkpoint-authority` (set 10, `0ca7466a`)
  → `slug-first-checkpoint` (set 11, `af67093f`)
  → `slug-phase-a-closeout` (set 12, released on master at `e056146f`)
  → `slug-fighter-dna` (set 13, released on master at `0f44da5f`)
  → `slug-prospect-proof` (set 14, released on master at `fdeebae2`)
  → `slug-prospect-registry` (set 15, released on master at `6443d163`)
  → `slug-trusted-prospect-context` (set 16, released on master at `e94c1cfa`)
  → `slug-gym-identity` (set 17, released on master at `7968e0a0`)
  → `slug-prospect-birth` (set 18, released on master at `c88abe11`)
  → `slug-prospect-service` (set 19, released on master at `56a4889d`)
  → `slug-mining-templates` (set 20, released on master at `3ed3b71e`)
  → `slug-automatic-miner` (set 21, released on master at `59e14a8c`)
  → `slug-desktop-pilot` (set 22, released on master at `fad9cbdf`)
  → `slug-wallet-foundation` (set 23, released on master at `e1b25e7f`)
  → `slug-wallet-recovery` (set 24, released on master at `0b04cd1b`)
  → `slug-phase-a-activation` (set 25, approved activation at height 62)
  → `slug-desktop-wallet` (set 26, released on master at `f03f1ef6`)
  → `slug-account-enrollment` (set 27, released on master at `a541fb58`)
  → `slug-identity-recovery` (set 28, released on master at `bc92fd53`)
  → `slug-recovery-vault` (set 29, encrypted fixture retention)
  → `slug-production-profile` (set 30, approved D73 profile design)
  → `slug-provider-coordinator` (set 31, fixture authentication/reset journal)
  → `slug-resumable-recovery` (set 32, bounded candidate wallet recovery)
  → `slug-fighter-manifest` (set 33, static candidate fighter previews)
  → `slug-fighter-portraits` (set 34, deterministic preview SVG)
  → `slug-fighter-condition` (set 35, exact candidate aging projection)
  → `slug-manifest-jobs` (set 36, candidate appearance-bound job codecs)
  → `slug-manifest-authority` (set 37, pinned candidate admission store)
  → `slug-idle-headers` (set 38, idle-chain history serving)
  → `slug-manifest-miner` (set 39, candidate service and durable miner)
  → `slug-manifest-desktop` (set 40, opt-in candidate fixture desktop)
  → `slug-gym-stable` (set 41, durable candidate stable and ten-slot roster)
  → `slug-fight-preview` (set 42, released on master at `fc672113`)
  → `slug-saturday-entry` (set 43, released on master at `e2761812`)
  → `slug-event-snapshot` (set 44, released on master at `1fc307e0`)
  → `slug-event-seed` (set 45, candidate seed contract under review).
- Mike requires a thorough joint design review before friends/tester or public
  product release (D74, November 16). Keep this gate in LAUNCH_READINESS.md;
  a solo review does not replace the session with him. He wants one full review
  at the end once all major flows are ready, rather than partial design reviews.
  Continue authorized engineering deployments, but do not treat them as product
  launch approval.
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
- Set 9 closed the blockchain/integration execution debt: complete tagged
  suites passed, with production vectors and smoke checked separately.
  Run `make test-chain` for both complete suites. The explicit `slugtestpow`
  build tag selects 64 KiB Argon2 memory and incompatible test identities;
  default/release builds retain 256 MiB. Never add that tag to release or
  Railway builds. Tagged daemons require regtest/simnet and use separate
  default files. No runtime flag enables cheap PoW.
- Set 7's signed checkpoint deliverable is complete: Mike's public key was
  enrolled in set 10 and his signed height-1 record is archived in set 11 at
  `checkpoints/phase-a/height-1.json`. Explicit local import reported finalized
  at tip 61 with 60 descendants. Mike authorized checkpoint-aware deployment
  as part of set 12. See `PHASE_A_CLOSEOUT.md` for the rollout and rollback
  configuration. Production genesis remains a separate explicit decision.

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
- Mike approved D71 on November 16: implement, validate and deploy the existing
  BIP34/65/66, CSV, SegWit and Taproot rules at Phase-A height 62 in set 25.
  Preserve genesis and blocks 0–61. Confirm the seed still has the recorded
  height-61 tip before promotion; stop if it has advanced. Update nodes before
  further mining. This does not authorize public-chain mining or production
  genesis. After height 62, a binary downgrade is not a sufficient rollback.

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
`successful-cooperation`, service `slug`). Phase A closeout switches its
source from `slug-rebrand` to **`master`**. Check the live Railway configuration
during the transition; afterward any push to master auto-deploys. Its start
command adds `--signedcheckpoint=/usr/share/slug/checkpoints/phase-a/height-1.json`
to `slugd --notls --rpcuser=u --rpcpass=p --datadir=/data`, with a
persistent volume at /data; public TCP endpoint
`junction.proxy.rlwy.net:16023` → port 29333 (RPC is not exposed).
Implications: never push a broken build to master; if a future
change set renames the binary again, the Railway start command must be
changed in the same deploy or the seed will not start. Deploying a
different branch means switching the service's source branch in Railway
settings. Preserve `slug-rebrand` at `0574b8b` as the rollback source;
rollback also removes the signed-checkpoint flag, which that older client
does not understand. Never detach, replace, or clear the /data volume.

## Signed checkpoint policy (set 7, Mike's decision)

- Signed checkpoints are report-only official league declarations. Conflicts
  are publicly reported; normal chain selection and validation continue.
  Never feed signed records into btcd's consensus checkpoints or fast-add
  shortcuts. Enforced finality requires a separate explicit decision.
- D62 burial means 60 descendant blocks, excluding the checkpoint block.
- Mike runs the offline key/signing tool himself. Never create, read, or
  request his operator private seed; only his public key is compiled into
  chaincfg. Test fixture keys are not operator keys. MainNetParams now uses
  his public key `811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3`.
  Other networks remain unconfigured. The first signed Phase A checkpoint
  is archived and verified; load it explicitly using `--signedcheckpoint`.
  Publishing the file does not automatically configure every node.

## Style

- Fighter DNA is a separate off-chain component in `fighter`; its exact
  versioned byte contract is in `FIGHTER_DNA.md`. Keep independent reference
  vectors independently derived, preserve old DNA versions, and do not treat
  a preview hash as qualifying work or an ownership claim. Complete fighter
  generation and registration are subsequent work, not node consensus.
- Prospect proofs live in `prospect` and use `PROSPECT_PROOFS.md`'s exact
  byte contract. Verification is conditional on separately trusted context;
  never accept claimant-chosen targets or mistake a proof for account
  authentication, registered ownership, discovery time or full block validity.
  Production proof verification must reject the cheap test PoW profile.
- The local `registry` is conditional evidence storage, not ownership or a
  birth clock. Preserve first evidence and stable IDs across restart and
  later status changes; never reset age by resubmission. Use a separate
  explicitly initialized directory, synchronous atomic writes, and fail
  closed on corruption. A write error has an uncertain commit outcome;
  reopen before retrying. See `PROSPECT_REGISTRY.md`.
- `prospectnode` resolves current-parent context from an explicitly trusted
  node and policy. Online verification rechecks the node after hashing;
  guarded admission checks before writes and duplicate acknowledgment.
  Never describe this observation as permanent canonicality or fighter birth.
  Never accept a claimant-selected policy or expose RPC credentials in output.
  See `TRUSTED_PROSPECT_CONTEXT.md`; stored receipts remain supplied-context.

- `gymauth` binds self-certifying gym IDs to wallet-compatible secp256k1 keys
  and signs only a specific prospect admission. Signing is automatic client
  work, never a player prospect-selection step. CLI tools accept public inputs
  only; use fixture keys for tests, never real wallet or operator keys. Explicit
  schema-2 registries require and atomically persist authorization; do not
  upgrade schema-1 evidence or arbitrary gym IDs implicitly. Key control does
  not prove an account, ownership transfer, birth, or permanent canonicality.
  See `GYM_AUTHORIZATION.md` for exact bytes and integration boundaries.

- D68 jobs last exactly five minutes. Age starts at durable issuance; a new
  admission at or after expiry is rejected without grace. Job IDs/challenges
  enter proof-v2 work before hashing. Only explicitly created schema-3 or schema-4 stores
  issue and admit jobs; no retrospective birth is attached to older evidence.
  Keep accepted birth immutable across duplicate recovery, expiry and reorgs.
  Historical receipts do not assert renewed eligibility. Persist clock high
  water even for expiry rejections, fail closed on rollback/uncertain writes,
  and never accept claimant time or nonce on authority APIs. Client handling is
  automatic under D44. See `PROSPECT_BIRTH.md` and spec decision D68.

- `prospectservice` is a loopback-only authority boundary with one active
  operation, bounded connections/bytes/rates and durable configured capacities.
  API credentials bind access to a public gym key but never replace admission
  signatures. Preserve historical receipts after uncertain acknowledgments,
  drain real work before closing storage and never reopen a poisoned handle
  automatically. `slugregistryd` is not started by seed deployment. See
  `PROSPECT_SERVICE.md`. Protocol v1 requires schema 3; v2 requires explicit
  schema 4. Never silently change old eligibility or present context-only jobs
  as full templates.

- D69 broadcasts block-target wins immediately, before prospect signing or
  admission. Only the exact winning block on the canonical chain can use the
  stale-parent exception, within the original D68 lifetime. Authority guards
  take internally verified work, never claimant-provided hash summaries. Keep
  admission-time eligibility immutable with birth. The worker synchronizes
  found work before transport, and public authorization before admission;
  uncertain outcomes retain identical evidence for recovery. Retained-capacity
  exhaustion pauses mining, never drops pending work or receipts automatically.

- The desktop pilot has its own module/workspace under `desktop`; keep the
  ten-module chain workspace and Railway node build independent. Its fixture
  gym mines only a disposable loopback chain, with no production wallet or
  public-node option. Do not describe public receipt export as wallet export
  or hosted login. Mining requires an explicit Start; Quit drains owned work
  and processes. Real wallet custody, D48 export, calibrated policy and release
  signing remain separate integration gates. Never read operator/wallet keys.
  Set 26 uses a separate `LocalWalletPilot` v2 namespace and the published
  zero256 wallet fixture. Ordinary address/balance/history operations use actual
  btcwallet RPC with an owned minimal allowlist; signing/export stay inside the
  helper. Native dialogs select recovery paths, never the webview. Restore
  verification is asynchronous and excludes mining. Preserve old `LocalPilot`
  data; never adopt pre-upgrade cached chains or substitute a new wallet/gym
  when established data is missing. Runtime checks require activation height62,
  since the node's unchanged version string is insufficient. The explicit
  `--localwalletpilot` node option is restricted to connect-only literal
  loopback networking and RPC; it changes only old-tip template readiness.
  Wallet rescan readiness has the same private-pilot boundary. Never enable
  either path for the seed or replace the worker/job/outbox flow with hidden
  bootstrap mining. All consensus/proposal/PoW checks still apply.
  Mike confirmed the gym shell is new Go work against the slugd/btcwallet RPC
  surfaces, not a Paymetheus adaptation. Fork the separate btcwallet repository
  when its wallet workstream begins.
  That fork now exists at `taytr-llc/slugwallet`, checked out beside this repo.
  Its first set 23 branch starts from btcwallet v0.18.1. Keep node deployment
  separate from wallet work; no wallet service starts on Railway with this set.
  Mike approved D70: 24 English BIP39 words, no extra BIP39 passphrase, with
  versioned SLUG wallet/gym recovery metadata. Login/encryption passwords remain
  separate. New recovery profiles use explicit standard BIP32 derivation;
  existing wallets keep legacy derivation. The Phase-A fixture profile is not a
  production path allocation. Verify actual later export and fresh restore,
  never invent a phrase for an old raw-seed wallet or imply hosted login exists.
  Mike approved D72 on November 16: a verified password reset restores access
  to the same hosted wallet without the 24 words, with a post-reset security
  hold. Record this as closing D70's hosted-reset authority question. The
  service retains recovery authority; backup export does not revoke it. For
  24 hours after each verified reset, block withdrawals, fighter transfers,
  recovery phrase/file export, recovery-authority changes, marketplace purchases
  and listings. Viewing and mining into the same wallet remain available.
  Revoke existing sessions immediately. A distinct further reset restarts the
  hold; replaying the same verified event must not extend it.
  Set 27 starts with two explicitly published fixtures, not real registration,
  production derivation allocation, or a public wallet service.
  Set 28 adds an explicit schema-2 assertion/reset boundary. Preserve schema-1
  data and APIs; never allow legacy login/reset to bypass schema 2. Pin verifier
  trust, consume authority-issued nonces, and bind the signed credential epoch
  to the original authentication. Refresh issuance time is not fresh auth. A
  verified reset must establish its durable barrier before provider credential
  completion; pending state never clears merely on timeout or restart. Real
  provider deployment, arbitrary signup and production custody remain separate.
  Set 29 retains recovery and gym-signing material in separate purpose-bound
  envelopes in the wallet repository. Held gym operations must never unwrap
  recovery material or export a seed, scalar or data key. Full provisioning
  requires fresh authentication with no active hold; interrupted reservations
  keep their original identity. Keep provider I/O outside account/vault leases
  and reauthorize the original token/generation at publication. The initial
  fixture manifest is immutable, not a current address-cursor backup. There is
  no production KMS adapter, new-device wallet UI or real key issuance in this set.
  Mike approved D73 on November 16: a separate production recovery profile with
  dedicated SLUG coin type, four payment scopes, native84 receive/change defaults
  and a permanent-chain-bound gym key. Preserve all existing fixture and legacy
  paths/schemas. The exact proposed contract is in PRODUCTION_WALLET_PROFILE.md.
  Final coin allocation, descriptor bytes and permanent genesis remain issuance
  gates; a caller-provided finalized flag cannot bypass them. Use only explicit
  public fixtures until those gates are settled. Approval does not provision
  provider/KMS resources or authorize reading/creating real wallet keys here.

- Keep diffs minimal and close to upstream btcd structure; this fork tracks
  upstream and every gratuitous divergence makes merges harder.
- Where behavior intentionally differs from Bitcoin, leave a short comment
  saying so and why.
- Write a `CHANGESET_<n>_NOTES.md` per change set (format: what changed,
  what was deliberately not changed, how it was verified, known debt).

- Set 31's `gymprovider` accepts only the pinned published fixture issuer/key.
  Keep original-authentication epochs and provider-operation identities immutable.
  A reset may allow provider credential completion only after a synced, exact
  pending authority barrier; historical completion never grants another permit.
  Do not invent session replay after a lost acknowledgement or unlock abandoned
  pending resets on timeout. Keep JWTs/session tokens out of the journal. Real
  provider guarantees and product login integration require separate evidence;
  the Auth0-shaped fixture is not a deployed adapter. See `GYM_PROVIDER.md`.

- Set 32 uses explicit candidate wallet schema 11; never silently migrate
  schemas 8/9/10 or enable real issuance. Pending sessions exclusively own
  storage and cannot sign or spend. Preserve issued cursors across cancellation,
  restart, notification failure and reorg; unused lookahead is not issuance.
  Keep the aggregate 65,536-attempt quota and bounded node/watch requests.
  `RESUMABLE_RECOVERY.md` defines the scanner and ready-wallet limits; the
  fixture vault still does not track an evolving wallet snapshot.

- Set 33's `fightermanifest` preserves complete DNA v1 while adding versioned
  candidate names, physical traits, style and portrait selections. Keep the
  frozen tables, strict canonical codec and independently derived fixtures.
  A preview is not an official appearance assignment or fighter identity;
  current jobs bind only DNA version. Official assignment requires explicit
  issuance/version binding or an immutable committed mapping. Never change
  historical receipts or birth, or replace the existing DNA CLI route.

- Set 34's `fighterportrait` validates the complete manifest before drawing.
  Preserve exact versioned SVG bytes, pinned tables and independent reference
  fixtures. No uploaded markup, external assets, free text or mutable palettes
  enter rendering. Byte determinism does not imply cross-browser pixel identity
  or screen-reader verification. Portrait previews still carry no official
  assignment, identity, ownership or birth claim.

- Set 35's `fightercondition` is a pure projection of complete canonical DNA,
  supplied birth/time and already-accounted XP. It authenticates none of those
  external facts and grants no admission, ownership or roster eligibility.
  Keep exact duration weeks, nearest-even integer tenths, no added attribute
  floor, XP after aging and the D65 rating without training/form. The caller's
  later trusted event boundary must supply immutable birth and XP.

- D75 (Mike, November 17) keeps D65 unchanged and rescinds set 42's exact
  rounded-coefficient equality gate. Preserve the failed original report and
  contract; do not relabel them passed. Rating replacements require a positive
  paired bootstrap interval excluding zero and population validation covering
  prime, aging and XP-varied fighters, including close pairings. The incumbent
  wins ties or inconclusive comparisons. Run and publish coefficient-refitting
  bootstrap and population-coverage studies on the unchanged set 42 engine.
  These studies do not authorize new weights, condition v2, historical rewrites
  or gameplay rebalancing. See `RATING_VALIDATION_POLICY.md`. Other engineering
  and D74 product-release gates remain in force.
  The completed independent studies are in `RATING_VALIDATION_REPORT.md`:
  all eight primary paired improvement intervals include zero. Preserve both
  that evidence and the original failed equality result; D65 remains unchanged.

- D76 (Mike, November 17) freezes roster membership, ownership, eligibility
  and current condition at Saturday noon Pacific. Later roster changes,
  transfers or retirement do not alter that event. Keep birth/live aging and
  D39's immediate title vacancy intact. Candidate assignment is not proof of
  transferable production ownership; official snapshots need that authority.
- D77 (Mike, November 17) waits with visible delay/conflict, never substitutes
  a seed and never automatically cancels the event. If the required seed is
  unfinalized at the following Saturday noon-Pacific lock, escalate to the
  operator as a chain incident under checkpoint/community policy. Use calendar
  boundaries through DST, preserve original event/seed evidence, and keep node
  checkpoints report-only. No synthetic finalized flag or preview seed can
  stand in for the later trusted seed/finality authority. See EVENT_LOCK_POLICY.md.

- Set 39 adds explicitly pinned candidate service protocol 3 and a separately
  typed worker/outbox version 2. Preserve old services, workers and journals;
  never infer trust from a marker or silently migrate old evidence. Bind the
  outbox to origin, full authority pin and transport trust identity without
  persisting credentials. Found work, authorization and receipts synchronize
  before their respective transport/acknowledgment boundaries. Reserve remaining
  record growth so full capacity cannot prevent recovery of retained work.
  Cancellation joins real work; historical recovery still requires a live
  account session and preserves original birth/binding. See MANIFEST_MINING.md
  and its separate reservation addendum. No official descriptor activation,
  ownership authority, public receipt import or default desktop route is implied.

- Set 36 separates structural appearance descriptors from compiled candidate
  support and production activation. Production resolution has no entries.
  Keep Job v1 and its readers unchanged; the separate ManifestJob v2 binds
  an authority namespace and exact descriptor before work. Neither field
  authenticates issuance. BuildManifest is a typed unmined-candidate helper,
  not an admission path. No schema migration, current-service activation or
  historical appearance assignment is implied. See MANIFEST_JOBS.md.

- Set 37's separate ManifestStore requires an explicit schema-5 authority pin.
  Unsigned marker inspection supports provisioning recovery; it does not make
  its output trusted configuration. Never migrate schemas 1–4 or accept caller
  receipt JSON as authoritative. Generate the static binding from internally
  verified work and commit it with original birth/eligibility in one synced
  batch. Historical reads retain their original facts through expiry/reorgs;
  they do not establish current ownership. Production activation stays closed.
  See MANIFEST_ADMISSION.md for strict bounds, poisoning and read guarantees.

- Set 38 separates serving known headers from deciding that the chain is fresh.
  Keep the existing bounded LocateHeaders semantics, including the special
  empty-locator indexed-header lookup. Serving headers is not a claim that a
  node is current or that every indexed header has a validated block body.
  Do not relax chain currentness, mining templates, transaction relay, compact
  filters, proof verification or consensus to make an idle seed synchronizable.

- Set 40 adds a separate Candidate Pilot bundle and explicit helper mode with
  its own LocalManifestPilot namespace. Preserve all earlier app/data routes.
  Login cold-verifies at most eight retained records while stopped; only Start
  mines. Live cards follow fully verified, synchronized acceptance, never a
  claimant receipt or an uncommitted response. View credentials and generation
  remain bound to the original login through cache reads, portraits and exports.
  Never substitute a new account session during a pending operation. Public
  receipt export remains distinct from recovery export and uses an exclusive
  native-selected destination. See MANIFEST_DESKTOP.md and its packaging and
  acceptance addenda. The candidate build does not activate an official
  descriptor, ownership, real accounts, product distribution or D74 approval.

- Set 41 consumes verified records from one concrete pinned ManifestStore into
  a separate candidate assignment authority. Assignment, consumed sequence and
  clock high water synchronize together; roster membership never controls
  inventory retention. Ten reversible slots use durable prepare/apply operation
  identities and revision checks. Preserve original birth and XP=0 until an
  explicit trusted results authority exists. Current held sessions may make
  reversible roster edits; D72's asset/recovery/market restrictions stay intact.
  Keep bounded serial verification, original account leases, uncertain-outcome
  reconciliation and fixed-page versus Follow Recent behavior. The existing
  candidate bundle uses the same account root with a recoverable explicit v2
  upgrade; missing established components fail closed. No public receipt import,
  transfer, discard, entry lock, training, official descriptor or real account
  is enabled. See GYM_STABLE.md and its engineering addenda.
