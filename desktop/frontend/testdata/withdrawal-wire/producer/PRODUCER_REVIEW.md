# Finite independent wire-producer review

Read-only review on November 19, 2026, before expected-output execution. No production output or new golden was used. Source copies at the end of the review are retained in `wire-review-snapshot/`; root was editing the unfrozen proposal during review, so these hashes bind that captured revision, not a claim that every earlier finding remains in this revision.

- `model.py`: `191bb1966035f4a2bb4310b7bc20b9535f8b073646110f07db8db8b14b35f246`
- `generate.py`: `1b15b01a070aa559a7f564c078705e11b4e3f68afbbffac1ef91665ddeb1c893`

Reviewed against frozen desktop-v1.md and journal-v1.md, the existing independently generated journal input, original ready marker, and inherited transport-view grammar.

The canonical compact intent/draft projections, mapped amount/fee field names, dated preparation metadata, non-witness double-SHA256 transaction ID, binding/intent/draft domain and length prefixes, and unchanged exact signed-byte hash are consistent. Rebasing fixture account/source metadata changes the metadata digests without changing transaction bytes; these remain prospective/modeled wire cases and do not establish a new signed journal or node event. The HTTP transport trust preimage uses the origin length, loopback marker and empty-CA length correctly. Current exact authority/source values come from the frozen actual ready input.

Pre-freeze recommendations sent to root:

1. Split lexical lowercase64hex from required nonzero native IDs. Desktop nonzero IDs are OperationID, ActionID, ReviewID, journal/source. Inherited ViewID and transaction/digest fields must not acquire an unstated nonzero restriction; stored dispatch authorization ViewID is independently nonzero under the journal contract.
2. Require fee_atoms >=1 in the compact draft and add a zero-fee negative, preserving the inherited wallet draft rule.
3. A retry action in review must carry an already signed operation; a send review carries prepared. The first inspected generator selected prepared for every review state. Root reported the generator selection already corrected during review; the model should enforce this relation so it cannot bless a recurrence.

No other invalid current positive was found in this finite read. Existing negatives cover changed binding/source/generation, digest/amount arithmetic, frame and selector violations, confirmation framing, and error/payload isolation. This is a finite contract/corpus review, not an exhaustive parser proof or a claim that the limited known-address oracle is a general address validator.
