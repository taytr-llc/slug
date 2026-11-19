# Independent public fighter evidence reference

This is the preimplementation Set66 reference for the frozen candidate public
evidence contract. It checks supplied work, gym authorization, exact source and
appearance, receipt/assignment links and supplied-history arithmetic. It does
not authenticate issuance or birth/acceptance time, actual awards/finality,
canonical chain, current ownership, or career completeness/freshness.

`contract.md` is frozen at SHA256
`16a9166ebe1c7a0f5b0a8d554a043e6a690c29bec7864df9de123fc5cc01f272`.
`execution-lock-v2.json` binds the producer, case plan, exact existing-API public
inputs and repository-relative reference dependency manifest before execution.
Its SHA256 is `43aec989297bb766ad07244f07a4f40e7d341c8d5940c6e66a9a7cb383c8303f`.

The six actual inputs were read through existing APIs from copies of completed
Set64 v1 and Set65 v2 fixture roots before the new implementation. Each source
has both awarded gyms and one genuinely unawarded fighter. Expected stable pins
were independently retained from original stable markers and cross-checked
against original accepted entry gym identities before archive assembly. This is
test provenance, not a general claim that parsing a marker authenticates trust.
Original scopes, public source bytes, clocks and source sequences stay intact.

The reference does not call the new Go implementation. Unchanged independently
authored Python references provide secp256k1/low-S DER, job commitment, DNA,
manifest/portrait and history mathematics. A separate bounded transaction reader
checks stripped coinbase framing and reserved commitment outputs. The unchanged
standalone x/crypto/argon2 program imports no SLUG package and uses literal normal
parameters; dependency downloads are disabled. Six distinct header inputs are
recomputed per execution. Same-header modeled controls reuse only those exact
independently computed in-run values, never supplied Verified hashes. Each
successful logical verification calls work once; cheap rejection controls call
it zero times. No wallet, signer, production private key, RPC or node is used.

`reference-v1.json` is 2,820,282 bytes, SHA256
`eb51028daf2ebbda44dfbfdb79f58353137ed03f1836fad5b8db8cf428300772`.
It contains 28 positives and 44 negatives. Only six positives are observed actual
captures. Larger/tied/mixed histories are explicitly modeled claims using their
unchanged real original receipt. Two positive families intentionally demonstrate
the limits: coherent omission and unsigned accepted-time changes remain locally
consistent. No such positive is called an actual new award or authenticated time.

Positive records contain exact canonical archive bytes, independently supplied
expected pin and exact conditional Report. Negative records contain exact raw
ASCII input, expected pin and the required number of logical work calls (zero
for parser/source/signature/history refusal, one for a well-shaped false work
claim). Malformed input is never a successful report. Test the six actual normal
positives separately; all 28 cheap shape/roundtrip positives do not require
redundant Argon work.

The original generation passed before a reviewer noticed that inherited DNA
`int` fields had been constrained to uint32 in the independent scanner. The
successor fixes only their nonnegative signed64 grammar and adds 24 parser-only
boundary diagnostics. No DNA with those boundary values is valid evidence.
`parser-boundaries.json` records their structural/semantic distinction with
zero work calls. All 72 verification case bytes/verdicts and actual work results
are identical between attempts; the corpus SHA changes only because its top-level
execution-lock field now binds the corrected producer. Original source, complete
output and report remain preserved in workspace `artifacts/set66/oracle-attempt1`;
`predecessor/` and `verification/` retain the portable narrow correction record.

The final corpus reproduced byte-for-byte under Python3.14.3 and system3.9.6.
Their reports are in `verification/`. No new expected value was derived from the
implementation under test and no failed or superseded result was relabeled.

## Reproduce from a checkout

From this directory, with a locally installed Go and the already cached pinned
x/crypto dependency, use a fresh output directory outside the reference:

```sh
python3 -B generate_reference.py --lock execution-lock-v2.json --attempt /tmp/slug-evidence-reference-fresh --check reference-v1.json
python3 -B parser_boundaries.py --lock execution-lock-v2.json --output /tmp/slug-evidence-parser-boundaries-fresh.json
```

The producer locates its enclosing repository, validates every dependency hash
and reads existing immutable references at their committed repository paths.
External preparation may pass `--repo-root` explicitly. No workspace or temporary
input path is needed for repository reproduction. Do not copy the 43.65 MiB of
existing reference files into this bundle; `dependency-manifest.json` pins all
230 already committed files. Four old untracked stdout logs are explicitly
excluded as nondependencies and remain preserved externally. All inherited
directories remain read-only and Python bytecode writes are disabled.

Generation refuses an existing attempt directory, preserves its source binding,
report and failure trace, and never overwrites a different expected corpus.
Reserve the normal-work lane before reproduction: each run performs six normal
Argon2id computations. The parser-boundary command performs no work hashing.
