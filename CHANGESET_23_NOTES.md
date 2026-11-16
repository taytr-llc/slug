# Change set 23 — wallet fork foundation

Status: paired RPC implementation and local chain checks complete.

The paired chain branch `slug-wallet-foundation` is stacked on desktop
implementation `c0923eca`. Wallet source is separate at
`https://github.com/taytr-llc/slugwallet`, branch `slug-wallet-foundation`, based
on btcwallet v0.18.1 (`43f2950d2a065a7b3d4a4a1cf17ed1e84af6bcb4`). Its own
change-set notes record its implementation and test evidence separately.

The initial scope is consistent SLUG module identities, the slugd full-node
RPC backend, network/genesis validation, isolated fixture wallet lifecycle and
independent public address/spend/recovery checks. The paired chain change adds
a context-aware RPC connection API to make wallet startup cancellation cover
the actual dial/handshake and retries. Genesis, consensus, seed exposure and
the desktop fixture signer are unchanged.

D48 hosted login, recoverable phrase export, account recovery and gym-key
derivation are still integration work. This set provisions no real wallet or
hosted account, reads no real key and deploys no wallet service to the seed.

## RPC connection lifecycle

`ConnectContext` adds cancellation across connection/state lock acquisition,
direct and SOCKS dialing, TLS and HTTP upgrade handshakes, and retry waits.
An explicit cancellation callback closes a stalled upgrade socket because the
pinned WebSocket dependency only applies its deadline to that read. The callback
is stopped or joined before a successful transport is handed off. Cancellation
after successful publication does not close the established client; callers use
Shutdown or Disconnect for that lifetime.

Existing `Connect` delegates with a background context. New-client and automatic
reconnect paths retain their existing behavior. Shutdown aborts an active initial
attempt, and interrupted initial Disconnect can be followed by a clean retry.
Context-aware locks and atomic cancellation cleanup avoid a canceled caller
waiting on another operation or leaving a blocked helper goroutine.

## Verification

- Full RPC client race suite passed in 5.052 s; vet and formatting checks pass.
  Regressions cover actual stalled dial/HTTP upgrade/SOCKS, deadlines and early
  cancellation, retry waits, contended locks, shutdown, successful handoff,
  cancellation cleanup under contention and initial Disconnect followed by retry.
- All ten module builds and Linux amd64 node/command builds passed.
- Required normal-PoW chain smoke passed: two local 10 SLUG rewards, report-only
  conflict without chain veto, public-seed synchronization to height 61,
  checkpoint finalization, verification and restart. Existing context, gym,
  birth, service and payable-proposal checks passed, including overpayment
  rejection. No candidate was broadcast to the public seed.
- The paired wallet independently matched four fixture addresses and completed
  receive/maturity/spend/reopen/raw-seed-restore/spend on a private 102-block
  normal-PoW chain. Its own notes and smoke report record final runs after wiring
  this connection API. Raw-seed fixture restoration does not satisfy D48's
  user-facing export requirement.

Deployment observation is recorded after promotion. Existing chain CI debt is
unchanged; local passing checks do not imply green remote CI.
