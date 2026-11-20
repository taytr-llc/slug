# Owned gym desktop integration

Set73 engineering integration plan, November19,2026. This document wires the
adopted ownership, carrier and recovery contracts into the existing Go desktop.
It does not close the actual-transfer milestone or the final joint D74 review.

## Explicit runtime

The new fixture entry point is `--candidate-owned-gym`, with its own
`LocalOwnedGymPilot` data root. It uses the current layout and fist artwork.
The root owns the existing fixture account authority/vault, both normal nodes,
the guarded public admission source, the ownership replay store and the owned
league stores. Each original account view borrows its schema14 wallet,
allocation2 inventory, withdrawal journal, carrier journal and owned stable.

Setup chooses and synchronizes all independent IDs before constructing their
stores: public source AuthorityID/PublicationID, ownership StoreID, two wallet
owner/inventory/carrier/withdrawal journal IDs, and league namespace identities.
Existing public fixture identities remain the only wallet identities supported.
The exact public source pin includes the compiled fixture keys/rules/limits.
An existing component is opened against retained setup; its marker cannot supply
replacement configuration. Missing established components remain a recovery
condition. Setup retries retain the original IDs and exact partial prefix.

The owned league intent and ready records are under the selected app root at
`owned-league/SLUG-OWNED-LEAGUE-INTENT.json` and
`owned-league/SLUG-OWNED-LEAGUE-READY.json`. They retain the complete owned/core/event
pins, store IDs and original creation time, with the pool-runtime/source records
as corroboration. The concrete seed/result owner receives independently expected
pins and actual resource owners before reading these records.

## Reviewed fighter send

Selection comes from one immutable owned-fighter capture. The backend resolves
the selected fighter and predecessor from that capture; user input supplies the
recipient gym key. The controller captures the original account view before
work, creates a durable request, and releases the journal permit before calling
the concrete wallet's funding/preparation APIs.

The public ownership store authorizes the exact predecessor and funding tuple.
The wallet commits its return allocation, reviewed preparation and shared input
reservations together. The app retains that exact preparation in its carrier
journal before displaying the native review. Review shows fighter identity,
sender/recipient gym keys, fixed fees, return destination and total debit.
Fresh process-local review/action IDs bind the retained review digest and
original view; copying displayed JSON cannot authorize signing.

Native confirmation uses the concrete vault gym signer and wallet funding signer.
Both transaction legs and permanent input reservations are retained before the
first broadcast. A published dispatch mints a single-use delivery ticket for one
exact leg. The spend leg follows an observed commit. Interrupted delivery stays
unknown until fresh observation; explicit retry sends the same retained bytes
with a fresh original-view approval. Opening the app never sends automatically.

After delivery completes, Observe returns the unchanged saved delivery receipt
under the original View. It does not rewrite the receipt or refresh inclusion.
Fresh ownership status comes from public replay.

Transport observations are distinct from ownership. The app labels ownership
pending until the public replay proves its checkpoint and60 descendants, displays
void competing sends and reports later conflicts under D83. Current roster and
historical noon ownership use their respective concrete owned-store readers.

## Recovery and lifetime

Current hosted export uses the concrete allocation2 recovery head and typed vault
export. The original RecoveryExport capability gates publication. Reset holds
and session/view replacement invalidate pending exports and transfer approvals;
viewing existing public state remains available under the existing policy.

Independent gym recovery starts from the unchanged recovery file and independently
selected public source/node configuration. It derives the same gym key, performs
bounded schema12 scanning, and explicitly activates schema15 only after readiness.
Public ownership replay discovers current fighters without a hosted login or the
original private app journals. Missing public evidence is reported as unavailable.
An original private authority database is not a substitute for public history.

Commit-only recovery retains the exact canonical commit and historical predecessor,
matches funding/return paths only inside the original exported native84 bounds,
and reconstructs the committed envelope. The separate `slugrestore` command
requires an explicit review for reveal or refund (see RESTORED_GYM_CLI.md).
Only the reviewed single leg is signed; the original commit is not sent
again. A refund returns escrow and does not transfer the fighter.

Stop/logout/reset/quit cancel and join actual wallet, replay, network and file
operations before unloading their owners. Uncertain writes preserve their data
and require exact reopen/reconciliation. Each action has bounded work/storage
and keeps the original owner; replacement sessions cannot rescue stale handles.

## Acceptance

The milestone requires a reviewed actual A-to-B transfer, competing-send order,
checkpoint/depth59/depth60 states, noon before/after ownership and condition,
reset/hold races, lost-reply/restart retry, and independent same-gym fighter
recovery followed by an actual transfer. Commit-only reveal and refund must work
from the original file with current spendability checks. Normal builds, packaged
flows, old-route regressions and the changed-node rollout remain required.

After this milestone, work proceeds to the public mining pool and scheduled
payouts, then sign-in/onboarding. The full joint D74 review stays before release.
