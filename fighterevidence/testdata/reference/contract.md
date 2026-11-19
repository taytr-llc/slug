# Public fighter evidence v1 — frozen byte and verification contract

Status: frozen engineering candidate after root review, November 19, 2026. The root approved the byte rules, API names and report fields, with explicit profile-neutral cheap helpers and nil-context refusal. No new oracle values, archive examples or expected verdicts have been generated. The initial inventory is frozen as a provenance checkpoint only; additional actual public input extraction must be frozen before full oracle generation. This additive candidate format does not change any existing registry, stable, history, result, account or recovery contract.

## 1. Purpose and authority limits

The file preserves one original public candidate admission receipt, one immutable captured fighter history and every row of that supplied capture. The checker proves only the checks explicitly listed below, relative to an independently supplied exact expected stable Pin.

This does not complete the portable authoritative-history milestone. Admission issuance and its birth/acceptance timestamps are unsigned authority assertions. Career summaries are not result-chain proofs. A matching authority ID is a namespace match, not an issuer signature; a matching HistoryID is a correlation commitment, not authentication or completeness. No ownership, import, transfer, chain-selection, production activation or new signing authority is introduced.

Use the phrase **work consistency**, never “birth PoW verified” or “birth verified.” Valid work commits to the supplied job but cannot authenticate when that job was actually issued, mined or admitted. Local range/equality checks on timestamps are not trusted wall-clock observations.

## 2. Exact envelope and integrity

The compact file has exactly these five members, in this order:

```text
{"format":"slug-public-fighter-evidence","version":1,"admission":A,"capture":C,"rows":[R0,...,Rn]}
```

`A` is the existing canonical schema-5 `registry.ManifestRecord`; `C` is the existing canonical v1 `gymstable.CareerHistoryCapture`; each `R` is an existing canonical v1 `gymstable.CareerHistoryRow`. The names `A`, `C`, `R` are notation, not strings or additional envelopes. Rows remain newest first, exactly as the captured history presents them. Empty rows encode as `[]`, never `null`. No fields are optional.

No compression, filename table, path, remote reference, attachment, signature wrapper, embedded expected pin, SVG, new descriptor definition, extra ID or terminal newline is added. The capture's existing pin is retained as a **claim to compare**, never adopted as expected trust. Permanent pseudonymous account/gym/source IDs already present in nested records remain public correlation fields.

The sole whole-file integrity value is lowercase hexadecimal `SHA256(fileBytes)`, reported externally with the byte count. It is not stored inside the file and is not an authority attestation. No new domain-separated evidence identifier is introduced. Preserve the independently defined nested IDs and digests unchanged.

## 3. Closed parser and byte bounds

All bounds are inclusive maxima. Reject input over a bound before proportional allocation, recursive DTO decoding, signature verification or Argon work. File readers read at most limit+1 bytes to detect excess; there is no truncate-and-accept behavior.

| Object | Byte maximum | Additional limits |
| --- | ---: | --- |
| Complete archive | 524,288 | One complete JSON object; no trailing bytes |
| Admission `A` | 65,536 | Existing record grammar, standalone container depth 4 |
| Capture `C` | 8,192 | Existing capture grammar and semantic checks |
| One history row `R` | 1,536 | Existing row grammar and semantic checks |
| Embedded row event | 1,344 | Existing career event grammar |
| `rows` | 256 elements | Non-null array; count exactly `C.event_count` |
| Independently supplied pin file | 4,096 | Existing `gymstable.ParsePin` contract |

The new archive scanner has a separate maximum container nesting depth of 10, counting the root object as 1, an array as one level and an object nested in an array as the next level. It preserves each nested contract's stricter bound when parsing that component. The outer allowance does not relax registry's depth 4 or existing history's standalone typed-depth bound 8. There are no generic recursive extensions.

The complete existing container path inventory is Appendix A. It is normative for this new envelope: in particular, `capture.status.slots` is a fixed ten-element array. Later additions to Go DTOs do not silently extend this archive version.

The entire accepted archive is ASCII canonical JSON. Only the exact declared object fields and exact nested DTO fields are allowed. Reject duplicate, missing, additional or differently cased keys at every level; reject `null`, nonfinite numbers, signed/negative numeric spelling, fractional or exponent spelling, leading-zero variants, comments, BOM, whitespace outside strings and multiple JSON values. Strings must equal their existing canonical decoded value and canonical JSON spelling. Their existing enum, hexadecimal, length and date checks remain in force. No case normalization, string unescaping normalization or whitespace normalization is performed on an accepted archive.

