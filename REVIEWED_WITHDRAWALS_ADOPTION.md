# Set68 contract adoption and provenance

The contracts hashed in `reviewed-withdrawal-contracts.json` are the adopted engineering boundaries for the candidate fixture withdrawal implementation. Historical “proposal”/“not frozen” headings remain byte-for-byte in the original reviewed documents so their freeze hashes and independent inputs remain reproducible. They describe their original drafting status, not the current implementation authorization. The chain and separate wallet repositories must be built together on their Set68 topics.

The independent wallet and journal producers were frozen before their respective Go implementations. The desktop wire producer was derived separately from frozen contracts, existing public account/source/fixture inputs and the independent journal corpus; it never imported new application output. Separate review preceded its execution. Both Python3.9 and3.14 reproduced each corpus, and portable launchers reproduce checked-in bytes without replacing expectations.

Small additive contracts make paired setup presence/emptiness and full wallet inventory explicit, bind opaque tokens to the exact live journal pointer, and register saved change on the retained wallet notification connection. The RPC erratum preserves the incorrect original boolean shorthand and corrects it to the inherited node's integer0/1 verbose parameter. Native recovery accepts a missing operation selector only from the same native-retained action/helper tuple. None changes fees, transaction bytes, account/reset authority, checkpoint finality, or production chain choices.

Qualification is recorded in `CHANGESET_68_NOTES.md`; this contract index alone is not completion, packaging, deployment or product-release evidence. D74 remains the final joint product review.
