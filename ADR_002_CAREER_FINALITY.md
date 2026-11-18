# ADR-002: Derive career awards from durable result finality

**Status:** Implemented and released in Set49; scoped acceptance and deployment closeout passed
**Date:** November 18, 2026 Pacific
**Deciders:** Mike approved product policy D78; Codex selected this implementation

## Context

D78 makes records, XP and all result-derived points/title changes permanent
together at result finality. Set48 already synchronizes the first finality
evidence with its exact result, both fighters and original attribution. Its
publication rules grant no awards. Existing event snapshots and readers have
an explicit zero-XP candidate contract and must keep their historical meaning.

## Decision

Add a separate, explicitly activated career-accounting policy over the same
result journal. Each first post-activation finality is one irreversible event
for both fighters. Derive totals from bounded, audited events; do not maintain
independent mutable per-gym counters or require an Apply action. Retain the
same result transactions, first certificates, identities and funding reservations.
`CAREER_ACCOUNTING.md` defines the implementation contract.

Activation uses a recoverable parent version and result marker/metadata version
that older writers refuse. Reject activation if any retained result already
has finality. Those legacy journals remain readable with their original scope;
never backdate their awards. Provisional rows may finalize after activation.
The original publication-only statement stays unchanged: it supplies evidence,
and the newly activated career policy supplies accounting authority.

A small internal read-handle boundary avoids importing Result back into its
Stable/Entry/Event dependencies. Read handles copy immutable bounded facts
under the result's short mutex; they never traverse source authorities or take
the result action gate. Establish a durable result clock fence before acquiring
consumer gates when taking an immutable historical prefix. Use the existing
strict-before-noon convention so later writes cannot enter a closed prefix.

Keep old receipt bytes and saved bout inputs intact. New current-career views
are explicitly typed/versioned. Core C1 admission must reject nonzero records
at the relevant cutoff, not merely disable a UI button. Current standard
candidate bouts produce record/XP and base-point events; unsupported ranking
context or title classification is never invented.

Proof-backed Career pages use the existing asynchronous Stable job. A cold
eight-proof diagnostic showed that the initial synchronous implementation could
not reliably finish within the short metadata deadline; its failure is retained.
The job keeps its existing 30-second work limit. A separate 500-millisecond read
uses an opaque, original-session-bound handle for the already verified, dated
composite, with fresh source and account checks. It never accepts a caller's page
or repeats Argon hashing on that short read. The complete handle and terminal
page both count toward retained-memory limits. This corrects scheduling without
changing proof checks, page size, award timing or either deadline.

## Options considered

| Option | Benefit | Cost and risk |
| --- | --- | --- |
| Derive from first finality (selected) | One durable event accounts for both fighters; restart recomputes the same totals | Requires explicit activation, bounded projection and careful source/consumer lock order |
| Separate per-gym award journals | Independent local summaries | Needs cross-store transaction recovery; partial or repeated awards can diverge |
| Apply provisional awards then reverse | Earlier apparent career changes | Conflicts with approved D78 and would require rollback through ratings and snapshots |

## Consequences and verification

The record and finality cannot commit separately, and later chain-health
observations cannot subtract a finalized event. Reads still need original
account authorization and exact source-incarnation binding. The internal
handle is trusted application wiring, not a claim that arbitrary in-process
code cannot fabricate facts.

Activation/recovery, poisoned writes, wrong source, replay/re-inclusion,
noon ordering, C1 refusal, XP cap, bounded startup/reads and every implemented
provisional display are covered by the recorded Set49 gates. The scoped-clock
normal-PoW composition verified a genuine first-finality award through restart,
conflict and held reads; the normal-calendar Mac package separately verified
activation and zero-award pages. Its nine independent package reports and the
full 900-second, 31-sample deployment observation passed, including restart.
Release `0a4374de` and the preserved failures/scope limits are recorded in
`CHANGESET_49_NOTES.md`. Remote CI returned no runs/checks and is not claimed
passed. D74, production authority and future ranked/title mechanics remain open.
