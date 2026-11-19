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

D79 fixes accounting to difficulty-weighted per-block rounds, including the
winning share. Earnings lock at sixty descendants; daily settlement waits for
the funding coinbase's 100-block maturity, with no reserve advances. The 1%
fee applies once and balances below 1 SLUG carry forward. See
`POOL_ACCOUNTING_POLICY.md` for D58's approved exact carry-forward, single daily
batch and network-fee funding from the operator's 1% only. Insufficient fees
defer visibly; conflicted-funding incident resolution remains separate.
Set 52 implements and tests nonce-only manifest share verification with
separate pool/prospect/block targets and stable duplicate identities. Durable
acceptance follows in Set 53; no live pool or framework adaptation is implied.
Set 53's bounded `poolshare` journal now persists issued work, authenticated
share acceptance and block delivery, recovering exact duplicates and applying
D80 only to exact canonical payout-bound winners. Public transport and miner
integration and payouts remain ahead. Set 54 adds fixed D81 rounds, exact gross
allocations and fresh 60-descendant confirmation locks. Orphaning and return
change status without reusing shares; a later funding conflict retains locked
earnings. Set 55 adds exact net balances, original mature funding, one durable
daily batch, visible deferrals and identical-byte payout recovery. Its first
adapter signs only with published fixtures on an isolated chain. Public
transport, miner integration, the daily service scheduler, authoritative payout
address enrollment and production signer custody remain ahead.
Set 56 connects these components through authenticated pool HTTP and a pinned
Go client. The desktop credential adapter refreshes tokens between calls while
retaining its original account/view and pool trust. A real two-account isolated
chain demonstrates HTTP work, lost-response recovery, shared payout and
reset-scoped balance/mining access. The existing solo worker is preserved:
Set 57 adds the dedicated automatic pool worker and separate durable outbox.
Found work survives cancellation and lost acknowledgments; pool credit and
fighter admission recover independently against the original issued job.
An admission-only service shares the pool issuer without creating jobs outside
its work journal. Next connect the shared desktop runtime and separate
native-SegWit recipient binding, then the balance view and daily scheduler.
Set 58 supplies the separate native84 wallet-recipient binding, preserving
legacy payout markers and recovering uncertain allocation without another
address. Set 59 supplies explicit Pin2 per-gym stable scoping for a shared
issuer, preserving original source sequence/evidence and separating global
cursor progress from local assignment count. Sparse inventory, local Recent,
roster, noon entry and career reads reuse their original authority fences.
Legacy single-gym stores and contiguous projections keep their semantics.
Set 60 removes the account lease from slow pool proofs, node/source calls and
balance verification while retaining original-token authorization through each
actual local commit. Additive staged registry commits fail fast on lock contention;
the new shared runtime must select the explicit fenced admission-only server.
Legacy registry/session APIs and wire/journal formats remain unchanged.

Set 61 connects these components through the explicit shared fixture Pool Pilot:
one parent owns the nodes, issuer, pool and accounting journals; two actual
wallets retain their Set58 recipients; each original account view owns its
worker, outbox and Pin2 stable. Authenticated balances expose exact carry and
bounded daily settlement. Only Start mines. Account switch/reset drains the
child without replacing the source; Quit joins the complete runtime. The solo
route retains its namespace. See `CHANGESET_61_NOTES.md` for the actual payout,
isolation, reset/restart and package qualification.

