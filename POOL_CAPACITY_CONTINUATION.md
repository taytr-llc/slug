# Pool capacity continuation

Status: first integrated slice locally qualified, starting from Set74 `e0e71ac0`.
This is an engineering storage plan under D58/D79–D81/D84, not approval
to reset a source, change custody, deploy the public pool or launch the app.

## Context and selected approach

V1 has lifetime limits in issued work, receipts, accepted shares, round rows,
credits and daily batches. Its complete source config is bound into transport,
wallet custody, scheduler and operator setup identities. Public admission and
ownership publication have additional signed limits. Removing a database row,
starting a new accountant, or repinning an old wallet cannot safely extend
those limits: old D80 jobs, round windows and reserved payments still matter.

Keep original identities and one logical settlement owner. First remove the
requirement to materialize every historical job, proof and completed round in
one read. Then add explicitly selected durable continuation with retained
predecessor evidence and bounded active work. An unchanged V1 constructor must
continue to enforce its original contract; a new marker cannot silently make
old clients or wallets adopt different source semantics.

## First integrated slice

`poolround` derives one open share window across accepted-source pages, checking
the original global sequences, duplicate guards and exact rational allocations.
It verifies every stored row/index, metadata byte budget and complete consumed
prefix. Capacity never hides corruption or an incomplete source read. `Sync`
stages new rounds in one existing synchronous batch and writes only after the
complete audit succeeds. An unfinished window never closes at a page boundary.

`WalkSettlement` holds the original round-store gate through verification and
all callbacks. It audits the entire consumed history before the first callback,
then returns ascending owned pages of at most eight records. A nil visitor
still verifies source identity. It observes no node, creates no confirmation,
performs no mutation and exports no database/snapshot handle. Callbacks must
perform bounded local work and must not reenter or close the store. Close joins
the admitted operation. Any failed walk returns no source config.

Settlement stages new credits locally, checks every old credit and the final
source binding, then applies the complete addition only after success. Existing
pending-payment recovery comes first and retains its original signed bytes,
reservations, daily slot, funding claims and original owner/signing capability.
Legacy full-snapshot APIs still return their complete historical result.

The independent frozen V1 round corpus remains the arithmetic oracle. This
slice changes neither stored round encodings nor source/config IDs. Duplicate
sets still grow within V1 bounds; source verification still reads the complete
consumed prefix. It is not a claim of constant-memory or resumable lifetime
auditing, and **no V1 capacity ceiling has been raised**.

## Oversized daily payouts

D84 chooses whole-batch deferral. Inspect the full live funding set in bounded
requests; observation pages are not selection windows. If all ready funding
and eligible outputs cannot fit one transaction, preserve every balance, send
nothing, leave the day unused and report `transaction-capacity`. Never choose
an oldest-first subset or advance pool reserves. A retained signed batch still
recovers exactly before new capacity checks.

The current native SegWit bounds are 256 inputs and 256 payments. With change,
the conservative full signed size is 46,383 bytes, below the 100,000-byte cap.
The current 256-credit ledger also prevents a normal whole-ready input overflow.
Larger arithmetic/inspection fixtures are conditional coverage, not evidence
that the current operator has crossed its lifetime limits. Waiting need not
shrink an oversized batch; the visible hold may require a capacity upgrade.

## Remaining continuation work and acceptance

- Explicit durable adoption/continuation descriptors and atomic crash recovery;
  preserve original base IDs, predecessor evidence and one logical owner.
- Append/indexed credit and payout history, exact fractional carry and original
  funding-claim provenance without a growing monolithic state rewrite.
- Resumable source/round verification, preserved original-job/nonce recovery
  and global deduplication across storage boundaries.
- Versioned receipt/history clients and signed authority/publication capacity;
  old V1 transports must not accept counters beyond their original pin.
- Full-intake, segment-boundary and actual restart acceptance with a pending
  payout: one credit per exact late winner, unchanged reserved bytes, no share
  reuse and no partial or duplicate payment.
- Concrete host provisioning/retention/disk-full recovery and scheduled actual
  payout qualification before calling the public pool deployed.

Fixture/streaming tests alone do not close these items. Sign-in/onboarding
follows the public pool work; the full joint D74 product design review remains
last before release.
