# Candidate stable

`gymstable` keeps an immutable candidate assignment prefix and ten reversible
roster choices for one of the two published fixture gyms. It does not establish
transferable ownership, activate an official generator, award experience, or
create a league entry.

Create or open a store with a trusted `Pin` and the corresponding concrete
`registry.ManifestStore`. Keep that source open until the stable has closed.
The source cannot be replaced with parsed receipts or an application cache.
Every operation takes a `Session` whose trusted synchronous lease captures the
original account, generation and view. Never invoke an operation while already
holding that account lease, and do not reenter the stable from its lease.

`Sync` verifies and assigns at most eight consecutive source records. Inventory
and roster pages also verify at most eight records, serially. They compare each
receipt to its retained full-record digest before deriving condition at a
durably observed time, with professional XP fixed to zero. A successful source
count observation is retained even when no new assignment is committed. Open
checks the bounded local journal graph; source-record drift is detected when
that record is subsequently accessed, rather than by rehashing all history on
every open or status query.

Roster changes use `PrepareSlot` followed by `ApplySlot`. One pending intent
reserves completion capacity. Keep its operation ID after an uncertain result;
query `Operation` or `Pending` before retrying. A new view may inspect or abandon
an old intent, but cannot execute it. Committed outcomes remain immutable and
queryable. Reads and clock updates do not change the roster CAS revision.

An uncertain database write poisons the handle; close and reopen to reconcile
the retained journal. Close cancels queued work and joins the real source call
already in progress. Source APIs do not support interrupting an in-flight proof,
so no following verification starts after cancellation. UI callers should run
these operations in their tracked work lane and poll an owned status cache.

The fixed capacities are 1,000 assignments and 4,096 retained operation intents.
They are fixture resource bounds, not gameplay ownership limits. `slugtestpow`
refuses constructors and source-trusting operations before storage or disclosure.
The independent expectations in `testdata/reference` retain their own provenance;
expanded signed records used in behavioral tests are not golden outputs.
