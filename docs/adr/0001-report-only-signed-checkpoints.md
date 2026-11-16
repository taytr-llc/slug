# ADR-0001: Report-only signed league checkpoints

**Status:** Accepted
**Date:** 2026-11-15
**Deciders:** Mike (policy and key custody); implementation recorded by Codex

## Context

D62 declares checkpointed results to be the official record. If a chain
reorganizes across a checkpoint, the community decides its canonical branch.
D63 makes upgrades opt-in. btcd's existing checkpoints additionally reject
some branches and skip parts of validation, which would give the operator's
signing key control over chain acceptance if reused for league statements.

## Decision

Use Ed25519 signatures to authenticate public league declarations, report
their local chain status through `getcheckpointinfo`, and leave consensus
and chain selection unchanged. Mike explicitly chose this policy over
opt-in enforcement. The signed declaration survives a conflicting reorg;
the report exposes both its hash and the current chain's conflicting hash.

Mike creates the private key himself on his Mac with the offline
`slugcheckpoint` tool. Only his public key is compiled into chaincfg. Until
enrollment, no authority or official signed testnet checkpoint is claimed.
There is no runtime public-key override or generated default authority.

## Options considered

| Option | Effect | Decision |
| --- | --- | --- |
| Report-only declarations | Social finality and visible conflicts; ordinary chain selection continues | Accepted |
| Opt-in enforced checkpoints | Signer can reject branches; existing machinery also enables validation shortcuts | Rejected for this set; requires a separate decision |

## Consequences

- A lost or stolen key does not take over the chain. League services must
  respond to authority changes and conflicting declarations socially.
- No signature-based fast-sync or Argon2id verification bypass is added.
  Earlier build-plan language suggesting that benefit is superseded.
- Local imports are explicit, bounded JSON files verified at startup.
  Distribution, reload automation, and multi-key rotation are separate work.
- The format binds version, network magic, genesis, height, and block hash
  in a domain-separated canonical binary message. JSON is only transport.
- `finalized` means this local chain matches with at least 60 descendants;
  status can change on reorg without revoking the operator's declaration.

## Action items

1. Complete in set 7: verification, offline signing, and read-only RPC.
2. Complete in set 10: Mike supplied only his public key; it is compiled
   for the Phase A SLUG network. See the current operator guide for its hex.
3. Pending: Mike signs a checkpoint after the selected validated block has
   60 descendants; verify and load the public signed document.
