# Shared issuer, separate gym inventories

Set 59 adds an explicit candidate shared-source stable. A pool can issue jobs
for both published fixture gyms through one concrete `registry.ManifestStore`.
Each gym consumes that verified stream while retaining only its own fighters.
This is source scoping, not transferable production ownership.

## Explicit namespace

Use `Pin.Version = SharedPinVersion2` and `CreateShared` / `OpenShared`.
Legacy constructors require Pin1; shared constructors require Pin2. Nothing
migrates automatically. Pin fields and canonical field order remain unchanged.
The shared stable ID is SHA256 of:

```
"SLUG candidate shared stable\x00v1\x00"
|| uint32LE(len(canonicalPinJSON)) || canonicalPinJSON
```

The marker is `SLUG-SHARED-GYM-STABLE.json`, with canonical fields
`application, version, store_id, pin`, application
`slug-candidate-shared-stable`, version 2 and a final newline. Old stable IDs,
markers, assignment schemas, source records and signed jobs keep their bytes.
The independent Python reference under `gymstable/testdata/shared-reference`
derives the new public namespaces and paging models from frozen public inputs.

## Source cursor and local inventory

`Status.Checkpoint` is the last consumed **global source sequence**.
`ObservedSourceCount` is the durable global high water. Neither counts a gym's
fighters. `SharedStatus` wraps the unchanged Status with version 1 and
`AssignmentCount`, the gym's local count. It checks current source availability
and high water without hashing retained history. Global source history remains
bounded at 1,000 records; this change does not raise existing candidate limits.

`Sync` verifies up to eight consecutive source records through the original
concrete registry before filtering. Exact gym ID and public key matching decide
local assignment. Foreign rows advance the cursor even when `Items` is empty.
Assignments retain original source sequence, digest, birth and receipt. Foreign
records become bounded private skip witnesses containing source sequence,
prospect ID, gym ID, public key and canonical source-record digest. They do not
become imported receipts or foreign inventory items.

One synchronized batch retains assignments, reciprocal indexes, skip witnesses,
local count, source cursor and clock. Opening audits complete assignment-or-skip
coverage of every consumed source position, unique prospect IDs across both
classes and exact local counts. An uncertain write poisons the live store;
reopening reconciles the durable batch. Source loss or rollback fails closed.
Both assignment and skipped rows remain local audit evidence, not replacements
for the original proof authority. Fighter reads still verify the original row.

`Inventory(after, limit)` returns local assignments in ascending original source
order. If another local item exists, `NextAfter` is the last returned sequence.
Otherwise it is the consumed checkpoint, including any trailing foreign rows.
An empty local page can therefore advance its cursor. `SharedRecent` selects
the last local assignments, not `checkpoint - limit`; its `After` is the
preceding local sequence or zero. Page limits and serial proof bounds remain.

## Roster, entry and career

Existing roster and noon-cutoff journals refer to retained prospect IDs and
original assignments, so shared stables reuse them. Foreign IDs cannot enter
the gym's roster or event selection. Shared reads recheck the source after
proof work. New source growth may make the stable lag until its next bounded
sync; roster mutations require a current consumed prefix.

`OpenSharedCareer` and `ResumeSharedCareerBinding` use the existing explicit
career activation. `CareerSharedRecent` and `PrepareCareerSharedRecent` retain
the exact local page and original session in the existing opaque read fence.
`ReadCareerInventory` redeems that handle. Even foreign source growth can
invalidate an older captured read; no stale handle is silently retargeted.
Test-clock variants exist only under `slugtestclock`; cheap PoW is refused.

## Desktop integration boundary

Typed Stable/Career validators accept sparse sequences only for valid Pin2.
Follow Recent uses the local-tail APIs and preserves the returned `After` in
its career fence. The private portrait/receipt cache retains only owned items.

The older ManifestSnapshot/Projection family assumes contiguous sequences and
has no stable mode field. Shared mode suppresses that legacy count/projection
publication instead of interpreting global source progress as a fighter count.
Typed Stable/Career pages remain authoritative. The future pool snapshot must
expose the explicit local assignment count. This set adds no runtime route,
navigation change or automatic switch of the existing solo app.

Next is the shared parent pool runtime: ownership of issuer, services, round
and settlement journals, child miners and wallets, and ordered shutdown.
Its visible balances and daily scheduler follow the already settled payout
rules. Production custody, genesis and the final joint D74 review stay separate.
