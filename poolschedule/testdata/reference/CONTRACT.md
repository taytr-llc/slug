# Set74 standalone payout scheduler v1 — proposed contract

Source-only proposal, 2026-11-20. No installation or execution is authorized by this document. It follows the reviewed external `RecoverPending` draft, preserves D58/D79/80/81, and adds no fee rule, payout authority, reserve funding or missed-day catch-up payment.

## 1. Concrete owner and smallest settlement seam

Add root-module package `poolschedule`. `Create(ctx, path, expected Config, settlement *poolsettle.Store)` and `Open(...)` return one concrete `*Scheduler`; no backend interface, signing/clock callback, receipt import or caller-selected balance is accepted. The original operator runtime opens the exact configured settlement directory with its independently retained source/configuration, then lends that concrete pointer. It retains the fixed root/path association; the scheduler does not discover a store from disk metadata. The runtime alone owns and eventually closes the settlement store. Other consumers may use its existing read APIs; the runtime routes scheduled mutation through this scheduler.

Current `poolsettle.state` persists only `Config` and `poolshare.Config` as its immutable configuration. There is no unique StoreID/path in the store. Add exactly:

`(*poolsettle.Store).SchedulerBinding(ctx) (poolsettle.Config, poolshare.Config, error)`

It uses the existing operation gate, lifecycle/profile/poison checks and complete `verify`; rejects `auditOnly` before disclosure; returns an owned copy of `Config.Destinations` and the value-only source. It creates no metadata, performs no funding RPC and confers no payment authority. Existing `SourceConfig`, `RunDaily`, open/create and read APIs remain unchanged. This combined accessor avoids two full audits and detects a read-only audit handle without a probe mutation.

The logical settlement binding is SHA256 of `ASCII("SLUG pool settlement binding") || 00 || ASCII("v1") || 00 || LE32(len(body)) || body`, lowercase hex. `body` is canonical compact JSON with ordered fields `config,source`, containing the exact existing DTO grammars. Compare it with independently supplied configuration at scheduler creation/open. This digest is **not** a unique database identity, anti-cloning guarantee or spending capability. Fixed original runtime paths, directory locks, retained pointer/lifetime and the settlement's own audited DB provide the concrete association. No private journal copy, replacement pointer or marker-derived expected pin is an allowed recovery operation.

Hold an exclusive schedule-directory lock for the owner lifetime and a private process claim keyed by the exact `*poolsettle.Store` to reject a second live scheduler for that pointer. Release the claim only after joined shutdown and successful cleanup. This claim is process exclusion only; it does not replace durable financial authority. A scheduler never closes or reopens a borrowed store itself. Poisoned/closed settlement requires the original runtime to join owners and reopen the exact original components.

## 2. Immutable configuration and bounded bytes

`Config` ordered fields:

`version,scheduler_id,runtime_id,settlement_binding_id,daily_utc_minute,retry_ms`.

Version is 1. The three IDs are independently retained lowercase nonzero 32-byte hex values; the binding ID is computed as above, the other two are selected before effects. `daily_utc_minute` is an integer 0–1439, independent of local timezone/DST. `retry_ms` is an integer 1,000–3,600,000. Both are immutable operational configuration, not payout economics. No fee setting belongs here. Operational clock is `time.Now().UTC()`; no production setter or arbitrary clock callback. The per-store-call work limit is 30 seconds, capped by original runtime/run cancellation.

Directory layout is `SLUG-POOL-SCHEDULE.json` plus `db/`. Marker is exactly the Config JSON followed by one LF, at most 2,048 bytes. The DB contains only key `state`; one state value, at most 4,096 bytes, compact JSON without LF. Reject symlinks, unexpected entries, unknown/missing/reordered fields, duplicate keys, aliases, non-ASCII, trailing values and unsupported versions. Bounds: nesting 4, object members 16, tokens 128, no arrays, string length at most 64. Timestamps are integer milliseconds in 1…253402300799999, except explicitly permitted zero sentinels. Counters are uint32; overflow refuses and requires an explicit later continuity protocol. No unbounded attempt history or automatic eviction.

Creation refuses an existing directory and syncs the exact expected marker, DB state and parent directory before returning. Open requires both established components, exact independently expected Config and full state audit; it never creates missing pieces or adopts stored IDs. Partial/uncertain creation remains held for inspection. State writes are synchronous atomic replacement of the sole row. Any uncertain write poisons the scheduler; no new settlement call follows an uncertain attempt-intent write.

## 3. State and cursor

State ordered fields:

`version,config,revision,last_clock_ms,daily_cursor,next_wake_ms,attempt`.

`config` exactly equals the marker/expected configuration. `revision` starts at 1 and increases once per state write. `last_clock_ms` starts at creation time and never decreases; equality is allowed. Backward time refuses dispatch. `next_wake_ms` is zero only for an immediate initial/restart recovery; otherwise it is a bounded future retry or configured due time. No timer value authorizes a payment.

`daily_cursor` is empty or canonical `YYYY-MM-DD`, never later than `last_clock_ms`'s UTC date. It records only the greatest date for which a **RunDaily** result explicitly returned `complete` / `daily-batch-complete`. It suppresses redundant daily calls, not recovery. It is not a second slot ledger; only `poolsettle.Batch.Day` can occupy a financial day. Recovery's `complete` / empty reason never advances this cursor. If the cursor is behind today's date, missed dates are skipped; there is no loop over missed payment days.

`attempt` is null initially, otherwise ordered:

`sequence,kind,admitted_day,started_at_ms,finished_at_ms,result,report_state,report_reason,error_code`.

