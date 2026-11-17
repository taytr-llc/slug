# SLUG: Build-Out Plan

Consolidated November 15, 2026. The rules live in BRAWLCHAIN_SPEC.md;
this is the work list: what gets built, in what order, and what each
piece depends on. Decision numbers point back to the spec.

## Workstreams

### 1. Chain core
Fork btcd (D66), swap proof of work to Argon2id at 256 MB, t=1, p=1
(D50), set the D32 parameters: 21M cap, 10 SLUG starting reward,
halving every 1,050,000 blocks, 2-minute blocks, fees to the miner.
Includes difficulty tuning for a slow hash (share difficulty, genesis
difficulty), report-only signed league checkpoints, public seed nodes,
and an adapted open-source Bitcoin explorer. A security review of the PoW swap
before genesis is non-optional (pre-launch list, spec section 17).
Depends on: nothing. Everything else depends on this.

Set 7 clarification (Mike, November 15, 2026): signed checkpoints declare
the official record and report conflicts; they never reject branches or
skip validation. The earlier fast-sync-anchor proposal is superseded.
Argon2id sync cost remains work for the security/performance review.

### 2. Fighter layer
Built off-chain with on-chain checkpoint anchors, per the chain
evaluation. Fighter generation from the hash (DNA: attributes, name,
portrait, physical traits), prospect registration from a gym's own
shares under both pool and solo mining (D35), ownership and transfer
records, pro and exhibition records, the D65 rating, aging and
retirement state (D21, D22). Publication formats for entries, results,
and ownership changes are build-time items (spec section 17).
Depends on: chain core (hash and checkpoint formats).

Current first implementation slice: set 13 provides the versioned off-chain
DNA component and offline preview (`FIGHTER_DNA.md`), covering talent tiers,
base attributes and career lengths. It does not yet register prospects,
establish ownership, or provide names, physical traits or portraits. The
share-proof/ownership contract must precede real gym prospect registration.

Set 14 follows with a bounded proof-of-work verifier and public commitment
tool (`PROSPECT_PROOFS.md`). One pre-work coinbase commitment binds a header
to a gym identifier and DNA/target policy. Offline verification takes a
separately trusted chain context. Durable registration, account control,
birth/freshness policy, replay storage and transfers remain later work.

Set 15 adds a durable local evidence registry (`PROSPECT_REGISTRY.md`) with
immutable first receipts and persistent duplicate IDs. It re-verifies raw
proofs and preserves evidence across restart. This is the storage foundation;
authenticated registration, birth/freshness and account/transfer authority
remain required before gym enrollment.

Set 16 supplies current-parent context from an explicit trusted node and
prospect policy (`TRUSTED_PROSPECT_CONTEXT.md`). Online verification and
guarded registry admission recheck the node after hashing. This replaces
manual chain assertions on the online path; it does not establish birth,
account control, permanent canonicality, or a calibrated production target.

Set 17 adds proof-specific gym-key authorization and explicit authenticated
receipt schema 2 (`GYM_AUTHORIZATION.md`). Set 18 adds D68's five-minute
pre-work jobs and immutable issuance-based birth in schema 3
(`PROSPECT_BIRTH.md`). Original evidence schemas remain unchanged. These are
library and command foundations: end-user automatic job renewal/mining,
wallet/login and tested key export, public admission service, calibrated
targets, ownership/transfer state, status history and publication still need
their own implementation slices. The birth clock trusts the issuing authority's
clock and durable records; it is not an exact mined timestamp or consensus rule.

Set 19 adds the bounded local authority daemon and validating client transport
(`PROSPECT_SERVICE.md`, `ADR_001_PROSPECT_SERVICE.md`). It retains a schema-3
registry across requests, binds API access to gym keys and recovers immutable
receipts after lost acknowledgments. This is a private integration boundary,
not a deployed public enrollment service. Next, the miner needs complete payable
block templates and explicit ordering for prospect admission versus block wins;
the existing context-only jobs do not contain all transaction/coinbase data.

Set 20 supplies complete payable candidates, matching proof-v2 evidence and
nonbroadcast node proposal validation (`PROSPECT_WORK.md`). It preserves all
template transactions and witness data and takes an explicit local payout
address. The next milestone is a desktop mining pilot: automatic renewal and
mining with durable retries, wallet/login plus tested full export (D48), and
start/stop/status/roster UI. The shell can be developed with clearly labelled
fixtures while real wallet and worker integration proceed.

