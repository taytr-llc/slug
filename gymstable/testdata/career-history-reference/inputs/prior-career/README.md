# Independent modeled career-accounting reference

This Python-standard-library model derives the CareerPolicyID, ActivationID,
FinalityDigest, AwardID, ordered Event bytes and sealed Snapshot prefix digests
and requested totals from the pinned Set49 contract. New Go implementation
sources/outputs were not read or executed. This is not chain authentication,
an actual trusted Reader, an activation migration, or filesystem/crash evidence.

`inputs/CAREER_ACCOUNTING.md` preserves the original approved `d676a634...`
contract. The approved `8088f398...` constructor-preflight successor and exact
diff are additional inputs; neither changes DTOs or digest equations. The
input index records those hashes and the exact old Set48 Git commit/file hashes
and bounded declaration/method extractions establishing Finality's nested v1
JSON order. It also pins the previously independent fightercareer arithmetic
corpus/generator without copying or importing that corpus recursively. Standard
award arithmetic is independently repeated from its unchanged prose contract.

The Finality values are **synthetic encoding fixtures**: correctly sized fields
and old codec order, with dummy signatures, header hashes and transaction data.
No cryptographic/PoW/canonical-chain validity is claimed, and no actual published
result is authorized by this model. Event inputs stand for already audited
source facts solely to exercise the accounting equations. Multiple modeled
results are not proof of valid real weekly schedules or C1 source admission.
There is no new signing, source import, engine execution or Argon work.

`generate.py --check corpus.json` regenerates exact corpus bytes without writes;
`--write /fresh/output.json` writes only a fresh exclusive destination. Output
is deterministic across Python versions/timezones. The outer corpus uses
sorted JSON keys for reproducibility; each `canonical_json` string preserves
the exact required declaration order and Go-compatible string escaping, without
LF. Tests should use those strings/digests, not outer-object key order. The
policy and activation/award preimages are retained explicitly. Snapshot prefix
preimages are independently reconstructed from referenced canonical events.

Corpus schema:

- `policy`: exact literal policy JSON, preimage hex and expected PolicyID.
- `activations`: three named ordered objects, canonical bytes and preimages.
- `finalities`: four synthetic original-v1 objects/canonical byte strings and
  expected domain-separated digests, differing only by selected observation time.
- `events`: named synthetic standard events with canonical bytes/AwardID preimage.
- `snapshot_cases`: named activation/event selections, sorted requested IDs,
  as-of, mode, durable clock and modeled owner state; expected error or prefix
  digest, ordered included AwardIDs, requested totals/found flags, and a zero,
  false unrequested lookup.
- `index_refusals`: bounded malformed trusted-index examples with only a boolean
  invariant verdict. No unspecified public API error mapping is invented.
- `index_transitions`: proposed index changes; retained event deletion or any
  immutable event change refuses, even if the replacement independently has a
  structurally valid shape. These are abstract invariants, not executed commits.

The natural cutoff is the literal December 12, 2026 Saturday noon Pacific
(20:00 UTC); no new calendar policy is derived. Current includes equal time;
Cutoff excludes equal time and needs the owner's durable clock at/after cutoff.
The full temporal event prefix affects the digest even for unrelated fighters.
Requested IDs, temporal mode and as-of affect the digest even for empty totals.
Same-time records sort by AwardID. Repeated cutoff capture and later equal/after
events cannot change the sealed historical digest. Current can change when a
new same-millisecond event arrives. Post-conflict examples retain identical
first-Finality/Event bytes; they do not simulate chain governance or revocation.
Every fixture's owner clock covers all retained event times and is itself a
valid timestamp, even for invalid or unfenced requests. The Current no-fence
control queries noon against a valid clock at noon minus one millisecond and
only events committed by that clock; it does not model future committed rows.
The original candidate's clock inconsistencies were found in root's contract
review before any Go comparison. Its complete source/index/corpus/report are
preserved externally under `artifacts/set49/careerstate-reference-attempt1-preserved`.

Requested known-zero and unrequested IDs have distinct found flags. Closed,
inactive, poisoned, corrupt or cancelled state never returns authorized zeros.
No additional Current clock fence is invented. Refusal cases isolate one cause;
cross-error precedence is not specified. Sixteen results exercise XP saturation
without suppressing wins/losses/points. Every actual engine method and decisive
corner ordering is included; title arithmetic and rank bonuses are not enabled.
Full256 startup/capacity, durability, fencing under concurrent real mutations,
scope authentication and consumer original-View authorization require separate
Go and actual-account tests. This finite corpus does not establish those claims.
