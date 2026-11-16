# ADR-001: Keep the first prospect authority local and bounded

**Status:** Accepted for set 19 implementation; no public rollout authorized
**Date:** November 16, 2026
**Deciders:** Codex implementation judgment under Mike's continuation request;
Mike's existing D44 and D68 decisions govern signing and birth

## Context

Sets 17/18 provide gym signatures, five-minute pre-work jobs and immutable
issuance-based birth. Opening a registry per command repeatedly scans structure
and takes an exclusive database lock. Production Argon2id costs 256 MiB per
hash; jobs and receipts consume disk even after expiration. A transport must
not turn cheap unauthenticated requests into unbounded work or durable writes.

## Decision

Keep one existing schema-3 store open in a loopback-only daemon. Expose only
job issuance and signed submission, with fixed operator policy and trusted node.
Bind API credentials to gym keys before expensive work. Process one operation
at a time without a queue; bound connections, bytes, rates, time and persistent
counts. Preserve expiry, immutable receipts and uncertain-write semantics.

Add a validating client and public-input CLI transport. In-memory signing is
a callback, so transport never owns wallet keys. Use existing formats without
consensus or product-rule changes. Exact contract: `PROSPECT_SERVICE.md`.

## Options considered

| Option | Complexity | Resource behavior | Tradeoff |
| --- | --- | --- | --- |
| Per-command file/DB handoff | Low | Repeated open/scan, lock conflicts | Useful diagnostics; poor running-client boundary |
| Local bounded HTTP authority | Moderate | One operation; durable capacities | Testable transport with limited throughput |
| Public multitenant service now | High | Needs enrollment, hosting, abuse controls and TLS | Premature before wallet/client and target calibration |

## Trade-off analysis

One active operation bounds memory and contention at the cost of throughput.
A busy response avoids accumulating work that can expire in a queue. Rate
limits smooth traffic; durable capacity limits bound growth across restarts.
Loopback reduces exposure while integration is exercised. Verified HTTPS in
the client enables later separately reviewed TLS deployment.

## Consequences

- Jobs and receipts work across processes, with exact replay after uncertain
  acknowledgments.
- Operators manage the authority database and capacity. No automatic pruning,
  rotation, repair or rollback is introduced.
- Public hosting, wallet/login/export, mining automation and full block
  templates remain separate. A block win changes the parent and needs explicit
  prospect admission ordering in the miner integration.
- API credentials do not establish account identity or replace signatures.
  Authority time and durable history remain trusted under D68.

## Action items

1. Implement daemon, validating client and CLI transport.
2. Verify adversarial transport, persistence/retry and normal-node integration.
3. Carry complete templates and block-win ordering into the mining slice;
   do not present a synthetic coinbase scanner as a complete miner.