Mike settled the ordering as D69 on November 16: broadcast block wins immediately;
the exact canonical winning block can qualify within the original five-minute
job. Set 21 implements this in explicit registry schema 4/service protocol v2,
with a bounded automatic worker and synchronized public outbox. Old registries
retain their original eligibility rules. The worker integrates an in-memory
signer; it does not provide hosted wallet custody, login or full seed export.
The next desktop slice wires start/stop/status/receipts to that worker behind a
narrow Go bridge, with fixture identities visibly separated from real accounts.

Set 22 prepares a new Go desktop shell and separate Go supervisor using Wails.
Mike confirmed on November 16 that no existing desktop app is the base;
Paymetheus is not involved. The gym integrates against the slugd and btcwallet
RPC surfaces. btcwallet remains a separate repository to fork when the wallet
workstream begins.
It exercises actual production PoW and durable admissions on two disposable
loopback-only nodes, with a public test gym identity. Start/stop, menu-bar/tray
lifecycle, saved prospects and public receipt export are the initial interface.
This is a local pilot, not the hosted account/wallet promised by D48. Real login,
btcwallet custody, full tested seed export and public service provisioning are
the next integration boundary; they must not be implied by a fixture roster.

Set 33 adds the candidate static manifest and optional `slugfighter preview`
route (`FIGHTER_MANIFEST.md`). It preserves DNA v1 and independently derives
names, physical traits, style and portrait selections under explicit versioned
tables. The supplied simulator and HTML prototypes are now preserved in the
workspace; the spec remains authoritative. This is a preview library/CLI, not
an official assignment to admitted fighters. A bounded deterministic portrait
renderer, explicit issuance/version binding, roster/lifecycle history and the
fight engine remain subsequent slices.

Set 34 implements the exact candidate SVG renderer and `slugfighter portrait`
command (`FIGHTER_PORTRAITS.md`). Canonical manifest validation precedes all
lookups; exact integer coordinates, bounded fixed markup and an independent
Python oracle make bytes reproducible. A local review gallery is available,
but browser visual and assistive-technology checks remain open. There is no
desktop portrait integration or official assignment in this library slice.

Set 35 supplies a pure candidate current-condition projection and CLI
(`FIGHTER_CONDITION.md`). It applies completed duration weeks of specified
decline, retains retired exhibition condition, adds accounted XP after aging
and computes the D65 rating with exact integer arithmetic. Trusted receipt
birth, completed-fight XP accounting, current ownership and ten-slot roster
selection still need integration; supplied preview timestamps prove none of
those facts. The visual gallery check remains pending at Mike's request.

Set 36 adds the candidate static descriptor and separately typed manifest-bound
Job v2 (`MANIFEST_JOBS.md`). The exact immutable descriptor and public authority
namespace enter the job commitment before work; the typed builder produces an
unmined payable candidate without changing old jobs or proof-v2 encoding. The
production descriptor catalog is empty. The next admission slice must durably
pin an explicit authority/descriptor, preserve issuance-based birth and attach
immutable static binding through trusted receipt reads. Current service,
worker/outbox and existing registry schemas remain unchanged in set 36.

Set 37 supplies the separate candidate schema-5 authority boundary
(`MANIFEST_ADMISSION.md`): explicit pinned opening, durable manifest-job issuance,
atomic static binding and bounded fully verified historical receipt reads.
It preserves D68/D69 and all older stores. Service/worker transport, current
ownership/roster state and portable authenticated publication remain separate
integration steps; unsigned receipt JSON does not establish authority provenance.

Set 39 connects that candidate authority to a bounded protocol-3 client/service
and a separately typed automatic worker with durable outbox version 2
(`MANIFEST_MINING.md`, `MANIFEST_MINING_RESERVATION.md`). Explicit authority and
transport pins survive restart; synchronized work and saved authorizations allow
historical receipt recovery after lost acknowledgments, expiry and node outage.
The fixture account/vault composition exercises reset revocation and permitted
gym-only mining during the hold. This does not activate the production generator
or change the existing desktop route. The next desktop slice can present these
candidate receipts and portraits while ownership, roster/lifecycle authority,
portable publication and real provider/custody remain separate requirements.

### 3. The official pool
The company-run pool that earns 1% (D35, D36). No turnkey pool exists
for btcd plus Argon2id, so this is a real build: adapt a Miningcore
fork (or NOMP), add an Argon2id share-validation hasher (libargon2;
Blocknet pools prove the pattern), pool vardiff tuned to Argon2id
attempt times, proportional payouts by shares, daily payouts with a
1 SLUG minimum, the 1% fee collected by the company, and public
on-chain payout reporting. The pool also attests pool miners'
prospect shares to the fighter layer. Solo mining needs no pool work;
it is served by the chain and the gym clients directly.
Depends on: chain core (stratum surface), fighter layer (prospect
attestation format).

