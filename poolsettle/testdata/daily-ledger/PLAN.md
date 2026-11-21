# Daily indexed ledger independent reference

This producer is frozen before execution and before daily storage implementation.
It imports no product code and reads only the four selected public input files.
The nine signed transactions are the previously independently generated public
wallet fixtures; their raw bytes and transaction IDs are retained unchanged.
Conditional claims are explicitly not mined round or funding authority.

The output contains 18 prepared/confirmed transaction cases, one empty root,
12 closed-index/claim-commitment refusals, and a precise upper-byte calculation.
Index mutations recompute logical byte accounting so that missing or misdirected
indexes cannot be rejected merely because their old byte total was stale.
The change-digest case is an explicit financial commitment mismatch; the other
negative cases are rejected by the independent inverse-index model.

The budget uses the complete lifetime source bounds N,R<=16384, B<=32767,
G<=256, original allocation leaves<=N+R, and the single-change lineage bounds
total consumed inputs<=R+B and total payments<=G*B. It enumerates canonical
field/key lengths, hex-expanded signed transaction bytes, all inverse indexes,
each delivery-state reserve, and fixed head/report reserves. It separately
bounds a credit, a batch body, and the streaming active request. The upper-bound
strings need not be valid reduced fractions: they deliberately overestimate the
closed grammar. No expected value is obtained from the Go implementation.

Run the exact source with two available Python versions into two fresh output
paths. Each run must exit normally within 30 seconds, stay below the 8MiB output
limit, and leave source/input hashes unchanged. Require byte-for-byte output
equality and freeze output SHA/length before Go tests consume it. Preserve any
failed run without overwriting its output. This is a finite grammar/arithmetic
qualification, not a daily financial or actual-node acceptance result.

Root separately selected the source-owner pure CheckNonce memo prerequisite.
This producer does not grant authority to that cache or to any ledger DTO.
