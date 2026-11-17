# Independent static fighter manifest candidate

This independent corpus was prepared before the Go implementation and is now
committed alongside its conformance tests. It contains no official generator
activation, ownership or birth assignment, renderer, fight engine, marketplace
or private key.

The exact contract is `../../FIGHTER_MANIFEST.md`. The workspace handoff review
is `../../../sim-handoff-review.md`; the unmodified source archive is retained
outside this repository.

Run with Python's standard library:

```sh
python3 -B reference_manifest.py --check
python3 -B reference_manifest.py --generate
python3 -B reference_manifest.py --seed 0000000000000000000000000000000000000000000000000000000000000000
```

`--generate` writes only to stdout. The default is `--check`, which regenerates
the whole corpus and compares it with `manifests_v1.json`, rather than merely
checking hashes. `--seed` prints canonical compact preview JSON plus a console
newline; that console newline is not part of canonical manifest bytes. The
strict parser deliberately rejects that newline if supplied as manifest input.

`baseline/` contains the original independent DNA v1 Python oracle and its
goldens, copied byte-for-byte. Every run checks their pinned file hashes before
importing and verifies the original DNA outputs before generating expectations.
The new reference never imports or executes production Go. Expected values
were derived only from the written candidate contract and this independent
baseline. The source-table provenance and normalization are in `provenance.json`.
The extracted simulation originals have not been edited.

`tables_v1.json` is the frozen canonical table byte sequence with no final
newline. Its domain-separated table hash is
`72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44`.
Names and portrait tables originate in the handoff; distributions, integer
arithmetic and physical style predicates are new §17 engineering candidate
tuning. They do not claim byte compatibility with Python `random.Random`.

The corpus includes:

- Original DNA seeds and the two known colliding prototype short-ID seeds.
  No short ID is used by this candidate.
- Greedily selected lowest integer seeds covering all name choices, portrait
  choices, classes, styles and rarities, with selection rules recorded.
- Exact manifest objects, canonical strings and separate domain-separated
  digests. The digest is never an ownership or fighter identifier.
- Literal stream messages and SHA256 values at counters 0, 1, 256 and uint64
  maximum. Wide integers in stream transcripts are decimal strings to prevent
  accidental JavaScript/JSON float truncation.
- Rejection, last-permitted-draw and deterministic exhaustion checks; all 141
  weight-rank intervals; class, height, style and precedence boundaries.
- Strict-parser failures including duplicate/escaped/case aliases, wrong
  types, nonminimal numbers, byte/depth limits and attempted admission fields.
- A fixed 4096-seed mix for tuning diagnostics. It is neither a probability
  assertion nor fight-engine or economic calibration.

The current manifest has no caller-defined table, style, SVG or image URL.
Trait indices are deterministic; a bounded renderer can be specified later.
The portrait colors/geometry retained in the tables are provenance for that
work, not a silently accepted rendering byte contract.

The Go conformance tests consume these independently derived fixtures without
replacing them with implementation outputs. Official issuance
still needs a version-bound policy/job contract; existing DNA v1 receipts are
not retroactively assigned this candidate generator. D74's joint design review
with Mike remains required before product release.
