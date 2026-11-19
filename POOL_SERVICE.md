# Authenticated pool service and desktop credential adapter

Set 56 connects issued work, durable share acceptance and each gym's settlement
view through a separate `poolservice` protocol. It is additive: the existing
solo/manifest services, worker and desktop snapshots retain their contracts.
No new product or consensus decision is introduced.

## Trust and ownership

The server borrows one concrete `poolshare.Pool`, its matched `poolsettle.Store`
and the session authorizer. Construction compares the complete source config;
mining from one pool cannot accidentally serve another pool's balances.
The parent keeps the journals, node and account authority alive until `Run`
returns after cancellation and all handlers drain. A shared pool belongs above
individual gym clients, not inside each gym's isolated solo-mining child.

The client pins the complete source config, expected gym public key, normalized
endpoint and CA policy. `BindingID` hashes the canonical source JSON under
`SLUG pool service source\0v1\0`. `TransportTrustID` hashes the existing canonical
manifest transport identity, pool binding and public key under the distinct
`SLUG pool transport trust\0v1\0` domain. It excludes bearer tokens. These are
configured trust identifiers, not signatures or receipt import authority.
Independent Python identity fixtures freeze the byte contract.

## Miner operations

All routes are authenticated POSTs under `/pool/v1/`:

| Route | Canonical request | Response |
| --- | --- | --- |
| `issue` | `{}` | Original durable job and template |
| `work` | `job_id` | That gym's retained original job/template |
| `submit` | `job_id`, `nonce` | Share acceptance/block-delivery receipt |
| `lookup` | `job_id`, `nonce` | Existing receipt without new acceptance |
| `balance` | `{}` | Own exact balance, reason and observation timestamp |

Request bodies contain no gym, payout address, difficulty, account or signing
material. The original live session and pinned configuration determine those.
Responses carry version 1 and the exact source binding. Canonical JSON uses
the declared struct field order and encoding; outer whitespace/newline is
allowed, while duplicate/unknown fields and omitted required values fail.

Work responses export the retained template once, without a redundant full
candidate. The client reconstructs with the existing manifest builder, the
pinned pool payout and extra nonce zero. Only the header nonce may change.
Submission and lookup replies are checked against the client's independently
computed normal-PoW hash, target flags, duplicate identity and original job.
Pending acceptance sequence zero grants no credit. A pool receipt is neither
a fighter admission nor a current canonicality/finality guarantee.

An issuance error may strand an already durable job; there is no automatic
reissue or replacement birth. Ambiguous submission errors require recovery of
the same job/nonce. The client never automatically retries, follows redirects,
uses an environment proxy, persists credentials or accepts compressed bodies.

## Limits and sessions

The listener is literal-loopback only, validates the Host against its local
socket and rejects browser Origin headers. It bounds connections to 16 and
admits only one active operation without a waiting queue. Per-gym issuance,
submission and read limits survive token rotation, with at most 64 gym entries.
Request bodies are at most 1 KiB; ordinary responses are at most 64 KiB; work
responses allow the existing 16-MiB template plus 64 KiB of framing/job data.
Client/operation deadlines are thirty seconds and the listener bounds reads,
writes and shutdown. Slow response readers do not retain the operation slot.

Short session prechecks establish the gym and rate allowance, then release the
account lease before calling the pool or settlement journal, which acquire their
own original-token leases. A final same-token/same-gym check precedes response
publication. A reset after a possible mutation produces an unknown outcome,
not a false claim that nothing committed. Operator accounting pages, all-miner
reports, fee configuration, signing and `RunDaily` have no transport route.

`desktop/pilot.PoolSessionAuthority` supplies a fresh mining token between
operations while preserving the original account, generation, view, endpoint
and source. It checks the actual token used before and after HTTP; it cannot
substitute a new login during an in-flight operation. Closing cancels and drains
that call. No account/view lease is held across network I/O. The adapter exposes
typed work/share/balance methods for the subsequent worker and view integration.

## Integration boundary

The isolated normal-PoW proof uses two real fixture account enrollments and one
shared pool. Work and nonce submission traverse HTTP, a dropped winning-share
response recovers by lookup, and duplicate submission retains its original
acceptance. Sixty-descendant balances report immature funding; at consensus
maturity one transaction pays both gyms. Reset revokes the old account's work
and balance access; the other gym is unaffected and the recovered held account
can perform the viewing/mining allowed by D72.

The existing automatic worker builds a gym-owned payout and changes extra nonce;
it is intentionally not switched to pool work. A dedicated durable pool worker
must preserve pool-only shares, separate prospect-only work and block-first
fighter admission. Shared desktop runtime ownership, separate native-SegWit
recipient bindings, balance presentation and daily scheduling remain next.
No public hosting, production custody, UI reorganization or D74 release approval
is implied by this transport milestone.
