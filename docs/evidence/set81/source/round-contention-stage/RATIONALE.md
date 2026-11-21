# Selected DAILY round-maintenance contention successor

Source-only proposal. No Go/build/test/runtime execution or checkout writes. Root owns formatting, installation and all qualification. Desktop owns independent new test leaves beside this production checkpoint.

## Observed failure and source inference

Retained-admission A actual attempt 1 failed its existing 90-second initial drain-readiness wait after two accepted canonical pool receipts. The public report preserves three original operator stdout frames with public readiness true, pool-only draining at submissions 2, and round code busy with a zero displayed summary. Original CLI/operator/node owners joined. This remains a failed actual attempt; no acceptance predicate or timeout is weakened.

`roundMaintenance.step` returns a zero summary on failed Sync, but preserves the successful Sync summary if later refresh fails. Therefore each sampled zero/Busy frame identifies a failed Sync call. It does not prove the durable journal stayed empty or that every intervening call failed. The dated public frames precede the round frames by 3–5 ms; capacity transitions finish 14–27 ms later.

Concrete possible contention paths: original round Store.begin is nonblocking; Sync calls original Pool.AccountingPage, whose pool gate is nonblocking; each saved work checks original TryGetManifestJob, whose issuer gate is nonblocking. Exact issuer Busy maps to pool Busy and then exact round Busy. Public maintenance uses the same issuer, and its subsequent daily intake Status holds the pool gate for the complete dailyCensus. Both joined loops then updateIntake and sleep the same selected 1000 ms interval. Repeated admission collisions or sampling phase alignment are supported source explanations, not an instrumented identification of which gate caused A1. No cyclic blocking dependency was found along these nonblocking Sync paths.

## Minimal behavior change

Four production leaves: new private dailyRoundCall plus narrow selected dispatch in existing maintenance/live_round/runtime. Runtime passes its immutable daily flag. Legacy wrappers keep the one-call behavior and original context.

Selected calls retry ONLY the exact poolround.ErrBusy sentinel, after a cancellable 10 ms pause with no concrete gate or lifecycle mutex held. Sync, each List, each individual Refresh, and an already-selected Live handle's permitted BeginLive reanchor have separate retry boundaries. Successful earlier phases are not repeated. Existing one-reanchor rule and cursor advancement remain. Refresh may repeat a normal node observation after its post-observation source check returned Busy; it does not quote, sign, import ledger credits or relay payment. Full atomic Sync and financial audits are untouched.

The original roundLoop 30-second child remains authoritative. Each selected private entry adds a maximum 30-second child, so Background callers are finite too; nested children inherit the earliest deadline and no retry creates a new budget. The same bounded context reaches the concrete operation. Cancellation, deadline, wrapped/joined Busy, unknown, node/source-unavailable, corruption, poison and write uncertainty are not retried here. Existing outer classifications remain unchanged.

No schema/profile/capacity/economics/authority/cadence/harness deadline change. No owner replacement or persisted verification cache.

## Independent regression requirements

- Deterministically hold the original issuer gate, invoke original Sync, observe its real Busy result, release the original holder, and require the same selected invocation to reconcile the complete source tail. Compare the unchanged original one-call behavior while held; do not present a compile failure as a pre-fix reproduction.
- Exercise full selected step and already-selected Live refresh against original source/round owners; earlier successful phases remain completed and retained payout bytes/backend counts unchanged.
- Cancel/deadline while the real gate remains held: join without advancing source/round/financial state. Release/close the holder exactly once and retain cleanup errors.
- Pure controls: exact Busy retry, wrapped/joined/mixed/fatal/cancel refusal, V1 original context/one call, fixed deadline across attempts, Background finite bound.

A passing controlled test demonstrates retry behavior under the concrete gate; a new unchanged A/B actual run remains necessary for the real operator/TLS/worker acceptance claim.