### 4. Wallet
btcwallet from the btcd suite. Hosted wallet at signup with full
export at any time (D48). Signing lives in the clients and on Mike's
accounts, never on the assistant's machines.
Depends on: chain core.

Set 23 begins in the separate `taytr-llc/slugwallet` fork, pinned to btcwallet
v0.18.1 (`43f2950d`). The first slice adapts its six runtime modules to SLUG and
the full-node RPC backend, with explicit network/genesis checks, private wallet
RPC and independent fixture receive/spend/recovery tests. The desktop pilot
continues to use its labelled public fixture signer during this work. Neutrino
and Bitcoin Core backends are outside the initial contract.

Review also found that the inherited RPC connection handshake could delay
startup cancellation for over a minute. Set 23 adds a backward-compatible
context-aware connection API in the chain client library so the wallet can
bound its whole connection/identity check, including retry waits. This changes
client lifecycle behavior, not consensus or the seed's RPC exposure.

The upstream wallet does not supply D48's later recovery-phrase reveal or SLUG
admission signing. Normal login, protected recoverable seed material, fixed gym
key derivation and full export/restore acceptance remain required before real
wallet issuance. Restoring a published test seed verifies a foundation only;
it does not demonstrate a user-facing recovery export or fighter transfer.

Set 24 implements D70 in the separate wallet fork: encrypted retention of the
approved 24-word English phrase material, later export, fresh restoration and
the same wallet-derived gym identity. Its explicit Phase-A fixture profile uses
standard BIP32 while old wallets retain legacy derivation. A narrow automatic
admission signer joins the existing worker callback without exposing a raw-key
or arbitrary-signing RPC. Normal-PoW acceptance must restore from the actual
later export, recover beyond a default address gap, spend again and authorize
a prospect with the same gym. Hosted login, custody recovery, production
derivation allocation and fighter transfer remain separate gates.

Set 25 follows Mike's D71 approval: activate the existing script upgrades at
Phase-A block 62, preserve all earlier history, and prove funded native, nested
witness and Taproot recovery spending plus complete witness mining candidates.
Deploy only after boundary/reorg validation and a fresh height-61 history check.
Public mining and production genesis remain separate from this upgrade.

Set 26 integrates that fixture wallet into the Go gym shell through the
btcwallet RPC surface. It retains one wallet-generated payout and fixed gym,
shows test reward maturity, and adds native recovery export plus asynchronous
file-only restore verification. New manifest/data roots reject old cached
chains and preserve set-22 receipts. Only published fixture keys are used;
real custody/login, production key/path allocation and release signing remain
separate work. See `desktop/README.md` and `CHANGESET_26_NOTES.md`.

### 5. Gym clients
Set 27 implements the approved D72 reset contract in a private two-fixture
account pilot: verified reset restores the same wallet without the words,
revokes old sessions, and starts the fixed 24-hour hold. The hold blocks
withdrawal, fighter transfer, recovery export, recovery-authority changes and
marketplace purchase/listing capabilities; viewing and mining remain allowed.
Durable account enrollment uses a distinct narrow wallet signature. Production
provider/custody design and derivation allocation remain separate from this
mock-provider acceptance. See `GYM_ACCOUNTS.md` and `CHANGESET_27_NOTES.md`.

Set 28 builds the provider assertion and reset-ordering boundary, with pinned
RS256 verification, consumed login nonces, original-authentication epochs and a
durable pending-reset barrier. The two existing wallets exercise original-gym
recovery and mining under the hold; the set-27 desktop default remains intact.
A later provider adapter must demonstrate this reset ordering before deployment.
Production wallet profile, vault, real signup and provider provisioning are
separate from this component. See `CHANGESET_28_NOTES.md`.

Set 29 adds an encrypted fixture recovery vault in the separate wallet repo.
Separate purpose-bound envelopes let original-gym enrollment and mining use
only gym signing material during a reset hold. Full-material provisioning,
export and wrapping-key changes enforce their own current session/hold gates.
Durable reservations and reconciliation preserve the original wallet across
interruptions. The desktop's private signer adapter checks the original mining
session around the vault call without changing existing app defaults. This
initial manifest is immutable; current address metadata, watch-only new-device
views, production issuance and real provider/KMS deployment remain work ahead.
See `CHANGESET_29_NOTES.md` and wallet `RECOVERY_VAULT.md`.

