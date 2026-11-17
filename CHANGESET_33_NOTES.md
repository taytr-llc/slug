# Change set 33 — candidate static fighter manifests

Branch: `slug-fighter-manifest`, stacked on set 32 implementation
`1f300b6b6442f9933c968e8c8266aaf4071136b2`. The supplied simulation and
prototypes remain intact in the workspace; the spec takes precedence.

## What changed

`fightermanifest` adds a pure versioned preview with the entire unchanged DNA
v1 record, deterministic names, physical traits, derived style and portrait
selections. Independently frozen tables preserve source names and appearance
choices. Separate SHA256 streams isolate every new random field from DNA and
one another. Integer distributions, round-to-even heights, bounded unbiased
sampling and exact style predicates are defined in `FIGHTER_MANIFEST.md`.

The strict canonical codec enforces an 8192-byte and depth-8 ceiling, rejects
aliases, duplicate/missing/unknown fields and noncanonical spelling/order,
and regenerates every field from the seed. The separate digest fingerprints
only a preview. No arbitrary markup or image URL is accepted.

The optional `slugfighter preview --work-hash HEX` prints exact canonical
manifest bytes; `--digest` prints its digest. The original DNA invocation and
its output, help and errors remain compatible.

## Verification

All ten chain modules build; focused normal, race and cheap-tag suites, vet,
gofmt and diff checks pass. The independent Python reference regenerates its
entire corpus. Runtime, tests and expected vectors stayed unchanged during
final integrated validation.

The corpus covers 50 manifests, 68 stream vectors, rejection/exhaustion,
all 141 weight intervals, round-even boundaries, class/style precedence and
strict-codec failures. Separate audit compared an additional 4096 public
seeds, exact canonical bytes, digests and uppercase convenience parsing with
the independent Python oracle. No expectations came from production Go.
The staged parser fuzz run completed 430485 executions without failure.

Integrated CLI acceptance compared all 50 manifests and digests with the
independent corpus. A separate review compared 100 successful legacy DNA
invocations against the pre-change binary byte-for-byte, including original
help/error behavior, and checked ten invalid preview cases for empty stdout.
Evidence is retained in workspace `set33-checks.json` and
`set33-integrated-review.json`. Required node smoke, frozen release binaries,
seed promotion and live observation are recorded separately when complete.

## Preserved behavior and remaining work

No consensus, wallet derivation, receipt, mining job, birth or ownership rule
changes. This is an offline library/CLI, not a new desktop UI or an official
appearance assignment. Current jobs bind DNA version only. Official assignment
needs an explicit issuance/version contract or immutable committed mapping;
existing receipts cannot silently acquire new canonical traits.

Portrait rendering, admitted roster/history, aging/current condition, fights,
league and marketplace remain separate work. The candidate physical/style
mix is measured tuning evidence, not calibrated gameplay. The existing Mac
pilot remains the separately validated set-32 artifact. D74 requires Mike's
thorough joint design review before friends/tester or public product release.
