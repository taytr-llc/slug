# Independent Set73 public ownership reference

This producer is source-only until the root reviews and freezes its exact input and producer locks. Do not import `core_model.py` or execute the generator to obtain expectations before that approval. Running without `--execute` exits before model import; that convenience is not a substitute for the lock.

The 24 inherited/cataloged public inputs preserve original receipt sequences, source pins, descriptor identities and archived byte provenance. The adopted contract is added as a separate catalog input before execution. The six new admission statements are synthetic signatures around previously qualified actual public records. They do not assert that the old authorities actually emitted these new statements. Only the two original sequence-one records participate in complete modeled prefixes. Higher-sequence wrappers deliberately do not invent earlier admissions.

`inputs/inherited/crypto.py` is an unchanged copy of the independently audited Set45 pure-Python primitive implementation. Its published RFC8032 and secp selfchecks run before generation. `seed-common.py`, `seednode-attest.go` and `signedcheckpoints.go` are source provenance only, never imported. The new public log key uses the root-selected public seed of 32 repeated 0x73 bytes. The checkpoint key separately uses the inherited seed-checkpoint fixture domain. These are public fixtures, not deployment/operator keys.

`core_model.py` defines exact canonical codecs and statement/domain calculations. `generate_reference.py` composes one portable corpus from the locked inputs and explicit modeled facts. It never invokes Go, a node, Argon2 or a new product implementation. Each positive includes its canonical bytes and independent digest. Conditional replay rows state the facts assumed; modeled hashes never pretend to prove normal work or prior canonicality. The key/value fixture exercises codec/accounting only and cannot qualify a complete production Open. The grammar size cases deliberately compose maxima and are not a heap measurement. Actual runtime proof, reorg, fences, atomic writes and cleanup remain Go/normal-node qualification.

The frozen source set is exactly `core_model.py`, `generate_reference.py`, `case-plan.json`, `README.md`. The proposed lock contains `approved_by`, `execution_authorized`, `input_catalog_sha256`, and a `sources` filename-to-SHA256 map. The runner verifies the exact lock SHA, root authorization, every input size/hash, exact input membership and source bytes before importing either model or crypto. It checks all inputs/sources again before writing an exclusive output. No output can overwrite an earlier attempt. Preserve failures and predecessor locks before any approved correction.

After root authorization, the bounded invocation is:

```
python3 generate_reference.py --execute \
  --input-root inputs --input-catalog input-catalog.json \
  --approved-producer-lock /absolute/path/to/approved-lock.json \
  --approved-lock-sha256 ROOT_REVIEWED_SHA256 \
  --output /absolute/path/to/new-attempt
```

Use the same lock and copies for Python 3.14 and 3.9. Compare the complete `public-core-v1.json` bytes. The corpus is limited to 16 MiB and a single finite case plan; an external bounded runner should terminate/join its own process group on timeout. Nothing in this source grants chain, account, signing or publication authority.

Cutoff finality-prefix successor (source-only pending root freeze): the exact
unsigned finality outbox prefix is captured alongside admission/publication
prefixes. Later frames cannot enter that cutoff, even if they carry an older
height and the same publication count. These additional controls model prefix
selection only, not normal work, canonicality, or a new finality policy. Exact
predecessor source/inputs/lock/corpus are preserved in the sibling directory
`public-core-pre-cutoff-finality-prefix`. No successor expected output has run.