Mike approved the separate production profile as D73 on November 16. Set 30
implements its descriptor, derivation and explicit candidate-fixture recovery
while preserving every existing wallet/profile. Native84 receive/change defaults,
dedicated coin paths and the permanent-chain-bound gym are the approved design.
Final allocation/genesis and exact frozen descriptors still gate real issuance.
See `PRODUCTION_WALLET_PROFILE.md`; the remaining launch work is tracked in
`LAUNCH_READINESS.md`. Set 31 adds fixture provider coordination, independent signed evidence and
durable reset replay; see `GYM_PROVIDER.md`. This does not enable real signup.

The gym is the product surface: native Mac and Windows apps (menu
bar and tray, gentle default mining setting) plus the browser
version. Miner integration with the pool and solo switch, roster and
Saturday entry management, stable and prospect views, training and
exhibitions, team gym membership and standings, marketplace access.
Signing and notarization run through Mike's developer accounts.
A working interface prototype exists (SLUG Gym space, November 15,
2026); visual design polish is deferred by Mike's call.
Depends on: fighter layer, pool, wallet.

Set 40 adds the separate opt-in Candidate Pilot: two published wallet fixtures,
automatic live candidate cards/portraits after explicit Start, bounded stopped
historical verification, and public receipt export. It retains the original
account app and uses a separate data namespace. This is a runnable integration
slice of the new Go gym shell, not real signup, official roster/ownership or
product launch. See `MANIFEST_DESKTOP.md`; the packaging and acceptance addenda
record the separate bundle default and precise composite lost-ACK evidence.

Set 41 extends that same Candidate Pilot with a durable assigned inventory,
trusted birth/current condition and ten reversible roster slots. Bounded
catch-up consumes the pinned admission store; the original receipt remains
the source of identity and birth. Slot edits use durable operation identities,
revision checks and explicit reconciliation after a lost acknowledgment. Fixed
inventory pages stay selected while Follow Recent resumes automatic arrivals.
The two existing fixture accounts retain their wallets across the recoverable
account-root upgrade. This candidate assignment authority does not establish
transferable production ownership. XP remains zero because professional results
have not yet been implemented. See `GYM_STABLE.md` and its addenda. Entry locking,
fight inputs/results and training accounting are the next league integration
boundary; visual acceptance and the joint D74 review remain required.

Set 42 implements the first unrecorded same-gym fight preview: a pure deterministic
three-round engine, a trusted same-class pair read at one observation time, bounded
async helper work, native portraits and local transcript replay. It records no
fight, XP, training or entry. D75 retains D65 and replaces the original exact-refit
equality rule with evidence requirements for future rating changes. Its required
coefficient-stability and fresh population-coverage studies passed independent
audit, with no demonstrated challenger superiority across the eight primary
comparisons. D75 successor acceptance retains D65. Exact two-wallet packaged
acceptance, normal-PoW smoke, deployment and the full live observation passed.
See `FIGHTER_BOUT.md`, `FIGHT_PREVIEW.md`,
`RATING_VALIDATION_POLICY.md` and `RATING_VALIDATION_REPORT.md`. Future recorded exhibitions
and professional results still need their own seed, event and award authorities.

Set 43 implements reversible Saturday entry intake in the same candidate
gym: explicit selection of up to five rostered fighters, one per class;
save/replace/withdraw, durable recovery and history; and authority-observed noon
Pacific closure. It does not lock entrants, choose seeds, book fights or award
results/XP. The frozen contract and independent calendar/protocol/state corpus
are in `gymentry/testdata/reference`. Source checks, exact Mac bundle acceptance,
independent verification, normal-PoW smoke and deployment at master `e2761812`
passed; the full 900-second/31-sample live observation and restart passed.
See `GYM_ENTRIES.md` and `CHANGESET_43_NOTES.md`.

