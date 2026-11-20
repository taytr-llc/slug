# Public pool payout runtime components

Set74 is in progress. These components preserve D58/D79/D80/D81: fixed per-block rounds, confirmed
earnings at sixty descendants, spendable coinbase funding only, a daily batch,
one-SLUG minimum, fractional carry and fees paid from the pool's one-percent cut.

## Scheduler

`poolschedule.Create/Open` retain one original concrete `poolsettle.Store` and
independent expected configuration: runtime/scheduler identities, full settlement
binding, UTC minute of day and bounded retry interval. The binding hashes logical
configuration, not a physical database. The owning runtime must retain original
paths and handles. There is no callback that can substitute another ledger.

The scheduler persists each admitted attempt before calling settlement. It first
calls `RecoverPending`; only a completed recovery with no reason can lead to a
due `RunDaily`. It never prepares a new batch before the UTC due minute, catches
up a series of missed calendar days, or infers success from elapsed time. The
batch's authoritative day controls the cursor even if preparation crosses
midnight. Future days, backward clocks and exhausted counters refuse operation.

Persisted status describes the last observation, not fresh spendability. Deferral
reasons remain visible. A completion-write uncertainty requires reopen and
recovery of the original settlement. Close cancels and joins work before storage
release; it does not close the borrowed settlement store. Operator storage must
be private/trusted: pathname-based LevelDB I/O is not a hostile-local-writer
boundary.

## Node connection

`poolsettlenode.New` pins current Phase A identity, one compressed pool public
key and matching native P2WPKH funding/change script. It supplies `Inspect`,
`EstimateFee`, `ObserveTransaction` and `Broadcast` with bounded operations and
joined lifetime cancellation. It has no signing method, key getter, balance
import or coin selection. Transport is authenticated HTTPS with selected trust,
or literal-loopback HTTP; no proxy, redirect or hidden retry is permitted.

Funding inspection checks exact canonical raw transactions, UTXOs, pool scripts,
amounts, mempool spends, next-block coinbase maturity and before/after chain tip.
Change retains every original round's funding provenance. If that original block
is orphaned, canonical change does not make the round payable. Only the node's
specific indexed-transaction absence response proves absence; unavailable or
malformed evidence defers recovery. Critical RPC members must be present even
when their typed zero value could otherwise match.

Fee policy explicitly selects `node` or `fixed`. Rates are atoms per 1,000
virtual bytes. Node decimal estimates use exact rational parsing and upward
rounding; no floating-point amount arithmetic is used. The quote includes every
specified payment, possible change, CompactSize transitions and a conservative
maximum native SegWit signature. Exceeding rate/fee caps defers; it never clips
the quote, surcharges miners or silently substitutes a fixed rate.

Broadcast checks canonical supported transaction bytes and the pool witness
shape, sends once and verifies the returned transaction ID. The original
settlement journal, not this client, proves which signed transaction is authorized
and owns reservations/exact-byte retry. An uncertain response cannot create a
replacement payment.

## Public transport

`poolingress` exposes only the six existing pool POST routes, admission/recovery
and public ownership V2 through a selected TLS 1.3 origin/certificate. Private
services retain authentication and authority. The ingress admits bounded bodies,
responses and concurrency, discards forwarded identity, and never retries a
request after uncertain delivery. Shutdown joins its handlers before the owning
runtime releases those private services. Miner enrollment and wallet integration
remain separate from this transport component.

## Miner enrollment

`poolminer` retains fixed public gym keys and native payout scripts matching the
original settlement configuration. Separate pool/admission credentials expire,
rotate by exact generation and revoke durably; tokens are stored only as scoped
hashes and cannot be reused across audiences. Local operator actions never
change a payout destination. Original session rechecks remain active only within
their admitted callback, including cancellation and joined shutdown. This does
not implement the deferred hosted sign-in or onboarding flow.

## Remaining acceptance

The dedicated encrypted wallet, immediate signing authority and public TLS
runtime are connected; see `POOL_OPERATOR_RUNTIME.md`. Actual funded scheduler
payout/restart and remote service acceptance remain. Component checks use
independently derived public references and synthetic node responses; they are
not evidence that a public service or real wallet has paid anyone.
Qualification and preserved failures are recorded in `CHANGESET_74_NOTES.md`.