`sequence` starts at 1 and increases per dispatched call, at most `revision`. `kind` is `recovery` or `daily`; `admitted_day` is the start timestamp's canonical UTC date. `result` is `running`, `returned` or `failed`. A running attempt has finish=0 and all three detail strings empty. A returned attempt has finish≥start, empty error code and only an existing valid settlement report state/reason pair. Failed has finish≥start, empty report fields and a closed error code: `busy`, `unavailable`, `clock`, `storage`, `closed`, `profile`, `invalid`, `cancelled`, `deadline` or `other`. Finish cannot exceed the state's clock high-water. No raw errors, TxID, transaction bytes, balances, credits, payments, reservations or fee amounts are copied into scheduler state.

Only the latest attempt is retained. An unfinished durable attempt on reopen is evidence that its result is unknown; it never implies the store call did or did not happen. The next dispatched call must be recovery. Replacing the attempt record cannot release or replace settlement reservations.

## 4. Dispatch algorithm and timing

`Run(ctx)` admits one loop per owner; it blocks until original cancellation or a fatal owner/storage error and joins all work before returning. `Status(ctx)` returns an owned copy of the bounded persisted state plus runtime running/poison status, without waiting on backend RPC or claiming fresh funding. Constructors do not start a loop. One internal dispatch gate prevents concurrent timer/control calls; no scheduler mutex or settlement gate is retained while sleeping. Existing settlement operations acquire their own gate.

1. On every start/restart, and at each retry wake, audit original binding/lifetime and sample time. Persist a `running/recovery` attempt and clock high-water before calling `RecoverPending` under a fresh bounded child context. Persist its dated result before any next action.
2. Only if recovery returned `complete` with empty reason may the same cycle consider a new daily call. Sample time again. Require now at/after today's configured UTC minute and `daily_cursor` earlier than today. Persist a distinct `running/daily` attempt, then call the original `RunDaily`. Never fall through from a recovery hold, error, cancellation or uncertain scheduler write.
3. Persist the returned report classification. Advance `daily_cursor` only for the exact daily-complete reason, using the returned report day. Refuse a daily returned report whose day is later than the sampled finish/high-water UTC date; never advance a cursor from future-dated evidence. Otherwise retry after the immutable interval; the settlement journal decides whether old work is still pending, a new batch already exists, funding is deferred or a new payment may be prepared. No successful recovery report is substituted for this decision.
4. Normal next wake is finish time plus retry interval, checked for overflow, shortened to a future configured due time if earlier. When due is already past, retain the retry interval rather than spin. Timer waits are cancellable and recheck wall time on wake. Restart may perform one immediate recovery even if the stored timer is later; this cannot create an early daily batch.

Due time authorizes **starting an attempt**, not promising a broadcast or confirmation at an exact minute. Existing `RunDaily` assigns a new batch's day at durable preparation time; a slow call admitted before midnight may occupy the next UTC day. Preserve that rule and its existing regression. Do not backdate it, launch a compensating payment or claim the scheduler can undo it. Fresh time is sampled before each later daily call, and the store still enforces the occupied day. Recovery/rebroadcast remains allowed before the next configured minute.

## 5. Failure, restart and Close

Returned pending/deferred reports retain their existing closed meanings and exact reservations; retry only through recovery-first dispatch. Busy or unavailable evidence schedules another bounded cycle under the same original owners, without retargeting pins. Context deadline/cancellation after dispatch has no financial-abandon semantics. An externally cancelled `Run` stops after joining; if cleanup prevents a completion write, retain the running attempt for restart reconciliation.

Store or scheduler poison/storage uncertainty, invalid/profile/corrupt binding, backward clock and closed original store stop the loop and retain ownership. A scheduler must not repeatedly call a poisoned handle or create a replacement store. Closed-code mapping never prints underlying backend errors or credentials. If the settlement call succeeded but the scheduler completion write failed, reopen reconciles the original settlement first; it does not trust missing scheduler completion as permission to sign again.

`Close(ctx)` prevents new calls, cancels the original loop and joins it before closing the scheduler DB/directory resources or releasing its process claim. It does not close the borrowed settlement store. If the bounded caller cannot observe a join or exact resource cleanup fails, return the closed error while retaining the exact owner/locks and first cleanup failure; never report closed success or hand that owner to a new runtime. The runtime closes settlement/backend/wallet only after scheduler join. A clean restart opens the same pins/paths and begins recovery; no persisted timer or cursor may bypass that step.

## 6. Required focused evidence

Before implementation qualification, independently freeze marker/state literal positives and corruptions for limits, clock/date/counter relationships and unresolved attempts. Use a real concrete settlement fixture to prove pre-due recovery makes no fee/sign/new-credit/new-batch call; due dispatch and deferred retry; midnight commit-day semantics; confirmed old payment plus new mature credit cannot pay early; interrupted attempt/completion publication recovers once; exact-byte lost-ack retry; changed config/foreign handle setup/read-only store refusal; backward time, overflow/capacity, cancellation and joined Close. Add the unchanged settlement/history controls from the RecoverPending plan. Tagged schedule-clock acceptance changes no node/PoW/account clock, and an untagged configured-time trigger remains required.

This bounded scheduler does not refresh round evidence itself: the standalone operator's separate concrete accounting maintenance updates the original round store. It also does not solve source/journal capacity rollover, custody, miner enrollment, public TLS deployment or key rotation. Those boundaries remain explicit in the payout-runtime and authority-profile plans. No user economic decision is required to implement this contract after review.

Implementation boundary: this slice uses the existing trusted private operator-directory model. It retains original no-follow directory descriptors and checks marker/directory identity around synchronous operations, poisoning detected replacement. LevelDB retains its ordinary pathname-based storage; this is not a hostile-local-writer filesystem sandbox.
