# Change set 74 — public pool runtime and scheduled payouts

Branch `slug-public-pool-runtime`, stacked from completed Set73 maintenance
`1aba70ca36dfbd8e492f64b33c8d329f682de56c`. The live seed remains release
`e3eb4d495455383121bc1138f2cc010c90c6fb72`. This set is in progress; no public
pool deployment or scheduled-payout acceptance is claimed.

The work order is public mining service and scheduled payouts, then sign-in and
onboarding. D74 remains the final joint product review. Preserve D58/D79/D80/D81:
per-block difficulty-weighted rounds, confirmation at 60 descendants, payment
only after coinbase maturity, one daily batch, 1 SLUG minimum, exact carry,
network fees from the pool's 1% only, and exact-byte recovery without advances.

## Completed component: recover without preparing a new payment

`poolsettle.Store.RecoverPending` uses the original transaction-recovery prefix
but returns before importing new credits, selecting funding, quoting a fee,
signing or occupying a new daily slot. It can observe or resend only the exact
already-retained transaction. Successful `complete` with no reason describes
recovery, not today's payment; uninspected available funding is conservatively
zero. Existing RunDaily behavior and the authoritative Batch.Day remain intact.

`SchedulerBinding` audits the original writable settlement and returns owned
configuration copies. It refuses read-only handles and performs no backend
operation or financial mutation. Its logical configuration digest is not a
unique database identity; the runtime must retain the original paths/owners.

Qualification: `artifacts/set74/settlement-attempt1/report.json` passed with
3,792 source/reference inputs unchanged under the existing Go 1.27.4 no-CSE
candidate profile. Focused normal tests passed in 14.814 seconds, race tests in
77.154 seconds, cheap-PoW refusal tests in 2.922 seconds and vet in 0.445 seconds.
Every owned process exited cleanly. Coverage includes no pre-due preparation,
lost-ack exact-byte retry, next-day new-credit separation, unresolved/conflicted
funding, cancellation, capacity, uncertain writes/reopen, original-owner binding
and the existing transaction/maturity/history controls. No node or public chain
was mined by these library checks. This is not full service or wallet acceptance.

## Remaining integration

- Explicit operator public-authority profile and concrete key owner; legacy
  fixture profiles remain closed and unchanged. The official checkpoint key
  stays offline; signed checkpoint import verifies public evidence only.
- Independently running public pool with authenticated TLS miner transport,
  immutable enrollment/payees and joined operator lifetime.
- Concrete guarded wallet settlement backend and persisted recovery-first UTC
  scheduling, followed by actual payout, restart and lost-reply acceptance.
- Capacity continuity or an explicit finite-run limit, then remote-host rehearsal.

No real key or seed has been read or created. Existing fixture applications,
production genesis/custody gates and final native/design acceptance remain
separate. Operator host/custody configuration will be made concrete before any
required deployment input is requested.
