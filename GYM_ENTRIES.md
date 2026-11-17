# Candidate Saturday entries

Set 43 adds reversible entry intake to the existing candidate gym. A currently
authorized fixture account chooses a Saturday and up to five rostered fighters,
one per weight class. Save replaces that date's selection; Withdraw records an
empty selection. Both use a durable prepare/apply operation and a revision
check. A prepared intent is not a saved entry. There is no fee or automatic
entry, and editing the roster does not silently rewrite a saved entry.

This slice closes intake at noon Pacific. It does not lock fighters, choose
future-block entropy, make matchups, run an official event or award results,
XP or training. Closed intake is reported as waiting for event locking.
The two published fixture accounts, candidate labels and pending D74 joint
product review remain in force.

## Authority and recovery

The new `gymentry.Store` borrows one concrete `gymstable.Store`, which borrows
the original admitted manifest source. Entry pins include the full stable pin,
account authority namespace, calendar and rule identifiers. Browser clocks,
receipt imports and caller-provided items cannot establish admission or cutoff.
The private account lease preserves the original token, generation and view.
Current held sessions can manage these reversible entries; D72's asset,
recovery and marketplace restrictions are unchanged.

Apply validates the original view, expected entry and roster revisions, current
roster membership, class uniqueness and retirement. At most five retained
proofs are checked serially before the final commit guard. Stable observation
writes finish before the entry commit callback. The final authority clock
sample decides cutoff: equality is closed. Time spent preparing or checking
does not reserve admission. Accepted time is the final pre-write sample;
later filesystem acknowledgement does not change that recorded time.

Each new committed operation increments the entry revision. Exact replay
returns the original receipt without changing its time or revision. Historical
queries preserve their original author view inside a current authorized query
envelope. A new login may inspect or abandon an old pending intent, but cannot
Apply it. Abandoning a pending intent does not withdraw a saved entry.
An uncertain write requires close/reopen and a durable operation query; it
cannot be reported as a definite cancellation.
Failure to project a response after a successful commit likewise reports an
unknown outcome. The client queries that exact operation rather than repeating
the mutation. Abandon on an already committed operation returns its committed
record; it does not undo the saved entry.

## Calendar and storage

The supported dates are Saturdays in 2026–2099. A pinned, embedded
`America/Los_Angeles` TZif supplies noon Pacific, including the future POSIX
rule tail; the host timezone is not authoritative. Upcoming means the earliest
supported cutoff strictly after authority time. Pending and historical dates
never automatically roll forward. Clock rollback fails rather than clamping
an older time into a new authorized save.

The entry store lives at `gyms/<account-id>/entries` with its own marker and
LevelDB. Bounded opening audits its complete event/operation graph and canonical
records. Cheap reconstruction is not a substitute for cryptographic proof
verification. There are at most 256 dates, 4,096 operations and one pending
operation; capacity is reserved so an existing pending operation can finish.
No credentials or recovery secrets are stored in entry records.
Opening and metadata reads do not run Argon or regenerate SVG. They check
structural continuity, canonical bytes and monotone source/checkpoint/roster
history; Apply and Check authenticate selected records against the trusted
source. The unsigned local journal is not protection against an attacker
rewriting every related record consistently.

Explicit parent schema 3 retains all existing stable installations while
adding entry installation state for both fixtures. Installation resumes from
prepared state; a missing ready child fails instead of silently being recreated.
Only the new entry route reads/writes schema 3. Existing candidate and stable
routes remain explicit and do not silently migrate it. The entry marker limit
is separate from the old parent marker limit.

## Desktop boundary

The separate closed Entry request/response family shares the existing helper
pipe and single active/single queued work lane. Native methods derive the
entry pin from the complete authenticated stable snapshot and correlate full
view/store/event/job/operation tuples. Apply's operation lookup runs inside
the already held pipe gate. Completed job payloads are immutable, including
repeated polls. Snapshot-discovered IDs require complete correlation before
an action or result is accepted.

Metadata operations use the bounded lane and do not reverify source proofs;
snapshot/job polling reads bounded caches. Entry data has a 128 KiB share
inside the existing aggregate 1 MiB presentation bound. Cache admission is
reciprocal with inventory and preview data. The frontend keeps explicit
selection separate from fixed-page navigation and Follow Recent. Save may
continue from Prepare to Apply only under the exact original view/selection
epoch. Unknown outcomes query durable state before another mutation.

The helper requires all three explicit flags: `--candidate-manifests`,
`--candidate-stable`, `--candidate-entries`. Ordinary flags default false.
The separate Candidate Pilot build enables them without changing ordinary
account/manifest/stable routes. It remains an ad-hoc signed fixture build,
not real signup or public distribution.

## Independent evidence and limits

The immutable contract and reference source are retained under
`gymentry/testdata/reference`. The reference producer used frozen prior inputs
and the exact contract while runtime authors worked separately. The execution
order amendment is recorded: expected outputs were not generated from or
adjusted to the new Go implementation. The complete corpus was frozen and
independently audited before runtime comparison was authorized.

Comparison later exposed two nested canonical-order expectations that
disagreed with the written contract. The original complete reference is
preserved in Git commit `b73c4da3`; the active successor's `correction.json`
identifies both changed strings. They now match positive examples already
present in the original frozen oracle. No values, acceptance flags, other
cases or product rules changed, and the correction was independently audited.

The corpus covers all 3,861 supported Saturdays, both fixture namespaces,
canonical IDs/bytes/digests, strict codecs, protocol correlation, state
transitions and retirement boundaries. Six retained genuine normal-work
records cover three weight classes. Separate synthetic five-class examples
test structure and state only; they are not claimed as admitted proof evidence.
Runtime comparison executes 449 core codec cases, 36 canonical core/disk DTOs,
both receipt digests and all calendar boundaries. The 54 reference state
scenarios are retained specifications; they are not all replayed by the Go
test bridge. Runtime transition, capacity and crash tests supply separate
evidence for implemented state behavior.
Filesystem/process faults, real proof checks and packaged helper acceptance
require their own execution evidence. Neither these tests nor DOM tests prove
actual window rendering, native chooser behavior or screen-reader usability.

See `CHANGESET_43_NOTES.md` for current validation status and
`LAUNCH_READINESS.md` for the remaining release gates.