Before populating slices, the scanner permits arrays only where the existing nested schema permits them: `rows` (0..256), the admission proof's `merkle_branch` (0..20 canonical 32-byte hashes), and capture status `slots` (exactly 10 existing slot strings). Any other variable array is rejected. Embedded fixed arrays retain their exact size if an inherited schema requires one. Every outer key has a fixed expected type. Bound object members to the exact schema, rather than accumulating arbitrary names.

Admission strings are additionally bounded by their inherited field limits: the largest is coinbase lowercase hex for at most 16,384 bytes (32,768 characters); header is exactly 80 bytes of hex; hash IDs are 32 bytes of hex; compressed public keys are 33 bytes of hex; DER signature is 8..72 bytes of hex. The non-null merkle branch contains at most 20 canonical hashes. Capture and row strings retain the existing history token cap of 4,096 decoded bytes and all stricter field-specific limits. No blanket 4,096-character limit is applied to admission coinbase data.

Typed integers fit their inherited widths. History/capture integers retain the existing nonnegative maximum-safe-integer limit 9,007,199,254,740,991 and stricter timestamp/count bounds, including timestamp maximum 8,640,000,000,000,000. Registry sequence is additionally 1..1,000 because this format must bind a valid existing stable assignment. Fixed format/version values are the exact literals above. No integer is round-tripped through JavaScript.

Canonical byte equality is mandatory after closed parsing. Compose the envelope in the literal member order above, using the existing canonical ordered encodings of `A`, `C` and each row; require byte equality with the input. Existing history transport parsers may tolerate field order or whitespace, but this **new archive** does not. Do not change those legacy transport parsers. The outer scanner rejects oversized/deep/duplicate/nonclosed input before DTO allocation; canonical re-encoding is an additional check, not the only resource guard.

The sum of independent component maxima is 466,944 bytes before 255 row commas and fixed envelope delimiters. This fits below 524,288, but all actual component and whole-file bounds remain checked; no claim is made that all semantic maxima can coexist in a real record.

## 4. Independently supplied source

Frozen root package API:

```go
type Archive struct {
    Format string
    Version uint32
    Admission registry.ManifestRecord
    Capture gymstable.CareerHistoryCapture
    Rows []gymstable.CareerHistoryRow
}

Parse(raw []byte) (Archive, error)
ValidateSuppliedShape(expected gymstable.Pin, archive Archive) (Archive, error)
Marshal(expected gymstable.Pin, archive Archive) ([]byte, error)
VerifySupplied(ctx context.Context, expected gymstable.Pin, raw []byte) (Report, error)
```

Archive fields use lowercase JSON names in the frozen order from section 2. `Parse` is closed/canonical parsing only, not a work check. `ValidateSuppliedShape` returns a detached checked value and performs all cheap source, signature, static and history checks without Argon. `Marshal` applies those same cheap checks before canonical serialization. These three cheap data helpers may remain profile-neutral; none returns a trusted registry/stable handle or a successful normal-work Report. Live source extraction remains normal-profile-only and uses one verified concrete source read followed by cheap `Marshal`; it must not call offline `VerifySupplied` and hash the same receipt again. Root approved these exact exported API names and report fields on November 19, 2026.

Validate `expected` with the existing exact `gymstable.ValidatePin` semantics, preserving stable Pin1 versus shared Pin2. Validate the full fixture identity, account authority/account ID, source schema/network/genesis/authority/descriptor, condition version and projector. Resolve only the exact compiled candidate descriptor named by that source. Neither “some supported descriptor” nor a different supported fixture/gym is sufficient.

Require `C.status.pin == expected` as complete value equality and recompute `C.status.store_id` from that expected pin. Do not read expected trust from `C`, from `A`, from a sibling archive file or from an unrequested endpoint. Caller-supplied expected trust remains the caller's responsibility; matching values do not authenticate its provenance.

The no-network CLI is `slugevidence verify --source-pin PIN_FILE --file EVIDENCE_FILE`. Both flags are mandatory. It reads only those bounded explicit files, parses the pin using its unchanged parser, invokes the checker and prints a small report. It opens no wallets/stores, invokes no RPC, creates no key, writes no trust file and performs no discovery. Missing expected trust is an error; there is no automatic unbound success mode. The CLI may use normal stdout newline framing for its report; that does not alter archive bytes.

## 5. Verification phases and inherited semantics

### 5.1 Profile and cheap rejection first

