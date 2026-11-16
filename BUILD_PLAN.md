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

### 5. Gym clients
The gym is the product surface: native Mac and Windows apps (menu
bar and tray, gentle default mining setting) plus the browser
version. Miner integration with the pool and solo switch, roster and
Saturday entry management, stable and prospect views, training and
exhibitions, team gym membership and standings, marketplace access.
Signing and notarization run through Mike's developer accounts.
A working interface prototype exists (SLUG Gym space, November 15,
2026); visual design polish is deferred by Mike's call.
Depends on: fighter layer, pool, wallet.

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
