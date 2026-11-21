# Finite daily pool admission

Set78 adds explicit intake reservation for one original Source2 pool. Local
build, normal/race, profile-refusal and vet qualification passes, as do the direct
normal-node/schema16 fresh-process actual and seed smoke/restart. Exact-tip CI
passes all seven jobs at chain `1f086601647c` / wallet `dfc5d9dd7e0f`; see
[remote evidence](docs/evidence/set78/ci.json). This is a finite kernel addition, not public TLS/runtime
deployment or continuous capacity.

`AttachDailyOperatorAdmission(ctx, expected, budget)` selects the original pool,
round/credit/batch limits and an owned, sorted exact gym set. Composition must
match them to the original round and daily settlement bindings and configured
payees before enabling callers. Session and job-key checks still authenticate
each request; a budget or gym list grants no financial or signing authority.
There is no caller-selected ordinary quota or implicit V1 adoption.

The pure reservation derives an ordinary ceiling of at most 4,095 rows, reserving
one additional canonical winner under the worst all-block partition. It counts
all retained rows and block rows, including pending, rejected, orphaned and
same-funding aliases. Paid or reserved obligations are never subtracted. The
selected smaller actual profile derives two ordinary rows; it is not manually
limited for the test.

Attachment audits complete original metadata, work, rows, nonce/global/accepted
indexes and gym membership before capacity classification. Candidate metadata
cannot replace the owner's current metadata on error. The accepted-window audit
preserves the 4,096-share round limit. Above-ceiling retained rows must already
be accepted canonical winners; incompatible old daily history is held, not
silently granted a reservation. Exact saved-row and financial recovery remain
available. V1 behavior and all durable encodings stay unchanged.

Issue checks membership and reserves worst-work bytes plus the remaining
ordinary receipts and one canonical receipt **before issuer effects**. It checks
actual serialized work and that holdback again before sync. Work bytes, issuer
limits or explicit drain may stop ordinary issuance earlier than the derived
row ceiling. Original source caps still apply: at most 8,192 works and 16,384
receipts, within the selected byte budget. No rollover or history reset exists.

After drain, an unseen original-job winner needs a complete fresh retained
census, sufficient whole-history financial and physical headroom, and fresh
canonical proof from the original node. This path never broadcasts the unseen
block. The original session and final clock checks precede one synchronized
receipt/index/acceptance commit. Lost replies recover the same job and nonce;
no replacement work, duplicate credit or new payment is authorized. A genuinely
full physical journal or insufficient remaining budget still holds recovery.

D84 remains unchanged: if the complete ready payout cannot fit, defer it in full,
retain all balances, sign/send nothing and leave the daily slot unused. Never
select a funding subset, split payments or charge miners the fee. Original signed
pending bytes recover first through the same Store/backend capability.

The independent capacity and actual identity/economics corpora were reproduced
by Python 3.14 and 3.9 before product expectations. See
[Set78 notes](CHANGESET_78_NOTES.md) for identities and qualification status.
The actual proves derived-full two-to-three receipt recovery with real job
expiry, original schema16 custody, fresh-process reopen, one late credit at
60 descendants and unchanged pending payout bytes with one original send.
Preserved runner/checker failures were corrected against the unchanged execution;
see the linked notes and committed evidence. This does not model sustained miners
or share load.
Explicit V2 transport/client/worker, operator setup/ingress, signed-authority
capacity, continued retention and hosted scheduled operation remain gates.
Public pool operation and payouts precede onboarding; D74 stays last.
