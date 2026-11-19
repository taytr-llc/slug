# Set 59 — separate gyms on one shared issuer

Branch: `slug-shared-gym-source`, stacked from Set 58 `f6f17f02`.

## What changed

Added an explicit Pin2 stable namespace for a shared manifest issuer. Each gym
fully verifies the global source stream, keeps its own assignments and records
bounded foreign skip witnesses. Assignment sequences, receipts, birth and source
digests remain original. One synced batch retains assignment/index/skip coverage,
the consumed source cursor and a separate local count. Reopening audits the
complete consumed prefix rather than assuming every source row belongs locally.

Sparse inventory pages retain original source cursors. Shared Recent selects the
local tail, including older local fighters separated by many foreign records.
The reused roster, entry and noon-cutoff paths retain ownership/session checks;
shared reads recheck source health after proof work. Career recent reads preserve
the original opaque selection and account session. Foreign IDs cannot enter
another gym's roster or event selection.

Desktop Stable/Career validators recognize sparse pages only under valid Pin2.
Owned receipts remain available to the private portrait/export cache. Shared
mode suppresses the older contiguous ManifestSnapshot/Projection publication;
the future PoolSnapshot will use the explicit local assignment count.
See `SHARED_GYM_STABLE.md` for the storage and integration contract.

## Preserved contracts

Pin1 IDs, marker bytes, schemas and contiguous pagination remain unchanged.
Shared mode requires separate explicit constructors and marker; it never
migrates or reinterprets an existing store. Source records are neither copied
into child registries nor renumbered. The global 1,000-record candidate limit
and eight-proof call bound remain. No chain, account reset, D65 rating, D76 noon
snapshot, D78 career award, pool fee or payout policy changed.

The existing app route, qualified Set51 bundle and navigation remain unchanged.
This is functional shared-source preparation, not the final joint D74 review.
No real keys, wallets or production ownership are introduced.

## Verification

Evidence is under `../artifacts/set59`, using the pinned Go 1.27.4 no-CSE profile.

- The independent Python reference derives Pin2 IDs, marker bytes and sparse
  cursor expectations from frozen Set41 public inputs. Its corpus SHA256 is
  `29b0aa42752e3db62afed4c5800373a9ada39fb1c9fd4319110d9725eafab225`.
  Namespace/marker checks passed. Real newly issued and fixture-signed normal
  proofs drive the independent paging model; old source-bound receipts are
  never relabeled into a combined issuer.
- Focused shared tests passed in 12.737 seconds: foreign-only pages and empty
  local inventory; proof verification before filtering; complete prefix audit;
  cancellation and Close draining; uncertain-write and reset recovery; source
  loss/rollback; local roster/cutoff and career fences.
- The downstream event composition passed in 1.06 seconds under `slugtestclock`.
  Two genuine admissions from one concrete issuer produced one fighter per gym
  at original sequences 1 and 2. Both entered the same noon event. A later roster
  removal and reopening issuer, stables, entries and event preserved the exact
  original snapshot. Its current-parent guard and clock are modeled; this is
  normal proof/storage composition, not live-chain finality evidence.
- Final checks passed on their first attempt: complete gymstable normal suite
  39.351 seconds; shared race suite 84.900 seconds; desktop Stable/Career
  regression 6.228 seconds; shared desktop race 17.097 seconds; cheap-profile
  refusal 8.514 seconds; clock API/reference 1.790 seconds; vet 0.820 seconds.
  Full-repository gofmt and `git diff --check` are clean. Daemon dependency
  inspection confirms no changed package is linked by the seed.
- Preserve the initial failures: core compile-attempt2 and desktop-attempt1
  selected the root workspace for a separate desktop module; no applicable
  desktop tests ran. Corrected compile/focused attempts passed. Shared test
  attempt1 expected two incorrect existing error sentinels for a foreign entry
  and a closed registry; the corrected expectations passed without production
  changes. Review also caught the `shared` metadata key sharing the skip-key
  iterator prefix before focused validation; the iterator now distinguishes it.
- HANDOVER smoke passed with newly built normal-profile daemon/control binaries.
  Two blocks were mined only while isolated with zero peers; the coinbase paid
  10 SLUG and the conflicting signed checkpoint stayed report-only. Read-only
  public seed sync completed in 13.37 seconds at height61 with the original tip
  and 60 checkpoint descendants. Activation62, chain verification, stale-template
  refusal and restart all passed. The owned node stopped cleanly without force;
  no public-chain mining, deployment or live data changes occurred.

## Next functional boundary

The shared parent runtime still needs to own nodes, issuer, pool journals,
services and daily settlement while children own their wallet/miner/view.
Inspection for that integration found that current pool, manifest admission and
balance operations hold the global account lease across node/proof work. The
next set will shorten those leases while keeping original-token authorization
atomic with final writes, so a slow miner cannot delay verified reset or another
gym's account operation. No new policy decision is needed.

Master and the live seed remain Set50 `592e143c`; these off-chain changes require
no rollout. A new shared-pool app route and package follow the runtime work.
