# Change set 24 — retained wallet recovery and gym identity

Status: local acceptance passed on paired `slug-wallet-recovery` branches;
exact-commit CI and release observation are recorded separately after publication.

The chain branch is stacked on set 23 implementation `4dc49e10`; the separate
wallet branch is stacked on `28ab638d`. The chain source records Mike's D70
decision: 24 English BIP39 words, no extra phrase passphrase, and a versioned
SLUG wallet/gym recovery manifest. Wallet implementation and acceptance evidence
are recorded in the wallet repository's change-set notes.

The completed library slice retains recovery material encrypted at atomic wallet
creation, supports later export after reopening, restores an empty wallet,
and preserves the same wallet-derived gym identity for automatic admission
signing. New fixture profiles explicitly select standard BIP32; existing
wallet derivations and raw-seed behavior stay unchanged.

This is a Phase-A public-fixture acceptance slice. It does not provision real
accounts, hosted custody, production derivation paths or a wallet service on
Railway. No consensus, checkpoint policy, birth rule, or fighter transfer is
changed.

## Verification

All ten chain modules build; gofmt and diff checks are clean. The normal-PoW
chain smoke passed: fresh isolated mining pays 10 SLUG, an isolated conflicting
branch is reported without rejection, a separate observer synchronizes with
the seed at height 61, and the signed height-1 checkpoint reports finalized with
60 descendants. Registry authorization, birth, replay, expiry-related service
behavior, payable template proposals and restart checks remain intact. The
smoke did not mine on or broadcast blocks to the public seed.

The paired wallet's six runtime modules pass builds, complete normal tests and
vet. Recovery, address-manager, cancellation and gym-signing race checks pass.
Independent BIP32/BIP39/address/gym oracles include a leading-zero hardened
derivation case that distinguishes standard derivation from the legacy mode.
Mandatory recovery schema 9 prevents old binaries or missing retention fields
from silently selecting legacy derivation. Existing schema-8 wallets retain
their existing behavior.

The normal-PoW wallet recovery integration passed in 72.94 seconds: fund a
published fixture at receive index 25, mature its reward, spend through RPC,
close and reopen, export the retained phrase, then restore from that export
alone after renaming the original database. The restored wallet recovers its
balance and signed history beyond the ordinary address gap, preserves gym
identity and automatic admission authorization, and confirms a second spend
that consumes recovered change. No real wallet or operator key is used.

## Known limits and next gate

The successful funded roundtrip uses legacy addresses. All four supported
scopes have independent derivation and recovery-cursor evidence, but Phase A's
inherited Bitcoin activation settings currently reject witness spends. Enabling
those rules requires a separate consensus decision and change set; this set
does not change activation or claim witness/Taproot spending works.

The desktop remains the set-22 fixture pilot. Hosted login/custody, user-facing
secret reveal/export, exclusive migration, production derivation allocation and
fighter transfer are not delivered by these library APIs. A recovery export is
the phrase plus its current manifest; an old manifest cannot describe later
addresses beyond normal discovery gaps. The old wallet is not revoked by restore.
