# Selected DAILY wallet pipe v1

Source contract for the first interactive slice. Existing V1 remote frames and
the automatic DAILY CLI remain unchanged. Selection is the existing
`--daily-recoverable-issue` plus the new explicit `--daily-wallet-helper` flag;
the helper starts signed out and awaits fixed commands. Both flags are required
for this route; mixed legacy/false/duplicate flags refuse before I/O. Only the
complete original RecoverableRemotePoolConfig is accepted. No persistent bytes,
remote HTTP contract, credential audience or financial policy change.

Canonical JSON object followed by exactly one LF. No CR, non-ASCII, whitespace
variants, extra/duplicate/reordered/missing fields, null integers or numeric
strings with signs, exponents or leading zeros. Readers bound before allocation.
The canonical serializers and parser validators are separate from V1. Limits
include LF: request2048 bytes, response65536 bytes. Decimal strings cover all
uint64 request/operation/capture/cursor fields and cumulative int64 atom totals.
Existing bounded numeric page transaction amounts, times and local wallet values
remain their original types; every numeric value crossing JS stays safe.

Ordered request fields:
`version, application, id, command, config_id, view_id, fixture, sequence,
capture_sequence, before_batch`.
Version1, application literal `SLUG-REMOTE-DAILY-WALLET-1`, id decimal uint64>0.
First request id1 is snapshot with empty view; IDs strictly increase. The three
operation fields are canonical uint64 decimal strings, zero for control commands.
Five existing control names are snapshot/login_fixture/start/stop/logout.
Snapshot has empty fixture; login has exact selected fixture and empty view;
all other controls have exact original view and empty fixture.

Three new fixed commands:

- daily_payout_begin: sequence>0; fresh capture_sequence0/before_batch0, or
  exact retained capture sequence and nonzero before_batch. Native caller supplies
  and remembers the operation sequence before dispatch. Web callers use decimal
  correlation values; native/controller enforce exact sequence, not authority.
- daily_payout_read: sequence>0; capture_sequence0/before_batch0.
- daily_payout_cancel: same request fields as read. Success returns the exact
  read after cancel/join; it does not start another operation.

Ordered response fields:
`version, application, id, command, config_id, view_id, ok, snapshot, job, read,
error`. Correlation fields equal the request. Required result arms are null
unless selected: controls use snapshot; begin uses job; read/cancel use read.
Refusal has all three null, ok=false and one closed error. Success has error="".
Snapshot reuses the explicit RemotePoolWireSnapshot field order with Version2
and its actual selected controller values, validated independently of V1. An
initial snapshot is signed_out; login publishes the actual original stopped
View; no subsequent response can replace it. All selected config/source/local
identity/wallet limits and before/after authority fences remain enforced.

`job`: ordered `sequence, capture_sequence, before_batch, state, error`.
States working/done/failed/cancelled. working/done/cancelled require empty error;
failed requires busy/rate_limited/stale_capture/unauthorized/unavailable/cancelled.
`read`: ordered `job, capture`; capture may be null or a separate dated previous
success. It must not adopt a failed job's sequence.
`capture`: ordered `sequence, binding, captured_at_ms, stale, first, current`.
`binding`: ordered `view, config_id, source_binding_id, settlement_config_id,
recipient_script, issue_profile_id, original_source_binding_id, wallet_owner_id,
allocation_binding_id`. All equal the independently selected config and original
View; issue_profile_id is required and nonempty for this recoverable route.
first/current use unchanged DailyPayoutPage ordered canonical JSON. They match
the exact source, settlement, recipient, gym, revision, high batch, balance and
observed_at_ms. First.BeforeBatch=HighBatch+1; current is first or an older
eight-global-slot window; only Done is EOF. No merged history or zero-on-error.

The native boundary remembers the caller-known sequence before Begin and never
silently selects another after uncertainty. Reads/cancels use that exact value.
The original controller validates monotonic sequence and exact repeats. A lost
frontend acknowledgement can query the same sequence; broken/malformed pipe
retires/joins the same helper. No replacement process or automatic recapture.

Closed envelope errors: invalid_request, stale_view, busy, unauthorized,
forbidden, cancelled, unavailable, stale_capture, not_found, rate_limited,
outcome_unknown, cleanup_incomplete. Lifecycle mutations retain existing
uncertain-result semantics; no joined cleanup error is downgraded. Exact read
Busy is distinct from authentication/integrity failures. The pipe remains one
request at a time; accepted native jobs live asynchronously under their original
maximum30s lifetime. Polling and cancellation do not repeat a remote audit.

Conservative frame proof to independently cross-check before qualification:
DailyBalance<=17408 existing bytes; each PayoutItem<=256 plus its wrapper<64;
8items add<=2560. All other page scalar fields and punctuation are <1024
(three64hex IDs,44hexscript,four20digit counters,boundedtime/version/bool).
Thus each page<20992; two<41984. Complete binding/View/snapshot/job/envelope
allowance<16384, yielding<58368 including LF, below65536. These disjoint maxima
must be checked against exact structs/literal independent frames, not measured
from production output. This new cap does not admit a larger remote page or
weaken existing validators.

The page renders only the same wallet area; static text/content only. Cumulative
decimal strings never round-trip through Number. Existing signed-out fixture
login and explicit Start are retained. No new login, transaction action, source
selection, arbitrary dispatch or wallet authority is exposed.
