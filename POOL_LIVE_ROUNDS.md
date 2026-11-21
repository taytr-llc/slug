# Live round maintenance

Status: Set77 source staged; runtime qualification pending. This is an explicit
maintenance optimization on existing round Stores, not a new journal or source
profile. Both supported round versions keep their original limits and bindings.

## Original owner and verification epoch

`Store.BeginLive(ctx)` performs the complete consumed-source and namespace
audit before first selection. It builds a private bounded directory of exact
metadata, sequence indexes and record digests. Repeated selection of a healthy
epoch returns the same handle. The handle owns no files or goroutines, accepts
no replacement source/node and expires with the original Store or revoked
epoch. Canceling the selection request does not cancel a successful handle.

`Live.List(ctx, after)` returns owned pages of at most eight committed records.
It checks original source liveness, metadata and each touched index/record.
It consumes no new source tail and makes no node call. `Live.Refresh(ctx, hash)`
checks one original record and its exact winning source entry, obtains the
existing fresh node observation, then checks that winner again before commit.
Confirmation and allocation rules are unchanged: later conflicts do not erase
the first 60-descendant confirmation or redistribute an old round.

Live reads do not promise immediate detection of unrelated untouched disk
mutation. Full `Sync`, `Get`, `List`, `Refresh`, `WalkSettlement`, audit and Open
retain their complete source/namespace validation. Financial import and signing
continue through those full checks; a live directory supplies no payout permit.

## Commit and runtime behavior

Full `Sync` remains atomic over one captured accepted tail. All source, old
prefix, record/index and namespace checks finish before its one synchronous
write. Capacity does not mask corruption, and failure commits no partial tail.
Known successful Sync/Refresh writes update the verified directory only after
durable success. A successful mutation without a verified delta revokes it;
an uncertain write poisons the Store and revokes it. Close drains the original
gate before releasing the Store.

`Runtime.EnableLiveRoundMaintenance(ctx)` selects this path before first Run
through the original local action lifetime. Concurrent Run refuses during
selection, and Close cancels and joins it. Default maintenance is unchanged.
Selected cycles still call full Sync first, then rotate through at most eight
records. At capacity, existing rounds can still refresh without consuming the
blocked source tail. Only exact `ErrLiveStale` permits one explicit full
`BeginLive` reanchor on the same Store and retry of the same target. Corruption,
unknown writes and closed owners do not trigger fallback. Existing 30-second
operation budgets are unchanged.

D84 still defers the entire payout when it cannot fit: no funding subset, new
signature, fee or daily-slot consumption. Original pending signed bytes recover
first. This change adds no financial or transaction authority.

## Scope remaining

The directory is bounded by selected MaxRounds; cold Open, first Begin, full
Sync and financial audits still scale with history. Source2's 8,192-work and
16,384-submission ceilings and daily settlement's 16,384-credit and 32,767-batch
ceilings remain finite. No persistence of trusted verification state, rollover,
pin change or V1 migration is introduced.

Local full accounting/operator normal/race, profile refusal, vet and all-module
build qualification pass, as does normal-PoW seed sync/checkpoint/restart smoke.
See `CHANGESET_77_NOTES.md` for the retained test-fixture correction and evidence.
The DailyV2 public operator still needs explicit transport/client, setup/ingress
and native source integration.
Signed publication limits, V2 full-intake late winners with unchanged reserved
payouts, sustained ordinary share/refresh traffic and host recovery remain
separate acceptance work. No UI reorganization or onboarding is included;
D74 remains the final joint product review.
