# Set 39 worker reservation correction

This engineering addendum changes only the capacity accounting in the approved service/worker contract, SHA-256 `c25e2fb7f7361a0207acea952c44d62e7fcc9a7ebeeba5887fb2cf3d2b9ebfdf`. The original contract and independent service corpus remain unchanged. Root approved this formula after independent review. It changes no public DTO, journal schema, service protocol, target, admission rule or old worker implementation.

The initial fixed-reservation copy could reject a required transition at exact saturation: changing `pending` to `submitted` grows the entry while charging the same unused 512-byte state allowance. Authorization growth had the same problem. Reserving unused growth must consume that allowance when bytes enter the durable record.

For an unresolved entry, let `S` be the exact canonical JSON byte contribution of `block_state`, `admission_state`, optional `last_error`, `next_attempt_ms`, and `retry_count`, including field names, punctuation and the leading comma. Equivalently, encode the bounded object containing exactly those fields in that order and subtract one byte from its length. Its maximum budget is 512 bytes.

Let `Amax = 4096 + len(,"authorization":)` and `Rmax = 73728 + len(,"receipt":)`, where the lengths include the literal JSON key quotes and leading comma. Let `Aused` be zero if authorization is absent; otherwise it is the exact canonical authorization body size plus the authorization field prefix size. The remaining reservation is:

- Any fully terminal entry: zero.
- Admission still nonterminal: `512 - S + Amax + Rmax - Aused`.
- Admission terminal or not applicable, block unresolved: `512 - S`.

The known state/error grammar and bounded counters keep `S <= 512`. Authorization and receipt body limits are unchanged. Rejected bounds fail closed; arithmetic cannot wrap to an acceptable capacity. A saved authorization's bytes are immutable. An accepted receipt's bytes become immutable and its future budget is released, even when block reconciliation remains unresolved.

Consequently, `Bytes + Reserved` stays constant as nonterminal scheduling/state or authorization bytes grow. Acceptance never increases that total: its response fits the fully reserved response field. Block-only evidence retains usable state-growth capacity until reconciliation completes. No future block or receipt expansion is charged twice. Immutable job/candidate bytes stay in the actual-byte charge; only a small bounded mutable projection is encoded for remaining reserve, never full block JSON per hash. The existing cached maximum-width new-entry estimate remains conservative before a hash begins.

Audit recomputes remaining reservations from canonical retained entries. All index, cursor, counter, terminal immutability, uncertain-write poisoning and capacity-increase rules remain unchanged. This is an unreleased new-version accounting correction, with no old-format migration or compatibility fallback.

Focused acceptance covers exact saturation for combined block/prospect and block-only entries; authorization, accepted receipt and unresolved block transitions; maximum counter/error/wrapper encodings; terminal release; full JSON page byte limits; uncertain writes; and restart after real process termination. Public fixtures and behavioral storage cases do not authenticate arbitrary journal content or claim portable receipt provenance.
