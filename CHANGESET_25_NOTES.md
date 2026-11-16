# Change set 25 — Phase-A script activation at height 62

Status: local implementation, independent review and acceptance passed on
`slug-phase-a-activation`; exact-commit CI and release observation follow publication,
stacked on set-24 implementation `dec10e165ac1be541f935502c4e8da2bfaf6cc89`.

## Approved behavior

Mike approved D71 on November 16, 2026: activate at block 62 on Phase A,
implement, validate, then deploy. This addresses the inherited Bitcoin schedule
that left witness transactions unusable despite wallet address support.

BIP34/65/66 use height 62. CSV, SegWit and Taproot use the existing
`AlwaysActiveHeight` override at 62, before voting-window/cache handling.
`MinActivationHeight` alone is not activation. BIP34's future hash remains nil,
so BIP30 duplicate-transaction checks continue. No future block hash is invented.
The existing height-61 tip is preserved; genesis, blocks 0–61, Argon2id,
network identity, subsidy, maturity and report-only checkpoints are unchanged.

The RPC adds `always_active_height` when an override is configured, independently
of the voting minimum. BIP9 `status` describes the next candidate block: active
at tip 61 for candidate 62. The inherited BIP34/65/66 `reject.status` describes
the current tip and becomes true at tip 62. Historical vote timestamps remain
visible but cannot override the approved fixed height.
Startup logs record the six configured activation heights and next candidate
height so deployment can be checked without exposing the seed's private RPC.

Integration uncovered an inherited JSON ambiguity: two embedded response types
both named `softforks`, causing the buried-rule array to disappear. Explicit
marshaling now emits the configured legacy or unified representation and rejects
an ambiguous value containing both. The paired RPC client also guessed unified
format for every btcd-derived backend, then rejected SLUG's restored array.
Chain-info decoding now selects the actual documented array/map representation,
preserves legacy responses that omit the array but include BIP9 data, and rejects
scalar forms. It no longer probes the software version merely to decode an
already received response. Other backend capability detection is unchanged.

The prospect client explicitly declares SegWit support in its template request.
This does not claim that a previously missing request field caused the failure:
the SLUG server's existing template implementation does not negotiate that field.
The pinned template response contract and strict unknown-field checks remain.

## Verification and rollout gates

Boundary, reorg and script-enforcement tests pass, including normal and tagged
profiles. The full tagged blockchain and integration suites pass. The normal
script interpreter, mining/mempool tests, production PoW vectors and ten-module
builds pass. Fresh upgraded history replay and reopening a copied height-61
database preserve all pinned historical hashes and checkpoint status.

The paired normal-PoW gate passes for legacy, nested witness, native witness and
Taproot funding: retained export after reopening, fresh recovery from that file
alone, exact history and balances, the same gym identity/automatic admission,
and a second confirmed spend consuming recovered change. The native case also
preserves a real wallet witness transaction through template resolution,
candidate construction, proposal validation, normal PoW mining, submission and
exact canonical-block observation. Total test time was 311.05 s (package
311.449 s); individual cases 77.42/72.60/89.46/71.56 s. All finish at height 102.

Two initial attempts failed on the inherited missing buried-status field and
wrong client decoder described above; neither is represented as a pass. During
the successful local run, the node executable was replaced by a build adding
only the startup activation log. Early cases used the prior build, Taproot used
the logged build, and the native peer's exact build was not retained. This is
functional evidence with recorded binary provenance, not a single-build claim.
The exact published CI must build one pinned node for its full acceptance run.
See the companion `set25-wallet-activation-smoke.json` and binary-replacement
record beside the checkouts. The final startup-log build also reopened the
retained height-61 fixture and reported all six heights as 62, nextheight 62.

Root/RPC/template race checks, the response-shape regression, normal Linux
amd64 node build, vet and formatting checks pass. The paired wallet passes all
six module builds, complete normal tests and vet, plus focused mixed-scope and
RPC-policy races. Ordinary
`verifychain` checks block sanity, not full contextual/script replay; it is not
a substitute for a fresh observer replay or fresh post-activation test chain.

Immediately before promotion, confirm the seed still has height 61 and hash
`1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0`.
If it has advanced, stop and agree a transition. Upgrade mining and validation
services before further mining. This change does not authorize public mining,
change production genesis, or start a wallet service on Railway.

Binary rollback is straightforward only before post-activation history exists.
After block 62, stop mining and assess the exact chain as a coordinated consensus
incident. Do not delete the persistent volume or rely on cached validity under
an older binary as a rollback proof.
