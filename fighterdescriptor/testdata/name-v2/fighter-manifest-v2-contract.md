# Static fighter manifest candidate v2 byte contract

Status: frozen engineering candidate after root review on November 19, 2026.
Root approved the exact candidate content, byte rules and compatibility mapping.
D51 permits duplicate names; D55 preserves each fighter's originally committed
generator. D74 remains a separate release gate. Current v1 defaults and
production-activation restrictions are unchanged.

## Authoritative inherited inputs

This contract incorporates the complete immutable v1 manifest contract at
`inherited/fighter-manifest-v1-contract.md`, raw SHA256:

```
ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80
```

It incorporates the original full v1 tables at `inherited/full-tables-v1.json`,
raw SHA256:

```
9f9380b708cae73b8d2641eaf04ecefb3a3b7fa9dd1b25b1f234f6511780b8c7
```

Their original v1 domain-separated table hash remains:

```
72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44
```

These are exact archived bytes, not mutable documentation paths. All DNA v1,
physical generation, style predicates/precedence and portrait trait selection
rules and streams are inherited unchanged from those inputs. Any conflict is
resolved by this document only for the explicitly replaced identity/name/digest
rules below. No other v1 rule is replaced.

## Version and input boundary

Supported new selector: integer `manifest_version=2`; exact generator ID:
`slug-fighter-manifest-candidate-v2`. DNA version and portrait trait version
remain integer 1. There is no latest-version fallback or caller-selected table.

Input is exactly 32 seed bytes in RPC display order. The text convenience
boundary accepts exactly 64 ASCII hexadecimal characters, upper or lower case,
then emits the lowercase seed in DNA. Zero is accepted. Shortened input,
whitespace, prefixes, non-ASCII and non-hex characters are rejected. Never
reverse decoded bytes or use textual hexadecimal bytes as the stream seed.

For a fixed seed, v2's complete `dna`, `physical`, `style` and `portrait` values
equal those produced by the inherited v1 contract exactly. This includes all
talent, aging, weight, height, reach, stance, style, color and scar rules. In
particular, do not use a v2 domain for non-name draws. An implementation may
reuse validated v1 generation internally and replace the fields specified
below, but the independent oracle must derive expectations from the archived
contract/reference rather than production output.

This is a versioned manifest statement, not permission to replace any retained
fighter's manifest. New issuance commits the exact descriptor before work.

## Exact v2 name table

`tables_v2.json` is exactly 9,017 ASCII bytes, compact JSON with fields in order
`first_names`, `last_names`; each value is an array of exactly 512 strings in
the reviewed order. There is no insignificant whitespace, escaped character,
BOM or terminal newline. Raw SHA256:

```
96e7ae3eaae038932355e44ef4a3a88ecab4c8f5fa1dea7371d748b6fab0b051
```

Only name components are in this v2 file; physical/portrait tables continue to
come from the separately pinned full v1 table. The v2 manifest `table_hash` is:

```
SHA256(ASCII "SLUG fighter manifest tables" || 00 || ASCII "v2" || 00
       || exact tables_v2.json bytes)
= b537490e1e602e27de01975aeec36f5fb53ba9a3a35f98143e8ecb7f9e2687c2
```

Both component arrays are distinct exactly and under ASCII case-folding. Each
component is 1-32 ASCII bytes of letters with optional internal apostrophes or
hyphens separating nonempty letter groups; none contains a space. The actual
reviewed maximum is 11 bytes per component. Runtime never trims, case-folds,
normalizes, translates or edits a component. These are format/content checks
of the fixed asset, not alternatives to its exact hash.

The displayed name is the selected first string, one U+0020 ASCII space, and
the selected surname exactly. No nickname or other field is added. Distinct
component-index pairs produce 262,144 distinct strings, including under ASCII
case-folding. Different seeds may select the same pair, and a first name may
equal its surname. No global collision lookup, redraw, suffix or blacklist is
part of generation.

## Independent name streams

Start a separate unsigned 64-bit counter at zero for each of the two exact
ASCII labels `name.first` and `name.last`. Each draw hashes these bytes:

```
ASCII "SLUG fighter manifest" || 00 || ASCII "v2" || 00
|| ASCII label || 00 || seed[32] || counter[8 little-endian]
```

Interpret digest bytes 0 through 7 as one unsigned 64-bit big-endian integer
`r`; discard all other digest bytes. Increment only that label's counter. The
maximum uint64 counter may be encoded once; a following draw fails rather
than wrapping. No clock, population, other fighter, source sequence or mutable
state enters the preimage. Non-name draws use their inherited v1 domains.

