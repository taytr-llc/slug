# Candidate career consumers — Set49 integration contract

This extends `CAREER_ACCOUNTING.md` with the fixed parent/component boundary.
It preserves all existing source rows, pins and public v1 codecs. It does not
authorize nonzero-record C1 matching or invent ranked/title inputs.

## Durable component binding

Stable and Card each retain one additional canonical LevelDB row at the ASCII
key `career_guard`. It contains `{version:1, activation:<CareerActivation>,
fence_count:<uint32>, fence_digest:<lowercase SHA256>}` in that order, at most
2048 bytes. The existing old audit rejects an unknown key,
so installing the synced row is an old-writer barrier without rewriting source
markers or historical rows. New legacy constructors still reject this row.

Both packages add the following explicit installation/read surface:

- `BindCareer(ctx, reader)` on an open legacy component synchronizes the guard
  and attaches the reader. Repeating the exact binding is idempotent. Installation
  is trusted parent work under its root lock, never a browser/account mutation.
- `OpenCareer(...original constructor inputs..., activation)` requires the
  exact existing guard and opens a bootstrap handle with no attached reader.
  It never creates a missing guard or falls back to legacy Open.
- `AttachCareer(ctx, reader)` on that bootstrap handle checks exact activation
  and scope, then enables its career-dependent operations. Metadata needed for
  source reconstruction remains readable before attachment; no operation may
  interpret an absent reader as zero career history.
- A prepared-only constructor may open either an unbound legacy component or
  the exact bound component, for resuming the parent's documented install
  intent. It does not install anything by itself; ready startup never uses it.

The reader's fixed Scope must authorize the exact Stable StoreID or Card StoreID.
Matching caller-supplied activation fields alone are insufficient. Closed,
poisoned, cancelled or foreign readers refuse. A possible guard-write outcome
is unknown and poisons the handle; close/reopen/resume preserves the exact row.
The component borrows the reader; it never closes or owns the Result store.

## Parent v9 ordering

The parent retains its exclusive root lock and drains owned work before upgrade.
Preflight a ready v1 Result under its real store lock **before** changing the v8
root: if any retained Finality exists, refuse without changing the root version
or installing a child guard. Preserve that legacy route and original evidence.

Persist a v9 prepared intent for the exact existing component identities, then
perform the Result v2 activation from `CAREER_ACCOUNTING.md`. Persist its exact
CareerActivation in the parent intent before installing any Stable/Card guard.
Bind both installed Stables and the shared Card; publish ready only after all
exact guard/source/reader checks and a synced parent-ready marker.

Prepared restart can encounter only a subset of installed guards. It uses the
prepared-only constructor and resumes each exact binding. A ready restart
requires every guard. Open lower sources as bootstrap handles, OpenCareer Result,
verify the parent activation, then attach its reader before publishing operations.
Missing ready data never recreates a store, substitutes a reader or becomes zero.
All inherited parent marker writers preserve the full v9 suffix; old routes
refuse v9. If two-gym source setup is incomplete, retain a visibly waiting career
phase and permit no career-dependent entry/capture/booking operation yet.

## Core C1 checks and immutable cutoff receipts

Stable entry selection checks a Current career snapshot for the selected IDs;
any nonzero Wins/Losses/Draws is an early `career_candidate_ineligible` refusal.
Stable cutoff selection repeats the check against a sealed Cutoff snapshot.
Card's first Build checks every entrant in the exact frozen source at its cutoff.
Existing saved cards replay without rewriting their original plan.

The parent/coordinator calls Result.FenceCareer **before** invoking capture or
new booking. Core consumers only read the sealed Cutoff handle; they never call
back through Result's action gate. Without the durable fence they refuse.
An award at/after noon is excluded by the established strict-before convention,
so a fighter who was eligible at noon keeps that event. The ineligible error
retains sorted, unique affected IDs (at most ten); do not label them retired or
unmatched for lack of opponents. A player may use other zero-record fighters;
general matchmaking requires its separate policy.

Persist the cutoff-prefix digest and activation with a bounded component
cutoff-fence row before the final snapshot/first-card commit. Its fields, in order,
are `{version:1, activation_id, cutoff_ms, prospect_ids, career_digest}`. IDs are
sorted, unique, and number 1–10. The ASCII key is `career_cutoff:` followed by
lowercase SHA256 of `"SLUG candidate career consumer cutoff\x00v1\x00"`,
LE64 cutoff, then canonical JSON of the ID array (no length prefix). The fixed
all-zero 32-byte ID is the sole digest request for an empty selection/page; no
fighter, assignment or overlay is fabricated for it.

A repeated cutoff and identical ID selection must match; different selections
have separate receipts. Only a sealed Cutoff snapshot with those exact requested
IDs can create a receipt. A later observation/conflict cannot change its digest.
At most 256 receipts, each at most 2048 bytes, are retained. The owning component
clock must cover every receipt time; no receipt predates activation.

The binding commits the complete receipt set. Its digest is SHA256 of
`"SLUG candidate career consumer receipts\x00v1\x00"`, LE32 receipt count,
then each lexically sorted ASCII key followed by its canonical JSON row, both
prefixed by their LE32 byte length. In both domains, each `\x00` denotes one
NUL byte (0x00), not four literal characters. Empty bindings commit count zero under this
same domain. Every new receipt and updated binding synchronize in the same batch;
memory is installed only after Sync. Open recomputes count/digest, detecting a
missing, added or substituted row. Stable may retain a receipt even if the foreign
snapshot write subsequently fails; exact retry preserves it. Card commits the
receipt/binding and first card together. Old v1 codecs remain truthful because every newly admitted
C1 member still has zero record and zero XP. Missing/unavailable career evidence
never permits a fallback to the old mutation path.

## Current fighter display

Add separately typed CareerPage and CareerRosterPage reads over verified Stable
pages. Each binds the same original view, source page as-of, activation and sealed
career digest. Each overlay identifies its exact ProspectID and carries Totals
plus `fightercondition.Project(1, originalDNA, originalBirth, asOf, Totals.XP)`.
The raw legacy page is retained unchanged. Recheck the original account lease,
assignment and relevant roster/source revisions before releasing the composite.

No arbitrary XP setter, Apply endpoint, JSON authority import, autonomous mining
or extra network connection is introduced. Held sessions may read. Client/helper
validation recomputes condition and D65 from the authenticated overlay; stale
views, missing binding and mixed page/digest/time responses refuse. Frontend
displays the career condition/record for the new route, not the legacy zero-XP
projection. BasePoints is explicitly base-point history, not a ranking position
or complete rolling league score. There is no new rankings or title page here.

## Verification

Prove prepared/ready mixed-guard recovery, old-constructor refusal, wrong-reader
scope, absent/poisoned reader refusal, held and stale views, Current-versus-noon
eligibility, exact-boundary ordering, saved-card replay and direct-core bypass
attempts. Test original-result activation refusal leaves legacy root readable.
Measure bounded page/cutoff/index and startup behavior; retain the existing
runtime deadlines. Provisional-label checks, native package and isolated normal
PoW end-to-end evidence remain required. No visual or D74 approval is implied.
