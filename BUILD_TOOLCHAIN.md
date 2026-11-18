# Candidate Go build profile

Set 48 locally qualified `slug-go1274-no-cse-v1` for candidate application checks:
Go 1.27.4 with `-gcflags=all=-d=ssa/generic_cse/off,ssa/lowered_cse/off`.
`build-profile.json` records the exact profile. This is a temporary workaround,
not a claim that the upstream compiler is fixed or that production launch is
approved. Normal Mac package and deployment acceptance remain pending.

The original two-result reservation test exposed a caller-retained key changing
after garbage collection under the default compiler. A separate program also
reproduced the problem without SLUG dependencies. The official
[Go issue #82026](https://github.com/golang/go/issues/82026) describes the same
local-array lifetime problem. No released fix has been verified. Original
optimized and race failures remain preserved; the test assertions were not
weakened to obtain a passing result.

Disabling all optimization passed that regression, but failed the existing
25-second full-capacity startup deadline at 25.059666084 seconds. That option
was rejected. Disabling the two CSE passes instead passed ten repetitions of
the original application regression. Full-capacity selected metadata took
228–241 ms against 500 ms, and parent startup took 11.299 seconds against
25 seconds in the preliminary diagnostic. Final-profile gates measured
228–242 ms and 11.16 seconds, respectively. Normal/race, installer recovery,
all-module and isolated actual-chain checks passed. Six actual proof exports
and one positive/twelve negative verifier controls passed independently.
These measurements are local candidate evidence, not universal
compiler correctness or platform-performance guarantees.

Use `sh scripts/candidate-go.sh build ...` or
`sh scripts/candidate-go.sh test ...` for this profile. The wrapper requires
the exact local Go patch, ignores persisted Go configuration and inherited
compiler overrides, and injects the recorded compiler setting. Shipping
builders check the embedded setting on every executable. They preserve normal
proof of work and exclude test-clock constructors. Acceptance reports also bind
the wrapper/profile bytes, compiler/linker hashes, exact commands, source files,
executable hashes and embedded build information.

Plain `go test` remains an ordinary unqualified diagnostic. The explicitly
labelled minimum-Go CI compatibility job is separate from candidate-profile
acceptance. A compiler upgrade or removal of this workaround requires fresh
original-regression, normal/race, capacity/startup, actual-chain and package
evidence before changing the profile. Keep earlier failures and do not relax
runtime limits or reference expectations to make a build pass.
