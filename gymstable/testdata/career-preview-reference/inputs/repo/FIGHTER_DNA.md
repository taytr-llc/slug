# Fighter DNA component, version 1

This is the first off-chain fighter-layer component. Given the same 32-byte
work digest and DNA version, any implementation can reproduce a talent tier,
five base attributes and career lengths. It implements D3–D6, D21–D22 and the
DNA part of D55. It neither validates qualifying work nor registers a fighter
or establishes ownership. It does not change node consensus or signed league
checkpoint policy.

`dna_version: 1` versions only this component. Names, physical traits, style,
portrait tables, registration and complete fighter manifests are later
components; this is not a finished full fighter generator. Future manifests
must retain the DNA version and seed rather than regenerate old DNA using a
new version. Unknown versions fail explicitly.

## Input and output

The input is exactly 64 hexadecimal characters representing 32 bytes in the
display order used by RPC block hashes. Decode the displayed text directly;
do not reverse it into `chainhash.Hash`'s internal byte order. Uppercase input
is accepted, but output is lowercase. Whitespace and `0x` prefixes are not
accepted. An all-zero seed is valid for a preview; it is not proof of mining.

The JSON output contains `dna_version`, `seed`, `rarity`, `talent_budget`,
`attributes` (Power, Speed, Chin, Stamina and Skill), `prime_weeks` and
`decline_weeks`. Attributes are original mined values; experience, aging,
weekly training and fight form are not applied. Career lengths do not start a
career clock: registration must establish an authenticated birth anchor.

No fighter ID is minted by this preview. The registration format must define
identity and bind work to its gym before work is performed, preventing a
copied public digest from becoming a competing ownership claim.

## Exact deterministic algorithm

All arithmetic is integer arithmetic. No platform random source, floating
point operation, wall clock, network request or secret key is used.

Independent streams use the ASCII labels `rarity`, `budget`, `attributes`,
`prime` and `decline`. Each stream starts its own unsigned 64-bit counter at
zero. For every draw, concatenate:

1. ASCII `SLUG fighter DNA`, followed by byte `00`, ASCII `v1`, byte `00`.
2. The stream's ASCII label, followed by byte `00`.
3. The 32 decoded seed bytes in display order.
4. The counter encoded as eight little-endian bytes.

Hash this message with SHA-256. Interpret the first eight digest bytes as one
unsigned big-endian 64-bit integer `r`, then increment that stream's counter.
The remaining 24 digest bytes are unused. SHA-256 expands the work digest;
it is not a replacement proof-of-work algorithm. In particular, rarity does
not inspect the leading zero bits constrained by the mining target directly.

To sample uniformly from `[0, n)`, calculate `t = 2^64 mod n`. Draw until
`r >= t`, then return `r mod n`. Rejected draws still consume a counter.
This rejection rule avoids modulo bias.

The rarity stream samples a ticket from `[0, 100)`:

| Tickets | Rarity | Probability | Inclusive talent budget |
| --- | --- | --- | --- |
| 0–54 | club | 55% | 250–299 |
| 55–81 | professional | 27% | 300–349 |
| 82–93 | contender | 12% | 350–399 |
| 94–98 | elite | 5% | 400–449 |
| 99 | generational | 1% | 450–489 |

The budget stream selects uniformly within the chosen inclusive range.
Within that budget, every ordered five-tuple of integers in `[1, 100]` with
the required sum is equally likely. Count these tuples, draw one rank using
the attributes stream, and unrank in lexicographic order of Power, Speed,
Chin, Stamina and Skill (each candidate value considered from 1 through 100).
The Go implementation counts with a bounded-composition dynamic program;
the independent Python oracle uses inclusion–exclusion and binomial
coefficients. Counts fit in uint64 because even all possible five-tuples
number only `100^5`.

This allocation distribution is an explicit engineering choice: it preserves
the chosen budget exactly, treats attribute positions symmetrically, and
does not clamp or repeatedly repair independently rolled attributes. The
spec fixes the rarity pyramid and budgets but does not prescribe the
within-budget distribution. Changing this distribution requires a new DNA
version; it must never silently change existing fighters.

Finally, the prime stream yields `26 + sample(27)` weeks, and the decline
stream yields `13 + sample(27)` weeks, both inclusive of their bounds.

## Independent reproduction

`fighter/testdata/reference_dna.py` derives the committed JSON vectors using
Python's standard library from this byte contract. Expected outputs were
not copied from the Go generator. The vectors include chain hashes, zero
and all-one seeds, a byte-order sentinel and examples of every talent tier.
The Go tests compare all output fields against these fixtures.

Use the offline `cmd/slugfighter` preview to inspect DNA. A preview is not a
claim that a seed meets a prospect target, came from a particular gym, or
was discovered at a particular time.

## Next registration contract

Before gym prospects can register, specify and test:

- A prospect target independent of the network target and pool variable
  difficulty, calibrated to D30's discovery cadence.
- A pre-work gym commitment, share proof, pool attestation and solo proof.
- Freshness, duplicate/replay handling and the authenticated mined-time
  anchor used to begin aging.
- Versioned public ownership/transfer records and checkpoint anchoring.

The report-only league signing key must not become a chain-selection veto.
Names, physical ranges, class population shaping and deterministic portrait
tables remain the build-time work listed in spec section 17.