Set 44 implements the bounded candidate event-snapshot foundation on
`slug-event-snapshot`. Mike approved D76: freeze roster membership, ownership,
eligibility and current condition at noon Pacific; later changes do not alter
that event. D77 requires visible wait/conflict with no substitute seed or
automatic cancellation, and chain-incident escalation if the required seed
remains unfinalized at the following Saturday lock. See `EVENT_LOCK_POLICY.md`.
The shared coordinator reconstructs both gyms from committed history, separates
noon inputs from actual capture evidence, and recovers interrupted captures.
Its frozen independent reference is in `gymevent/testdata/reference`; normal,
race, crash-recovery and scoped-clock desktop checks have passed. Exact Mac
bundle acceptance, independent verification and deployment at master `1fc307e0`
passed, followed by the full 900-second/31-sample live observation and restart.
Successful real-clock closed-bundle coverage awaits an eligible Saturday.
See `EVENT_SNAPSHOTS.md` and `CHANGESET_44_NOTES.md`.
Transferable ownership and actual anchor/seed/finality integration
remain required before official use; fixture assignment is not that evidence.

Set 45 is in contract review on `slug-event-seed`, stacked from set 44's
implementation. It connects complete candidate snapshots to a fixed publisher,
authenticated commitments, a permanently retained seed target and explicit
checkpoint evidence. No Set 45 runtime or deployment is claimed yet.

### 6. League services
The Saturday operation: entry collection and lock (Saturday noon
PT), matchmaking in the D52 ratio band with random draw (D13, D53),
card production, results published provisional at once and final
after 60 blocks with the signed public checkpoint (D62), points and
rankings on the rolling 26-week window, title logic (D18, D20, D39,
D64), and the monthly team exhibition event (D59).
Depends on: fighter layer, chain core (checkpoint signing).

### 7. Marketplace
Fixed-price listings at launch (D57), listings showing age, career
phase, current effective attributes, and prime remaining (D48),
settlement in SLUG with the 5% commission on official sales (D56),
direct wallet-to-wallet transfers free and outside the marketplace.
Wash trading and self-dealing display rules are a build-time item
(spec section 17). Dollar checkout comes later through a licensed
partner (D60) and is not launch work.
Depends on: fighter layer (ownership), wallet.

### 8. Presentation
The Mine Event pages: home page, division pages, round-by-round
replays, fighter pages, rankings, lineage. A working prototype fed by
real sim output exists (The Mine Event space, November 15, 2026).
Production work is wiring it to league services output.
Depends on: league services.

### 9. Pre-launch legal and operations
Parked until closer to launch, by design: trademark attorney opinion
and filings for SLUG and The Mine Event, domain purchases (open:
slugleague.com, slug.game, slug.money, slug.gold), the legal review
list and operations list in spec section 17. The one exception that
cannot wait for launch is the workstream 1 security review, which
gates genesis.

## Sequence

Mike requires a thorough design review session with Codex before friends/tester
or public product release (D74, November 16). Keep it as an explicit gate in
`LAUNCH_READINESS.md`; review a representative working build together and track
the agreed findings through validation. Routine engineering deployment and
automated/code review do not replace that session.
Mike wants this as one full review once all major flows are ready, at the end
before release.

The November 15 chain-core handover and seed rollout are a milestone within
Phase A, not completion of all three Phase A workstreams. Prospect
registration and pool validation/payouts remain ahead; the explorer and
broader independent seed distribution also remain operational work.

- **Phase A, testnet.** Workstreams 1 to 3 in basic form: the fork
  mines, prospects register from shares, the pool validates shares
  and pays out. Engineering only, disposable chain.
- **Phase B, production build-out.** Workstreams 4 to 7 against the
  testnet, then the full system hardened. Build the real thing once;
  there is no throwaway product (spec section 2).
- **Phase C, genesis and tester phase.** Genesis on the real chain.
  The small-group test runs on the real chain and its history is
  permanent: tester-phase inaugural champions are the real inaugural
  champions (D2, D20).
- **Phase D, public.** Public mining and public league from genesis,
  no invite gate on Season 0 (D1). Workstream 9 completes before any
  public marketing push.

## Assets and accounts (workstation rule)

Domains are bought in Mike's registrar, the canonical code lives in
Mike's GitHub and on his servers, app signing uses Mike's developer
accounts, and wallet or seed material never touches the assistant's
machine. Prototype and simulation work stays on the assistant's VM
until real assets exist.


### Idle Phase-A seed maintenance (set 38)

On November 16 Pacific (November 17 UTC), the unchanged height-61 tip crossed
24 hours old immediately after set37 deployment. The inherited OnGetHeaders
freshness gate then silently ignored new peers' requests. Insert the bounded
history-serving repair before candidate service/worker integration (now set39).
Preserve normal currentness and mining gates; verify a fresh node can download
existing history without mining another block. Re-run full seed observation
after this maintenance; set37's blocked observer is not a successful gate.
