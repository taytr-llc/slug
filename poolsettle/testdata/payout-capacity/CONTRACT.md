# D84 transaction capacity prerequisite

Whole ready funding is selected before testing transaction capacity. More than 256 ready inputs defers the whole new batch with the explicit `transaction-capacity` reason, distinct from journal capacity. Never take the first 256, split payments, consume a new daily slot, sign or broadcast a subset. Existing exact pending-transaction recovery precedes this new-batch gate and remains unchanged.

Private API: `transactionCapacity(inputs, payments int, change bool) bool`. Inputs and payments must each be in 1..256. Native84 funding spends have stripped input size 41 bytes and native22-byte-script outputs have size31. With `o = payments + (change ? 1 : 0)`, the conservative full signed length is:

`4 + CompactSize(inputs) + 41*inputs + CompactSize(o) + 31*o + 4 + 2 + 109*inputs`.

The last terms are witness marker/flag and, per input, stack count1 + signature length1 + signature73 + public-key length1 + compressed public key33. This is **raw bytes**, not weight or virtual size. The independent producer constructs symbolic serialized bytes; no signatures, keys, wallet state or Go outputs are used. No change yields one fewer output, not an omitted witness marker. Reject invalid counts before machine-int multiplication/addition.

For inputs256/payments256/change, the full maximum is 46,383 bytes; virtual size is25,454. Therefore the100,000-byte refusal branch is unreachable while the existing256/256 count limits remain. Mathematical rows near100,000 in the corpus are deliberately outside the valid input-count domain; they are serialization controls, not possible V1 payouts. V1 also limits historical credits to256. No257-credit end-to-end state is fabricated by this slice, and this is not a continuation qualification.

`inspect` may examine more than256 retained candidates using consecutive backend pages of at most256 **only to determine funding readiness**. It returns every ready coin in original order and aggregates existing reasons, including funding-conflict precedence across pages. Any backend error, wrong result length/order/outpoint, unknown state or cancellation discards all partial ready/reason outputs. It performs no quote, sign, broadcast, persistent write or claim mutation; backend inputs are deep-owned copies. A successful inspection of513 ready coins returns513, then the separate transaction gate refuses the whole set.

Focused draft coverage: independent count/CompactSize/raw-byte boundaries and invalid integer counts;513 candidates across256/256/1; >256 candidates with only the last ready coin; reason precedence across pages; exact later-page backend and malformed-result refusal; cancellation before/between/after pages; original argument/state immutability; no quote/sign/broadcast. These inspect tests use explicitly modeled coins and a controlled funding observer only, not authenticated credit or normal-PoW claims.

Parent owns RunDaily wiring/review and executable qualification. Drafts here have not been compiled or run with Go. The whole-batch contract is not proven solely by private arithmetic/inspection tests; source wiring must place the gate before unsigned construction/signing and preserve the complete ready set and old pending recovery.