`VerifySupplied` in an unsupported cheap-PoW build returns its profile refusal before parsing caller archive data or creating output/filesystem effects. The CLI checks profile before opening either input. This profile requirement applies to normal-work verification and live source extraction; it does not turn profile-neutral `Parse`, `ValidateSuppliedShape` or `Marshal` into work checks. `VerifySupplied` explicitly refuses a nil context without panicking, starting proof work or returning a Report. Check non-nil context cancellation before expensive work and before publishing success. Do not return a success report after cancellation. `VerifySupplied` owns one process-wide fail-fast TryLock gate covering its bounded parsing/validation/hash/report lifetime: a concurrent caller returns busy immediately, without joining an unbounded queue or starting work. Release the gate on every exit, including cancellation and failures. Pure Marshal/shape helpers do not consume or bypass a work-verification claim.

Perform bounded parser, exact source/descriptor/identity comparison, all timestamp and object link checks, supplied history consistency, signature validation and deterministic static derivation before the single normal proof check whenever possible. None of those checks requires normal Argon or a node. Failures in these phases must start no Argon work. Intermediate checks are not published as overall success.

### 5.2 Original receipt consistency

Require schema 5, nonzero original sequence no greater than 1,000, job schema 2 and birth rule 1, proof version 2, authorization version 2, supplied-context Verified version 2 and static-binding version 1. Reuse canonical public context/proof/auth/job validation without silently upgrading or normalizing values.

Validate the complete manifest job via `prospectjob.ValidateManifest`. Its recomputed job commitment must match the supplied job. Match job authority/descriptor/network/genesis and gym public key/ID to the exact expected source and identity. Require receipt Context equal job Context, proof JobID equal job ID, proof GymID equal job GymID and authorization PublicKey equal job PublicKey. Require the proof's embedded commitment to match that job/context as part of normal verification. None of these checks authenticates actual issuance.

Require birth RuleVersion equal job RuleVersion, BornAtMS equal IssuedAtMS, and `IssuedAtMS <= AcceptedAtMS < ExpiresAtMS`. Keep the five-minute job interval and commitment derivation exactly as inherited. Historic expiry is not compared to the machine's current clock. No claimed LastClockMS, live parent or live issuance journal is invented for the offline checker.

Verify the gym authorization over the exact supplied proof/context using `gymauth.VerifyAuthorization`. This proves key control over that message, not current ownership, original account enrollment, issuance, accepted time or career claims.

Resolve the exact descriptor tuple and regenerate the manifest from its ManifestVersion and the supplied WorkHash; require DNA equality, exact manifest digest and exact renderer-version portrait digest/static binding. Use the descriptor's correct v1/v2 domains. This deterministic comparison becomes work-backed only after the next phase confirms the supplied WorkHash and all Verified fields.

### 5.3 One normal work check

Invoke `prospect.Verify(A.Context, A.Proof)` exactly once per successful verification attempt and require equality with every supplied `A.Verified` field. This is normal Argon2id work consistency for a supplied context. No second header hash is needed to derive static appearance, verify assignment or check history.

The existing normal proof call is uncancellable once entered. Cancellation joins that one call, then suppresses success; it never detaches the hash or starts another check. Callers serialize their bounded desktop verification/export operation rather than spawning arbitrary proof goroutines. The report cannot accept a caller-provided preverified-work flag or normal-work cache as an oracle.

After fresh work verification, apply `registry.ValidateEligibility` against those verified fields and the supplied job context. Current-parent and canonical-winning-block are **claimed observations whose internal consistency is checked**; the offline operation makes no canonical-chain observation and grants no historical admission.

### 5.4 Receipt-to-assignment binding

Construct a comparison value only, never a trusted stable capability, with exactly the existing Assignment fields: Version1; original Sequence; verified ProspectID, GymID and WorkHash; unchanged BornAtMS and AcceptedAtMS; exact static descriptor/manifest/portrait digests; and the source receipt commitment. Require complete equality with `C.assignment`.

The source receipt commitment preimage is exactly:

```text
ASCII("SLUG stable source record") || 0x00 || ASCII("v1") || 0x00
|| LE32(length(A_canonical_bytes)) || A_canonical_bytes
```

The digest is lowercase SHA256 hex. The length prefix is mandatory; do not substitute raw-domain concatenation. Other nested hashes/IDs keep their existing independently frozen framing. Assignment Sequence is an original global source sequence under Pin2, not the fighter's gym-local ordinal or history ordinal.

### 5.5 Complete supplied capture, not complete career