For positive bound `n`, let `threshold = 2^64 mod n`; reject `r < threshold`,
otherwise return `r mod n`. Permit at most 256 draws per field including
rejected draws. Exhaustion fails the entire generation with no partial result,
fallback value, label or seed. Both v2 name bounds are exactly `n=512`, so the
threshold is zero and the first ordinary SHA256 draw is accepted. Retaining
the bounded rejection contract does not imply that actual v2 name draws can
exercise rejection. Direct helper/counter test cases are separately labelled.

The resulting indices select the exact ordered first/surname arrays. No
modulo reduction of a text hash, byte-order reversal or shared counter is valid.

## Canonical manifest and digest

Use the inherited exact manifest shape and key order, replacing only these
values: `manifest_version=2`, the v2 generator ID, the v2 name table hash, and
the complete `name` object. Specifically:

```
manifest_version, generator_id, table_hash,
dna: dna_version, seed, rarity, talent_budget,
     attributes: power, speed, chin, stamina, skill,
     prime_weeks, decline_weeks,
name: first, last, display,
physical: height_in, reach_in, stance, weight_lb, weight_class,
style,
portrait: portrait_version, skin, hair_style, hair_color, facial_hair,
          head_shape, mouth, brow, background, scar, shoulder_jitter
```

All keys occur exactly once in that order. Serialization is compact canonical
ASCII JSON with no insignificant whitespace, terminal newline, escaped ASCII,
non-ASCII text, array, null or boolean. Apostrophes in table-derived names are
literal JSON string characters; they are not JSON escapes. Integers use minimal
base-10 spelling, never exponent, plus sign, leading zero or negative zero.
Only `shoulder_jitter` may be negative. Inherited bounds/types and enum rules
continue to apply. Every value must equal full regeneration for the embedded
seed and exact selected version, not merely satisfy range checks.

The parser retains the 8,192-byte manifest cap and depth-8 cap, enforced before
unbounded decoding, with strict duplicate/unknown/missing/case-alias/escaped-
alias/trailing-data rejection. No JSON integer passes through float64. A
renderer or registry cannot accept a plausible caller-edited name. The name
table asset is not a manifest and its 9,017-byte size does not enlarge any
manifest/IPC cap.

Manifest digest is lowercase hexadecimal SHA256 of:

```
ASCII "SLUG fighter manifest digest" || 00 || ASCII "v2" || 00
|| canonical v2 manifest bytes
```

There is no self-referential digest field. This digest does not grant qualifying
work, birth, account control, ownership or official publication authority.

## Compatibility and descriptor binding

The v1 functions, tuple, table bytes, stream/digest domains, output bytes and
existing golden corpora remain unchanged. Explicit version 1 still produces
the same v1 manifest for the same seed; version 2 produces this v2 manifest.
Unknown versions and mixed generator/table/version tuples fail closed.

The new descriptor keeps descriptor schema/domain version 1 and DNA/portrait
trait version 1, but uses ManifestVersion 2 and RendererVersion 2. It pins this
contract's exact raw SHA256, the v2 table hash above, and the exact raw SHA256
of the separately reviewed portrait-v2 contract. A separate review binding
file records those non-self-referential contract hashes. Resolve only the
complete approved tuple; never mix individually supported components.

`CandidateDescriptor()` and current application defaults remain the old v1
tuple. V2 creation requires explicit opt-in and a fresh candidate source.
Existing source roots and retained admissions are never repinned; reopening
requires exact retained descriptor equality. Production resolution remains
unavailable. No genesis, real keys or production activation is authorized.

The fight engine and its contract are unchanged. Replacing a manifest or its
descriptor changes derived commitments/InputID values where those bytes are
inputs. Do not claim identical fight RNG streams, seeds or outcomes across
hypothetical version substitutions merely because physical/DNA values match.

## Review and independent derivation boundary

Root reviews this exact contract before a new oracle or expected vectors are
generated. Independently derive v2 name streams, complete manifest bytes and
digests using the frozen inputs. Preserve the original v1 corpus, and verify
inherited non-name equality. Scripted stream cases, actual-generation cases,
measured collision samples and real admitted proofs must be distinguished.
Content/collision acceptance for release remains separate from byte correctness;
no numeric collision tolerance or production population is adopted here.
