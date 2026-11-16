# ADR-002: Validate complete payable candidates before automatic mining

**Status:** Accepted for set 20 implementation
**Date:** November 16, 2026
**Deciders:** Codex implementation judgment under Mike's request to start
set 20; existing D44/D68 product rules remain unchanged

## Context

The authority can issue jobs and accept signed proofs, but its context-only
jobs omit payout/coinbase values, full transactions and witness commitments.
A working desktop miner needs those fields to produce payable, valid blocks.
The node's existing GBT proposal mode can check a complete candidate against
the current UTXO set without broadcasting or requiring a winning nonce.

## Decision

Build the complete candidate boundary first: one consistent full template
snapshot, strict transaction and arithmetic checks, an explicit local payout,
the D68 commitment, full witness handling and a matching stripped proof. Add a
bounded nonbroadcast proposal check and public-input CLI. Do not enlarge the
authority transport or introduce wallet custody, automatic scanning, submission
ordering or new prospect eligibility rules in this slice.

## Options considered

| Option | Complexity | Benefit | Limitation |
| --- | --- | --- | --- |
| Synthetic coinbase-only share scanner | Low | Exercises hashes and admission | Cannot serve as a full payable miner |
| Complete candidate and proposal boundary | Moderate | Tests real block validity and payout before automation | Worker and wallet integration remain separate |
| Full automatic miner immediately | High | End-to-end behavior | Combines template correctness, key custody, renewal, retries and block-win ordering |

## Tradeoffs and consequences

The chosen boundary keeps failures diagnosable and can be exercised against a
normal node without changing the public chain. It preserves every template
transaction and fails when the added commitment exceeds available space.
Transaction fees and some signature-operation costs still depend on the trusted
node's UTXO assertions, which proposal validation checks. A successful proposal
is a point-in-time node observation, not PoW qualification or chain acceptance.

Automatic mining still needs an explicit block-win ordering contract: node
broadcast and authority admission cannot be atomic under their present APIs.
Full remote templates also need a separate transport size/version contract.
Neither issue changes D44's automatic signing or D68's immutable birth/expiry.

## Action items

1. Implement full template capture, candidate construction and proposal CLI.
2. Check independent witness/nonwitness vectors and normal-node proposals.
3. Follow with the automatic worker and desktop pilot integration, including
   the wallet/login and tested export requirements already settled in D48.