Set62 connects shared-wallet export and fresh restoration to these actual pool
recipients, preserving the original view and D72 hold through publication. The
funded independent restore/spend and exact-package checks pass, including fresh
wallets at genesis. See `SHARED_POOL_RECOVERY.md` and `CHANGESET_62_NOTES.md`.
Set63 connects shared-pool fighters to Saturday entries and durable noon
snapshots in an explicit parent-owned coordinator. Both gyms retain original
Pin2 participant pins and source sequences across reset and restart. See
`SHARED_POOL_LEAGUE.md` and `CHANGESET_63_NOTES.md`.
Set64 connects shared publication, cards, fights, results and career awards,
activating career authority before the new event's lock and first finality.
Actual normal-PoW acceptance proves both finalities and exactly-once careers
through retry/restart. Publication funding stays separate from share-accounted
pool rewards. See `SHARED_EVENT_CYCLE.md` and `CHANGESET_64_NOTES.md`.
Set65 qualifies an explicit name-v2 candidate with 512-by-512 names, complete
independent manifest/portrait references and a fixed 100,000-seed collision
study. Both package routes and the actual v2 event/finality/career cycle pass;
existing fighters, source pins and defaults remain v1. See `NAME_DIVERSITY.md`
and `CHANGESET_65_NOTES.md`.
Set66 completes bounded original-receipt/history export from the existing panel
and a packaged offline consistency checker. Six actual v1/v2 exports match the
independent reference, including an older fighter outside Recent. Original-action
recovery, complete desktop, package and smoke checks pass; see
`PUBLIC_FIGHTER_EVIDENCE.md` and `CHANGESET_66_NOTES.md`. Supplied consistency
does not establish issuer timing, actual awards/finality, current ownership or
complete authoritative history. Transfer timing and competing-transfer ordering
still require an explicit policy decision.
Set67 implements authenticated own-payout history with fixed local pages and
a separate dated observation of the selected transaction output in the owned
wallet. Preserve daily accounting, original-view authorization and the distinction
between retained pool state and current wallet observation. No new send,
settlement or fee authority is added. See `POOL_PAYOUT_HISTORY.md`,
`POOL_PAYOUT_DESKTOP.md` and `CHANGESET_67_NOTES.md` for qualification status.
Set68 implements reviewed personal withdrawals with original-view D72 gates,
paired durable wallet/journal reservations, exact signed-byte recovery and
explicit native approval of recipient, amount, fee and total. Its actual
two-wallet rehearsal includes a lost acknowledgment, same-byte retry, one debit,
live change detection and restart. See `REVIEWED_WITHDRAWALS_ADOPTION.md` and
`CHANGESET_68_NOTES.md` for contracts and qualification. The candidate is limited
to public schema-9 fixture wallets.
Set69 adds bounded schema-12 restoration of existing Phase-A version-1 files,
with unchanged derivation/root/gym identity, explicit library resume, joined
scratch cleanup and exact synchronized tip comparisons. Live exports already
track current issued/change cursors. The separate hosted-vault path still needs
authoritative evolving metadata and integration; successful scratch verification
does not restore the installed payment journal or authorization. See
`PHASE_A_BACKUP_RECOVERY.md`, `PHASE_A_RECOVERY_CLEANUP.md` and
`CHANGESET_69_NOTES.md` for qualification and limits. Transfers and general
matchmaking still require their own decisions.
Never repin solo event histories. Keep the existing layout/logos until the final
joint D74 review. Public hosting, production signer custody/capacity, real account
configuration and permanent genesis remain separate gates. The implemented
contract is `SHARED_POOL_DESKTOP.md`; the earlier workspace runtime proposal
remains historical context. See `CHANGESET_52_NOTES.md` through
`CHANGESET_69_NOTES.md`, `POOL_WORKER.md`, `POOL_RECIPIENTS.md`,
`SHARED_GYM_STABLE.md` and `POOL_SESSION_FENCES.md`.

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

Set 45 is implemented on `slug-event-seed`, stacked from set 44's
implementation. It connects complete candidate snapshots to a fixed publisher,
authenticated commitments, a permanently retained seed target and explicit
checkpoint evidence. Independent reference comparisons and actual owned-node publication, finality,
reorg and restart composition pass. Final source checks, the exact original-icon
package, deployment at master `dcc44829` and the full 15-minute live observation
and restart passed. The requested app/menu-bar icons are included in the native
build; GUI, portrait and joint D74 acceptance remain open.
See `EVENT_SEEDS.md` and `CHANGESET_45_NOTES.md`.

