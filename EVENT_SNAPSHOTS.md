# Candidate Saturday snapshots

Set 44 implements the candidate foundation for D76/D77 in the existing Go
Candidate Pilot. The accepted policy and its rationale remain in
[EVENT_LOCK_POLICY.md](EVENT_LOCK_POLICY.md). These snapshots are conditional
local authority records for the two published demo gyms, not transferable
production ownership or official league events.

## What is fixed at noon

For a supported Saturday, use the pinned Pacific calendar's noon cutoff `C`.
Reconstruct committed roster and entry operations with `committed_at_ms < C`,
in original journal sequence order. A write sampled before noon and retained
after a slow synchronous write still belongs to that history. A prepared
intent is not a saved entry; a saved withdrawal is an authenticated empty
selection. Exactly-at-noon changes are excluded.

The complete ten-slot historical roster is retained. Each submitted fighter
is independently reverified against its exact source assignment and normal
256 MiB PoW record, then projected to noon using condition v1, original birth,
XP zero and the unchanged D65 rating. Saved members remain visible even when
excluded: retirement at noon takes precedence over absence from the roster.
Retirement or roster changes after noon do not alter this event. Live aging
and D39's separate title-vacancy rule continue.

Ownership provenance is explicitly `candidate_assignment`, bound to the
original account authority, account, gym and source. D76's official ownership
requirement still needs a transferable ownership authority. No candidate
record can later be relabelled an official snapshot.

## Capture and recovery

One separate `gymevent` coordinator fixes both complete participant pins
before the desired cutoff. Its immutable creation time is sampled only after
the pin is durable. Per-Saturday rows can first materialize later; creating
the coordinator at or after a Saturday's cutoff cannot authorize that past
event. A partial initial installation can resume only through the exact
parent-prepared recovery path. An established coordinator is never recreated
when missing, and a completed creation timestamp never changes on reopen.

Each demo gym captures its own history under its current original View
session. Capture may occur after noon; the recorded capture time is the actual
observation, distinct from the historical cutoff. The event stays incomplete
until both fixed gyms have contributed. An absent history source, invalid
proof, clock disagreement or corrupt journal is an error, not an empty gym.
Current held sessions may capture and read; D72's asset/recovery restrictions
are unchanged. There is no unattended scheduler credential in this slice.

Capture drains the actual entry and roster writers, checks the complete
bounded journal graphs, and synchronizes actual clock floors before committing
the contribution. Later writes cannot insert a newly authorized pre-noon
operation after clock rollback. Committed historical prefixes determine the
snapshot digest; prepared or lawful later tails do not. The second capture
and the complete receipt commit atomically. Retries return the original capture
bytes, including after a new login. A new login never authorizes an old
in-flight task.

Uncertain writes poison the affected handle until explicit reopen and audit.
Lost acknowledgements reconcile the same event and participant, not a newly
chosen identifier or timestamp. Cancel/Stop joins real work and does not undo a
completed capture. Source verification and condition calculations run outside
the final account lease; publication rechecks that original View.

## Desktop route and bounds

The explicit `--candidate-events` route requires candidate manifests, stable
and entries. The Candidate bundle selects that route; ordinary constructors
retain their earlier behavior. The same account root upgrades through a
recoverable v4 marker. Both existing fixture installations must be ready before
the shared coordinator can be initialized. Until then, snapshot setup reports
incomplete while login, mining, stable and entry controls remain available.

The helper owns the shared event store across account switches. A child borrows
it only within the existing joined operation lane. The native bridge pins the
complete coordinator identity and creation time for its helper lifetime, then
correlates every response with the current participant, view, event and job.
Historical captured author information does not grant current authorization.

Capture shares the existing single-active/single-queued lane. There are at
most two participants, five selected fighters per participant, 256 events,
4,096 operations per source journal, eight metadata summaries per history
page, 8 KiB requests and 128 KiB responses. Helper event cache reservation is
128 KiB within the existing aggregate limit. Full Record/History responses are
transient reads; they do not create a second retained helper transcript.

History cursors are exclusive retained event IDs. `next_event_id` is the last
returned ID only when `has_more` is true; otherwise it is empty. This exact
terminal behavior is recorded in contract clarification 1.

## Seed and incident boundary

`seed_authority` is always `unavailable` in this version. There is no selected
seed, finality assertion, pairing, result, XP award or observed chain incident.
The UI reports that missing integration explicitly.

The incident deadline is the following Saturday's noon in the same frozen
Pacific calendar, including 167/169-hour DST weeks. The final supported event
on December 26, 2099 has its deadline on January 2, 2100 without admitting 2100
events. D77 requires the later seed authority to wait and visibly report
delay/conflict, never substitute a seed or automatically cancel, and escalate
an unfinalized seed at that deadline under the report-only checkpoint/community
rule. The calculated deadline alone does not prove a chain incident.

## Verification scope

Exact core/helper contracts, reviewed size calculations, independent Python
inputs and the final reference freeze accompany the reference test data.
Reference bytes are derived independently; modeled journal/crash scenarios
are not claims that the runtime executed them. Implementation tests separately
exercise real normal-PoW sources, leases, source gates, synchronous journals,
lost acknowledgements and process kills.

The separate `slugtestclock` tag exposes only scoped fixture constructor
wrappers for tests. It keeps normal PoW, immutable source receipts and original
births. There is no release CLI, environment or request field for changing the
authority clock. Ordinary constructors keep real clocks, and the bundle build
checks its binaries for either diagnostic tag and test-clock symbols.

Before the next eligible real Saturday, closed-event helper composition uses
that explicitly labelled test clock. Real-clock package acceptance covers
setup, pre-cutoff and too-late refusals, history, account transitions and
restart; it cannot claim a successful real-clock closed event. That remaining
coverage stays explicit. No automated or code check replaces Mike's D74 joint
design review, the pending portrait-render check, signing/notarization or
production issuance/genesis gates.