Run the unchanged `ValidateCareerHistoryCapture`, including original source/status/store/activation/assignment/time/count/totals/HistoryID checks. Require `len(rows) == C.event_count`, preserving PrefixCount separately. PrefixCount includes other fighters' events and is not a claim reconstructed from selected rows.

Run existing typed row validation, and optionally `ValidateCareerHistoryPageLimit` on consecutive chunks of at most eight. Synthetic page offsets here are local checking aids; no new pages or trusted read handles are minted. Page-local validation alone is insufficient. Across **all** supplied rows enforce:

- Ordinals are exactly EventCount, EventCount-1, ..., 1, in newest-first order.
- AwardID and ResultKey are each globally unique, including across chunk boundaries.
- Newest-first ordering is descending `(EffectiveAtMS, AwardID)`, exactly as the inherited history contract requires; equal times still require strict descending AwardID.
- Each original Event is structurally valid for the same Activation and selected original account/gym/fighter corner, with EffectiveAtMS no later than captured AsOfMS.
- Recompute the inherited standard award delta from winner/method and selected corner. Earned XP stays one; applied XP and XPBefore obey the existing cap 15.
- Fold all rows chronologically from zero using the unchanged career arithmetic. Every TotalsAfter and final capture Totals must match, including every boundary. Zero events requires zero totals and `[]`.

Recompute HistoryID from the unchanged framing. CareerDigest remains an opaque supplied commitment because omitted global events contributed to it. Do not try to rebuild CareerDigest from these fighter-only rows. A coherent omitted or invented history with recomputed local fields may still satisfy this contract. The successful report must always retain that limitation, even for a trusted-looking fixture ID or empty history.

## 6. Report and user-facing semantics

The report describes a successful **conditional consistency** check, never a generic verified-fighter claim. Frozen closed report vocabulary:

```text
version: 1
status: "consistent_supplied_evidence"
file_sha256: lowercase SHA256 of whole canonical file
file_bytes: exact bounded file byte count
source_store_id: ID of independently supplied expected pin
prospect_id: exact selected original ProspectID
history_id: exact supplied capture HistoryID
captured_as_of_ms: supplied capture time, explicitly unauthenticated
event_count: number of supplied captured rows
checks: {
  exact_source_match: true,
  supplied_work_consistent: true,
  gym_authorization_valid: true,
  static_appearance_consistent: true,
  receipt_assignment_consistent: true,
  supplied_history_consistent: true
}
unverified: {
  issuance_and_admission_times: true,
  actual_awards_and_result_finality: true,
  canonical_chain: true,
  current_ownership: true,
  history_completeness_and_freshness: true
}
```

These booleans and text are checker-owned, not copied from the file. Errors return no successful Report and no partial trusted handle. Error categories are profile, invalid evidence, source mismatch, busy, and cancellation. The implementing agent chooses stable exported sentinel names for those categories; detailed bounded diagnostics may identify the failed check without echoing file contents, tokens or paths. Do not turn a mismatch into an unbound pass.

UI action: **Export public evidence**. Explanation: “Saves the original receipt and this captured history. Offline checks confirm supplied work and record consistency; they do not prove admission time, actual awards, ownership or a complete career.”

CLI success headline: **Supplied evidence is consistent with the selected source.** Follow with: “Issuance and birth/acceptance times, actual result awards/finality, complete history and current ownership are not established.” A checksum is called a file checksum, not a certificate.

## 7. Live export and optional separate result evidence

Live export consumes an existing opaque `CareerHistoryRead` under its original View/session, not a caller-created capture. A narrow gymstable accessor re-reads/verifies the selected exact receipt outside account leases and compares the same assignment, then provides an owned public receipt and original captured pages. This supports older fighters outside the eight-item UI receipt cache without a JSON-to-authority constructor. `fighterevidence` may import gymstable; gymstable must not import fighterevidence.

Native selection owns its pinned directory identity through synchronous helper descriptor acquisition. Large temp write/fsync occurs outside account leases; exclusive final link/directory sync remains under the original View fence. Preserve uncertain publication and cleanup failure. Set62 recovery's 64 KiB and RecoveryExport semantics remain unchanged. New work uses a separately bounded public writer and the already applicable View capability; a newly authenticated held session may view/export public evidence, while a revoked original session cannot finish.

Archive assembly retains the original capture across every page and dialog. Cancel/reset/logout/Stop/Close/backup drain owned work and any entered proof; no replacement credentials or newer capture are substituted. Budget temporary bytes explicitly, since a 512 KiB file plus existing retained history exceeds some legacy lane allocations. IPC returns a small summary rather than raw evidence bytes.

