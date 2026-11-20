# Independent Set73 carrier producer — unexecuted source

The producer has not been imported or run. Root must review/freeze exact candidate
contract, literal inputs and source hashes, then supply a root-approved execution
lock. AST parsing is a syntax check, not vector generation or mathematical proof.

`input-catalog.json` was written before producer design. Its copied files include
only the old published Set24 fixtures, audited independent recovery/RFC6979 Python
sources, old withdrawal transaction model as provenance, D83 policy, a candidate
carrier-contract snapshot and explicit modeled recipe. The complete original
paths, sizes and hashes are retained. The recovery module reads its copied English
wordlist; the ECDSA module's unrelated fixture generator is never invoked. Neither
module imports Go. The new model consumes their independent derivation/arithmetic
functions. Only the two named published fixture seeds may enter the model; no real
key, private database, export file, RPC or network is read. Derived private scalars
remain model-local and are not included in outputs.

`carrier_model.py` independently builds the fixed intent, compact gym signature,
redeem/locators, full SegWit commit and legacy reveal/refund, legacy and BIP143
SIGHASH_ALL preimages and txids. Its bounded parser/checker crosschecks semantic
and cryptographic relationships. Its transaction parser is not a full consensus
engine. Actual prevout/canonical-block existence and finality are deliberately not
asserted: full modeled parent transactions and PinID/StateID are explicitly literal/synthetic. Parent stripped TxIDs and selected outputs are verified. A parent with an unrelated10,001-byte output script is accepted within the4,000,000-byte containing bound; the10,000-byte limit belongs to executing carrier scripts. No parent dummy-input signature or chain-existence claim is made. The
public recognizer's need for complete actual predecessor and chain evidence is not
waived by these vectors.

The twelve positive cases cover both gyms,1/16 inputs, refund, three outer
representations, third-party payer, a different competing input tuple and a valid
non-RFC gym signature. That last case preserves public semantic validity but
explicitly denies deterministic writer reconstruction. Thirty-one negative cases
exercise exact templates and signatures; semantic mutations are re-signed at the
funding layer to reach their intended boundary. Two missing/wrong-parent cases expect unavailable, not invalid. Four extra tuple refusals isolate
count/order/duplicate framing. The generic520-byte boundary is the separate probe,
not falsely represented as the fixed240-byte contract redeem.

The generator's default path performs no model import. Execution flags:

```
python3 -B generate_reference.py --execute --input-root <locked-inputs> \
  --input-catalog <locked-catalog> --approved-producer-lock <root-lock> \
  --approved-lock-sha256 <sha256> --output <fresh-exclusive-directory>
```

Lock shape is `{approved_by:"root",execution_authorized:true,
input_catalog_sha256:<exact>,sources:{<each exact owned filename>:<sha256>}}`.
The exact source set is carrier_model.py,generate_reference.py,case-plan.json,
README.md. Guards check input names/size/hash and source hashes before importing
any model, then repeat them after generation. Output directory is exclusive and
Python bytecode generation is disabled. A failed run remains in that fresh
directory; no attempt is reused. Cross-Python and portable reproduction are
required after first authorized execution, not claimed by preparing this source.
