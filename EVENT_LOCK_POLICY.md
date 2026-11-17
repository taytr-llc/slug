# Event lock and seed-delay decisions

Status: accepted product policy. Decider: Mike Taylor. Date: November 17,
2026 Pacific. D76 and D77 extend the spec; their implementation is tracked
separately. They do not authorize production genesis or close D74.

## D76 — immutable noon snapshot

Mike's decision:

> Fix eligibility and condition at noon Pacific. The snapshot covers roster
> membership, ownership, eligibility, and current condition; later changes do
> not alter the event.

The Saturday card needs a stable, independently auditable set of inputs.
Rechecking live membership and retirement through the evening would remove
already-booked fighters when a roster changes, a sale completes or retirement
arrives. The accepted choice fixes those facts at Saturday noon Pacific.
An eligible-at-noon fighter can complete that event after later retirement.
His live fighter page and eligibility for later events still follow his
original birth and ordinary aging; no age or career extension is recorded.

The event retains ownership/gym attribution at cutoff even if the fighter is
subsequently transferred. It must derive condition at cutoff from the original
fighter, immutable birth and authoritative earned inputs, rather than using
the condition saved with an earlier entry. Later roster edits do not rewrite
the snapshot. Retirement at the cutoff follows the existing condition rule.

This policy leaves D39's immediate live title vacancy in force. A frozen event
participant is not a freeze of belt ownership, and title-award handling belongs
to the later title implementation. Existing D65, condition v1, professional XP
and training rules are unchanged.

The rejected alternative was reserving roster slots and repeatedly checking
eligibility through the fight. It would restrict normal gym activity and make
the published card unstable. The accepted tradeoff is that a locked fighter
may finish that one event after a subsequent sale or retirement, including
when the event is delayed.

## D77 — wait, preserve the seed, escalate

Mike's decision:

> Wait and visibly report delay or conflict. Never substitute the seed. If
> the seed block has not finalized by the next Saturday lock, escalate as a
> chain incident rather than cancelling.

Form must depend on the required future chain evidence under a rule fixed
before the seed can be known. An after-the-fact replacement would let whoever
selects it influence the result. A late or conflicting chain therefore leaves
the original event waiting, with visible status and preserved evidence.
There is no automatic cancellation or replacement seed.

The escalation threshold is the following Saturday's noon Pacific lock.
It is a civil-calendar boundary: across daylight-saving changes the interval
may be 167 or 169 hours. At the threshold, an unfinalized required seed becomes
an operator-visible chain incident. Escalation retains the same event, locked
inputs and seed identity/rule. It does not silently reroll, drop, reschedule
or move the entry to a new Saturday. Later resolution retains the incident
history. The rule applies to the required seed evidence; a local snapshot or
an unverified block is not a finalized seed.

Automatic cancellation was rejected because it discards that week's eligible
participation and disrupts the defense schedule. A bounded escalation makes a
prolonged problem visible without giving an operator a discretionary seed.
Waiting does not stop aging or automatically amend other league rules.

The checkpoint/community doctrine remains social and report-only. It does not
add a chain veto to the signing key. The later seed authority must define and
verify anchor ordering, immutable target selection, finality and conflicts.
No actual operator private key, public transaction or public-chain mining is
needed to implement or test this foundation.

## Implementation boundaries

Set 44 starts with bounded candidate snapshots for the two published fixture
gyms. Their current immutable assignment evidence must be labeled as candidate
assignment; the system has no transferable ownership authority yet. Official
use remains gated on authoritative ownership history at cutoff. This limitation
does not weaken D76's production ownership requirement.

Historical capture may happen after noon, but must reconstruct the cutoff
state from complete audited journals, drain prior writes, retain durable
clock boundaries and reverify selected source records. Record effective cutoff
and actual capture time separately. Missing or uncertain history cannot be
replaced with the current roster or an empty event. Exact byte formats, APIs
and boundary ordering belong in the frozen implementation contract.

The seed integration is subsequent work. A snapshot-only build must not claim
a selected/finalized seed, a public-chain incident or an official fight result.
It can retain the D77 policy and exact following-Saturday threshold without
pretending the chain evidence adapter already exists. No real ownership,
automatic result/XP award, marketplace action or production activation follows
from these decision records. The joint design review with Mike remains required
before friends/tester or public release.
