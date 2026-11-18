# Retained candidate bouts — set 47

Set47 connects one booked candidate BoutID to an explicitly requested fight and
an immutable saved transcript. It uses the existing C1 two-fixture card and the
unchanged standard three-round `fighterbout` v1 engine. Implementation and
acceptance progress are recorded in `CHANGESET_47_NOTES.md`.

The exact frozen contracts are retained beside the independent reference:
[core](gymbout/testdata/reference/inputs/contracts/set47-core-contract-draft.md),
[desktop](gymbout/testdata/reference/inputs/contracts/set47-helper-contract-draft.md)
and [source dependency addendum](gymbout/testdata/reference/inputs/contracts/set47-card-dependency-addendum-draft.md).
Original draft filenames remain unchanged; the separate freeze records bind
their approved bytes.

## Fixed inputs and immutable replay

A request supplies only its original authorized view, Saturday and booked
BoutID. The concrete card supplies the two ordered fighters, original birth,
canonical manifests, noon condition and fixed bout seed. The engine runs at
that original noon cutoff with zero professional XP and zero training bonus.
Current age, retirement, roster changes and later ownership cannot replace
the booked inputs. D65, D75, D76 and D77 remain unchanged.

A successful first Run synchronizes the complete Input and Result together
with their identities and original creation time. Repeated Run returns that
same result without simulating again. A current source conflict prevents a
first Run; an already saved result remains available under a current authorized
view with a separate historical/conflict status. Nothing substitutes a seed,
rerolls a fight, or automatically runs on navigation or polling.

The record is a candidate simulation transcript. It awards no professional or
exhibition record, XP, training, title, prize or ownership, and it is not an
official result certificate. Unmatched fighters remain unmatched; they gain
neither a bout nor a loss. The card retains all entry counts and reasons.

## Source and storage boundaries

The card store projects a compact source from its audited retained card. The
projection commits omitted card evidence but is not a portable admission or
ownership proof. New Run prepares its source outside all account leases and
lower source gates, holding only the card resource gate. Before commit it
reacquires the concrete source, checks the original session and exact immutable
source, and requires the source still be usable.

The separate journal retains at most 256 bouts. Open serially validates every
complete deterministic transcript, then retains only a bounded private digest
and summary index. Selected reads compare the actual stored bytes against that
audited digest before disclosure. Metadata, saved replay and cached polling do
not run the engine. Explicit source reads still include the existing bounded
card/seed semantic audit; that cost is not represented as free.

The v7 candidate parent owns the bout journal across account changes. It
records prepared/ready installation stages, preserves earlier component
identities, and refuses missing established storage. Close drains active work
before closing shared bout, card, seed and event stores. An uncertain write
requires reopen and an explicit saved-result check, never automatic Run.

## Desktop behavior and bounds

The Saturday card lists booked bouts as not run or saved. Run is explicit.
Opening a saved result starts a paused local replay using its retained ticks,
rounds and scorecard. A newer explicit source conflict cannot be overwritten by
an older completed job. The UI distinguishes cutoff facts, saved results and
the last source observation. Held current sessions may use these non-spending
controls; revoked or expired views cannot disclose results.

One selected transcript and at most two native-rendered portraits share the
new 384 KiB family allowance within the existing 1 MiB aggregate. Every portrait
disclosure, including a cache hit, requires a fresh matching Status under the
same original view. No webview-supplied manifest or SVG is accepted.

The frozen engineering targets remain 250 ms for explicit metadata, 30 seconds
including queue time for Run, and a 25-second startup admission/audit budget
inside the native 30-second first request. These are acceptance requirements,
not a substitute for the measured checks in `CHANGESET_47_NOTES.md`. Set46's
separate Card 500 ms amendment does not widen them.
Normal release builds contain no test clock or cheap PoW.

The independent reference predates the Go implementation. It includes one
hypothetical composition from the genuine retained conflicted card and ten
explicit model-only projections. Its portrait placeholders check envelopes,
not rendering. Actual owned-chain execution, restart, native authorization,
cache/capacity and latency checks remain separately required. The original
detailed app icon and simple letterless menu icon are preserved; GUI/portrait
rendering and Mike's thorough joint D74 review remain product-release gates.