Set 46 is complete on `slug-event-card`: retain a candidate booked card from
the immutable noon inputs and fixed seed. Scoped exception C1 allows the two
zero-record fixture gyms to match inside the existing same-class half-to-double
band without five opponents. Report all entered, paired and unmatched fighters
and reasons; general league widening, record filters and next-week priority
remain open. See `CANDIDATE_CARD_EXCEPTION.md` and `CHANGESET_46_NOTES.md`.
The implementation and initial normal-PoW scoped-clock composition now produce
one retained bout from three entrants, with one reported unmatched fighter.
Independent arithmetic, frontend checks and the corrected final-source Mac
package pass. Implementation `5e6b36d3` is deployed at master `5c11cbaf`;
the full 900-second live observation, 31 samples and clean restart passed.
Railway still runs only the Phase A node. Remote CI returned no runs/checks;
no remote CI pass is claimed. Failed earlier package/race attempts remain retained.

Set 47 on `slug-event-bouts`, stacked from `5e6b36d3`, retains one immutable
candidate transcript per booked BoutID with explicit Run and saved replay.
The independently frozen reference, implementation checks and actual native
composition pass. The closed-event test uses normal PoW and a scoped clock;
authenticated core seed preparation runs without an admission worker. The
final normal-clock package and its seven independent reports passed. Release
`ba218f55` is deployed; the full 900-second live observation, 31 samples and
clean restart passed with unchanged Railway configuration. Remote CI returned
no runs/checks and is not claimed passed. See
`EVENT_BOUTS.md` and `CHANGESET_47_NOTES.md`. Professional/exhibition records,
awards, general matchmaking policy and the full joint D74 review remain separate.

### 6. League services

Candidate status: Set 48 shipped result publication/finality and public proofs;
Set 49 implementation `5b590f40`, released at `0a4374de`, adds prospective
finality-timed standard records, capped XP and base points, retained through later
conflict. Local and scoped-clock normal-PoW awarded acceptance passed. The normal
Mac package and nine independent reports separately passed zero-award current
Career pages and recovery, followed by the full 900-second/31-sample deployment
observation and restart. Remote CI absence is assessed, not a pass. C1 still
refuses awarded fighters; historical noon inputs remain fixed. The shipped
legacy practice preview still uses XP0. Set 50 implementation `99d36ef2` adds a
separate career-aware practice path under `CAREER_PREVIEW.md`. Local checks,
all 249 frontend tests, three scoped awarded-fighter jobs/two distinct
compositions, and five normal-package XP0 jobs/ten typed portraits passed.
The final ten independent package checks include nine unchanged-input-bound
reused reports and the corrected tenth check; failed attempts remain preserved.
Release `592e143c` is deployed. Final observation and provider closeout
passed: 900.03 seconds, 31 successful samples at height 61 with 60 checkpoint
descendants, and a clean restart.
Railway remained healthy on the exact release, with configuration, volume and
TCP endpoint unchanged. Remote CI absence is assessed, not
a pass. Practice grants no permanent awards. Complete rankings/title authority,
production gates and D74 remain open. See
`CHANGESET_49_NOTES.md`, `CHANGESET_50_NOTES.md` and `RESULT_AWARD_POLICY.md`.
Set 51 implementation `751b947b` completes authenticated finalized fighter
history: one captured prefix, bounded newest-first pages and per-result
base-point/earned/applied-XP explanations. Local, scoped actual, cold-capacity,
normal Mac package and eleven independent package checks pass. The candidate
desktop build is 0.4.11/build 51; the unchanged seed stays on Set 50. Preserve
the failed first package attempt and its unresolved cause separately from the
successful diagnostic and strict successor package. Full ranking/title policy
stays separate. See `CHANGESET_51_NOTES.md`.

The Saturday operation: entry collection and lock (Saturday noon
PT), matchmaking in the D52 ratio band with random draw (D13, D28),
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
He clarified on November 18 that app organization and navigation belong in
that final session too. The external navigation draft is deferred and unapplied;
it is not the next engineering set. Continue the functional workstreams below.

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