Existing result proofs remain separate bounded files (24 MiB, up to 512 unique headers and three block bodies). A row's ResultKey/AwardID/FinalityDigest can locate a separately requested proof, but no proof file is embedded, auto-fetched, auto-trusted or automatically verified by this contract. Future explicit per-result checks must take independent exact result-source trust and bind all original row links. Hash each unique header once within that separate invocation; never accept claimant “already checked” flags. Such checks can support named results under their own stated chain limits, but still cannot authenticate unsigned birth/acceptance time or demonstrate career completeness.

## 8. Freeze and oracle gate

Root approved the exact contract/report/parser choices. Additional actual public source-input selection and extraction must still be frozen before generating full expected files, hashes or verdict vectors. Archive the complete inherited inputs/scripts with hashes; freeze any actual v1/v2 capture as public observed input, clearly distinct from independently derived expected output. Use independent Python framing/history/manifest/portrait and signature/work references rather than the new Go implementation to derive expectations. Preserve all failed authoring/checker attempts. No production Go output becomes its own golden oracle.

## Appendix A. Complete nested container inventory and field order

This inventory was compared to the actual existing DTO declarations and immutable receipt/history fixture shapes. Objects have exactly the members listed in order; every non-container field retains its inherited scalar type and validation. `rows[*]` denotes each array element, not a literal JSON member. The only arrays are the three explicitly listed after the table.

| Object path | Ordered members |
| --- | --- |
| root | format, version, admission, capture, rows |
| admission | version, sequence, context, proof, verified, authorization, job, birth, eligibility, static_binding |
| admission.context | version, network, genesis, dna_version, prospect_target, parent_hash, parent_height, expected_bits, min_timestamp, max_timestamp |
| admission.proof | version, gym_id, header, coinbase, merkle_branch, job_id |
| admission.verified | version, verification, network, genesis, prospect_id, policy_id, gym_id, work_hash, parent_hash, parent_height, dna |
| admission.verified.dna | dna_version, seed, rarity, talent_budget, attributes, prime_weeks, decline_weeks |
| admission.verified.dna.attributes | power, speed, chin, stamina, skill |
| admission.authorization | version, public_key, signature |
| admission.job | version, rule_version, id, authority_id, descriptor_id, context, public_key, gym_id, challenge, issued_at_ms, expires_at_ms |
| admission.job.context | version, network, genesis, dna_version, prospect_target, parent_hash, parent_height, expected_bits, min_timestamp, max_timestamp |
| admission.birth | rule_version, born_at_ms, accepted_at_ms |
| admission.eligibility | rule_version, source, hash, height |
| admission.static_binding | version, descriptor_id, manifest_digest, portrait_digest |
| capture | version, status, activation, career_digest, as_of_ms, assignment, prefix_count, event_count, totals, history_id |
| capture.status | version, store_id, pin, checkpoint, observed_source_count, caught_up, roster_revision, slots, operation_count, pending_operation_id, last_operation_id, last_as_of_ms |
| capture.status.pin | version, account_authority_id, account_id, identity, source, condition_version, projector_id |
| capture.status.pin.identity | network, genesis, profile, root_public_key, gym_public_key, gym_id |
| capture.status.pin.source | version, network, genesis, authority_id, descriptor_id |
| capture.activation | version, policy_id, activation_id, result_store_id, result_created_at_ms, source_created_at_ms, activated_at_ms |
| capture.assignment | version, sequence, prospect_id, gym_id, work_hash, born_at_ms, accepted_at_ms, descriptor_id, manifest_digest, portrait_digest, source_record_digest |
| capture.totals | wins, losses, draws, xp, base_points |
| rows[*] | ordinal, event, earned, xp_before, xp_applied, totals_after |
| rows[*].event | version, award_id, result_key, card_id, bout_id, source_record_digest, input_id, result_digest, finality_digest, effective_at_ms, saturday, corner_a, corner_b, winner, method |
| rows[*].event.corner_a | prospect_id, account_id, gym_id |
| rows[*].event.corner_b | prospect_id, account_id, gym_id |
| rows[*].earned | wins, losses, draws, xp, base_points |
| rows[*].totals_after | wins, losses, draws, xp, base_points |

Array paths and cardinality: `admission.proof.merkle_branch` has 0..20 canonical hash strings; `capture.status.slots` has exactly ten slot strings; `rows` has 0..256 objects in the row schema above. No array may be null or appear under another path.
