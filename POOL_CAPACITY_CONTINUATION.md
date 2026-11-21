# Pool capacity continuation

Status: Set75 streaming and the fresh Set76 V2 daily foundation are locally
qualified, including fresh-process financial recovery and all seven exact-tip
GitHub checks on chain2ca9fc16 and walletdfc5d9dd.
This is engineering under D58/D79–D81/D84, not public deployment or launch approval.

## Identity and compatibility

Source, settlement, wallet, scheduler and signed publication limits are bound
into their selected identities. Removing rows, resetting an accountant or
repinning an old wallet cannot extend them safely: original D80 jobs, open
round windows and reserved payments still matter.

V1 constructors and clients remain strict. Set76 uses fresh explicitly selected
V2 roots and new source/config identities; it does not adopt or migrate any V1
root. Schema16's guard grammar and derivation remain unchanged, with the new
binding selected at creation. One original settlement Store and backend retain
payment authority. A future continuation format must be explicit and preserve
old jobs, duplicate guards, financial provenance and pending signed bytes; it
cannot silently reinterpret lifetime ceilings as page limits.

## Completed foundation and its limits

Set75 streams original accepted-source pages and completed rounds, preserving
the exact global prefix, open window, duplicate guards and allocation rules.
Full source and stored-namespace validation precedes callbacks, credit import
or commit. Capacity must not mask corruption. `WalkSettlement` keeps the
original gate, returns owned bounded pages and no source config on failure;
its callbacks must not reenter or close that owner. Legacy encodings remain
unchanged. Global duplicate sets and cold validation still grow with history;
this is not constant-memory or resumable lifetime auditing.

Set76 adds fresh daily source/round modes and indexed settlement with immutable
batch bodies separate from delivery state. Original-owner recovery resolves a
signed pending batch before new credit or payment preparation. The daily wallet
requires that Store's active exact-request capability; node evidence grants no
signing authority. Explicit daily scheduling retains the same Store, recovery
ordering and UTC rules. Component normal/race/profile/vet checks pass, including
71 top-level ledger/scheduler tests per mode.

These are finite ceilings: Source2 permits 8,192 works and 16,384 receipts;
daily settlement permits 16,384 credits and 32,767 batches. The warm source
memo avoids repeating pure retained-proof hashing, not canonicality, expiry or
authority checks. Cold reopen still verifies the original source.

The 17-target build/full operator normal/race closeout and normal-PoW seed
smoke/restart pass. Actual attempt3 passes 721 canonical rewards, a 721-input/
64-payee payout, unchanged A/B bodies, lost-response recovery and one change
with 721 origins. Its owner reopen uses the same process and retained wire hash
cache. Fresh-process attempt4 also passes: its cold source audit took 142.534
seconds, exact B recovery added no broadcast, all balances and reserved bodies
stayed unchanged, and every owner/node joined. Exact-tip CI also passes. The test-only settlement day does not qualify real-day
scheduling. Winning jobs alone do not model continuous mining by 64 miners,
ordinary share volume or refresh/expiry traffic, nor do they cross Source2's
lifetime caps. See `CHANGESET_76_NOTES.md` for evidence and preserved results.

## Whole-payout deferral

D84 requires the complete ready payout to fit. Observation pages never select
a funding subset. If the transaction cannot fit, retain all balances, sign and
send nothing, leave the daily slot unused and report `transaction-capacity`.
Never choose oldest-first inputs, split the payout, add a miner surcharge or
advance reserves. An existing signed batch still recovers exactly first.

Legacy signing remains bounded at 256 inputs and 100,000 raw bytes. Explicit
daily signing allows at most 1,464 native inputs and 256 payees, subject to the
complete 400,000-weight limit. Daily node evidence can inspect the full bounded
funding frontier with four joined workers and the original 30-second operation
limit. Waiting need not shrink an oversized batch; show the capacity hold.

## Remaining integrated acceptance

- Explicit V2 public receipt/history transport, operator setup/ingress and
  native composition, with independently selected source and recipient bindings.
  Existing V1 clients must refuse the new identity and sequence range.
- Signed admission/ownership publication capacity under its original authority
  rules. Increasing local storage does not enlarge a signed profile.
- Continuous source and financial-history retention with bounded verification,
  global deduplication and exact carry. Any rollover or later adoption needs an
  explicit format and crash/restart proof; old roots remain preserved.
- Actual V2 full-intake late winners and boundary/restart recovery with a pending
  payout: one credit, unchanged reserved transaction, no share reuse or duplicate
  payment. Preserve Set74's V1 proof without relabeling it as V2 acceptance.
- Sustained ordinary-share/job-refresh load for the intended miner population,
  host disk-full/retention/recovery operations and real scheduled payouts.

Public pool operation and payouts precede sign-in/onboarding. D74's full joint
product organization/design review remains last before release.
