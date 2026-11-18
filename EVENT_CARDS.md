# Candidate Saturday cards — set 46

`gymcard` projects a complete noon snapshot and its fixed, historically
finalized seed into one retained candidate card. The two demo gyms use
exception C1: same weight class, zero professional records and the unchanged
inclusive half-to-double D65 rating band. Only the five-opponent minimum is
waived. See `CANDIDATE_CARD_EXCEPTION.md`; general league matchmaking remains
open.

The exact frozen engineering contracts are retained with the reference corpus:
[`core contract`](gymcard/testdata/reference/inputs/contracts/set46-card-core-contract-draft.md)
and [`helper contract`](gymcard/testdata/reference/inputs/contracts/set46-helper-contract-draft.md).
Their original draft filenames are preserved; the adjacent freeze record pins
the approved bytes. Later standalone-status and literal-ASCII clarifications
are documented in the reference provenance and concrete codec tests.

## Pairings and unmatched evidence

Every fighter in the noon snapshot appears in the card, including fighters
who were ineligible at the cutoff. The card reports total, eligible, paired
and unmatched fighter counts, bout count, and both gyms' counts. An unmatched
fighter has exactly one reason, in this priority order:

1. Retired at the noon cutoff.
2. Not rostered at the noon cutoff.
3. No opposing entrant in the same class.
4. The opposing entrant was ineligible at noon.
5. The opposing rating was outside the half-to-double band.

The band uses integer hundredths: `2 * min(a, b) >= max(a, b)`. The two-gym
profile has at most one opponent in each class, so it makes no random opponent
draw. Entrants are ordered by class, gym and full prospect ID; bouts follow
class order, with corners ordered by full prospect ID. Names come from the
existing deterministic manifest. No new name, record or ownership authority
is created.

Zero professional records are an explicit candidate profile, not an inference
from XP. This set records pairings only. It does not run bouts, issue results,
award XP or training, update records or titles, transfer fighters, or settle
record filters, widening and next-week priority. Unmatched evidence is retained
to inform those later decisions.

## Immutable card and separate source status

Card identity binds the shared card store and source event. Input and bout
identities bind the immutable noon snapshot, fixed target block and C1 rules.
Capture timestamps, later observations and account sessions cannot reroll
pairings. Repeating Build returns the same saved card and creation time.

A source conflict prevents a new card. A saved card remains inspectable with
its original bytes and a separate unusable status. Offline nodes or depth loss
strictly above a historically finalized target do not erase that finality.
The UI distinguishes saved evidence, current local status and the last actual
chain observation; polling an old completed job cannot hide a newer conflict
or observation failure. D76's noon snapshot and D77's fixed seed, waiting and
incident escalation rules remain unchanged.

The source projection contains the retained snapshot, declaration, anchor,
target and first-finality evidence. It is not a portable proof of earliest
publication: the concrete seed journal and its prior ordered scan provide
that authority. Callers cannot submit a replacement source, seed or plan.

## Storage and desktop integration

The explicit v6 candidate parent retains a separate card journal with durable
prepared/ready installation steps. Card setup may complete after noon; the
existing entry, event and seed creation restrictions remain in force. A missing
established journal is refused rather than recreated. The parent owns shared
event, seed and card journals across account switches and closes them only
after draining its child on final Close.

The store retains at most 256 cards. Open fully audits canonical bytes and card
semantics, then builds a bounded private date/ID/raw-byte-digest index. Reads
compare the selected stored bytes with that audited identity before decoding;
the index retains no record payload and accepts no caller validation verdict.
A history request loads at most eight records. Writes synchronize the immutable record and
metadata together. An uncertain write requires reopen and explicit recovery,
not automatic retry or replacement.

`--candidate-cards` requires all earlier candidate flags. Candidate build
0.4.6/build46 enables that complete chain. The helper/native/webview family
accepts only current view IDs, Saturdays, job IDs and history cursors. Build
uses the existing serial work lane; status, record and history reads have a
500 ms context budget and Build has a 30-second queue-inclusive bound. Cached Snapshot/Job polls
do not build cards, derive names or conditions, hash proof headers, sign,
mine or contact a node. The 288 KiB card allowance is charged reciprocally
within the unchanged shared 1 MiB cache; a rejected admission preserves the
previous accepted display.

Actual-clock package testing exposed repeated source/account clock writes that
the initial stationary-clock capacity test did not measure. The corrected read
path authorizes before retained-row validation, then performs one final source
observation and original-session check. Current seed evidence still receives its
full semantic validation. The original 250 ms Card read budget was explicitly
revised to 500 ms after a populated, advancing-clock test with actual account
checks still exceeded it following both redundancy reductions. Earlier failures
remain preserved. This is an engineering budget amendment, with no matchmaking
or authority-policy change. Shorter caller deadlines remain effective; ongoing
synchronous writes are joined rather than forcibly interrupted. Other families'
budgets and the separately proposed Set47 Bout budget do not inherit this change.

The original detailed app icon and simple letterless menu-bar fist are
preserved. Normal bundles omit cheap-PoW and scoped-clock build tags. Set46
verification and rollout status is tracked in `CHANGESET_46_NOTES.md`;
implementation alone is not release acceptance. Portrait rendering and Mike's
full joint D74 design review remain separate product-release gates.
