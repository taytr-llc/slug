# Shared fixture entries and noon snapshots

Set63 connects the Pool Pilot's mined fighters to the existing Saturday entry
and noon-snapshot controls. It keeps the same shared source, wallet recipients,
Pin2 stables, layout and logos. This is candidate engineering with the two
published fixture gyms. It does not activate production ownership, seed
finality, booked fights, results or career awards.

## Authority and retained history

The parent runtime owns one event coordinator across sign-ins. Each child
opens its own entry store beside its existing stable and borrows the event
handle. Both participant pins use the exact shared source, account authority
and original stable IDs. Original global receipt sequences remain unchanged;
local assignment counts remain distinct from those shared-source cursors.
Nothing copies admissions into solo stores or repins earlier event histories.

League setup runs after both actual fixture wallets have verified and before
services become ready. The separate `shared/league-v1` directory contains an
immutable `SLUG-POOL-LEAGUE-INTENT.json`, one `<accountID>-READY.json` for each
participant, `SLUG-POOL-LEAGUE-READY.json`, and the `events` store. Entry stores
live at `shared/gyms/<accountID>/entries`, beside their original stables.

Intent records the complete expected binding and whether each stable already
existed. A new component can be created only under that exact intent before
its completion marker. Once either league or child completion establishes a
store, its disappearance is retained-data loss and never triggers recreation.
A completed matching store whose completion acknowledgment was lost is opened,
verified and closed before the matching marker is synchronized again. Original
store IDs and creation timestamps survive retry.

Empty new directories can resume initialization where the core constructor
supports it. Nonempty incomplete stable/entry initialization fails closed and
retains the evidence; the desktop does not delete it or infer missing authority.
The event store's documented prepared state uses its existing ResumePrepared
operation. Orphan artifacts, conflicting pins, symlinks and noncanonical managed
paths are refused. Normal startup has no fake-clock option.

## Entry and noon behavior

Players select rostered fighters, prepare an exact entry choice and apply it
under the original authenticated view. Saved entries, pending operations and
unknown outcomes keep the existing explicit reconciliation behavior. A reset
revokes the old view; a replacement session can read retained history without
authorizing an earlier session's pending mutation. D72 held sessions retain
allowed roster, entry and viewing actions; recovery export remains blocked.

D76 fixes roster membership, candidate assignment, eligibility and condition
at noon Pacific. Each signed-in gym captures its own historical state. The
coordinator visibly distinguishes one captured gym from a complete two-gym
snapshot. Later roster changes do not rewrite that snapshot. Replay preserves
the original participant evidence, completion time and combined digest.

The shared source and event coordinator outlive logout and reset. Child shutdown
joins its lane before closing its entry/stable handles. Runtime shutdown drains
children before closing the event coordinator and shared source. Backup work
blocks new lane dispatch, cancels and joins active work outside account leases,
and leaves job polling/cancellation available. Backup completion never starts
mining. Only explicit Start mines.

## Protocol and interface

Entry and event operations reuse their existing closed wire families through
narrow surface interfaces. Pool snapshots remain a separate type; the helper
does not pretend to supply legacy solo snapshots or enable practice to satisfy
an unrelated interface. The outer request scanner admits the existing entry
family's 8192-byte bound while each family keeps its own parser and limits.

Native event preparation reads the actual pool, stable and entry views and
rechecks the same original view at dispatch. Both native and browser consumers
require both event participants to be Pin2 with the exact shared source. The
legacy Pin1 route keeps its existing independently pinned authorities. Signed-out
and replaced-view responses cannot retain another gym's presentation.

Existing entry and snapshot panels are enabled without navigation changes.
Practice, seed publication, cards, bouts, results and career remain inactive
until their concrete shared authorities attach. A completed noon snapshot is
not a finalized fight result. The final joint D74 review remains before any
friends/tester or public release.

## Qualification scope and next integration

Normal-PoW integration uses actual shared mining, wallets, admissions and stores.
Only the explicitly tagged test constructor advances stable/entry/event calendar
observations to noon; accounts, sessions, mining jobs, reset holds and settlement
keep real clocks. Normal packaged binaries contain neither this constructor nor
any runtime clock control. Normal-clock package acceptance covers real upcoming
dates, before-noon refusal, source/authority continuity, backup and clean restart.

The next integration needs an explicit shared-owner publication adapter over
the parent chain, with separately funded fixture carriers. Never silently spend
share-accounted pool rewards on event publication. D77's fixed seed, D62's
report-only checkpoints, C1's scoped candidate matching and D78's finality rules
remain unchanged. See `CHANGESET_63_NOTES.md` for completed evidence and limits.
