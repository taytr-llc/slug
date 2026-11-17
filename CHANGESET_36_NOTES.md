# Change set 36 — candidate appearance-bound mining jobs

Branch: `slug-manifest-jobs`, stacked on set35 implementation
`884fbea8b23ed76978f39c40b65d95f040467429`.

## What changed

The leaf `fighterdescriptor` package encodes the exact DNA, static manifest
and portrait bundle. Its strict canonical codec can archive unknown component
tuples, while the candidate resolver recognizes only the entire compiled
tuple. The production resolver has no activated entries. Archived contract
hashes identify immutable normative bytes, including their original status
paragraphs; mutable documentation paths are not identity inputs.

The separately typed `prospectjob.ManifestJob` binds that descriptor and an
authority namespace in the v2 job preimage. Every operational helper requires
the compiled candidate and matching DNA version. Descriptor parsing requires
canonical bytes; job transport accepts whitespace, field order and escaped
spellings but requires canonical decoded values. Both have explicit size,
depth, duplicate, required-field and scalar bounds. D68's five-minute duration
is unchanged; these pure helpers neither read a clock nor prove issuance.

`prospectwork.BuildManifest` constructs the corresponding unmined payable
candidate through the same private assembly routine as legacy `Build`. The
typed entry validates the new job before assembly. Both entries require normal
PoW configuration and preserve transaction/witness data, payout, template and
resource checks. Existing Job v1 and its strict readers remain separate.

`MANIFEST_JOBS.md` preserves the approved engineering contract verbatim (SHA256
`6a761c2e78081178e45c493953b78759f721ad3772a4e1fe684f38ec5fb0cd66`).
Its proposal-status text records provenance; implementation and validation
status are recorded here and in the workspace reports.

## Independent validation

The independent Python reference freezes two descriptors, seven jobs, three
complete block candidates and three authorization-v2 cases. It also covers
68 descriptor and 127 job transport cases, 59 structural boundaries,
39 constructor cases, 20 mutations, three normalizations and ten resolver
cases. It was derived from the approved contract and previous independent
fixtures, without reading runtime outputs. Python 3.9.6 and 3.14.3 regeneration
passed with varied timezone/hash seeds and unchanged original inputs.

The second descriptor is deliberately unsupported. Its mathematical
commitment differs while the DNA version stays fixed; operational job and
builder APIs must reject it. No synthetic catalog entry is added for a test.
Final staged normal/race/tagged suites, build/vet, both Python interpreters
and both focused parser fuzz runs passed with all 3959 captured files unchanged.
Root integration passed all ten module builds, normal/race/tagged checks of
the descriptor/job/builder and existing worker, vet, reference regeneration and
full formatting. The 30 integrated source/reference files exactly match the
frozen stage; its nonnormative verification log stays in the workspace.
The per-set normal-PoW smoke and deployment observation have separate reports.

## Preserved behavior and remaining work

No consensus, wallet, registry, service, worker/outbox, CLI or desktop activation.
Context v1, DNA v1, proof v2, authorization v2 and prospect IDs retain their
contracts. Candidate version1 still denotes the same DTO; it does not imply a
supported issuer. An authority namespace assertion does not authenticate its
issuer or establish birth, ownership or roster eligibility.

The next explicit store schema must durably pin issuance and preserve accepted
history before any official appearance binding is claimed. Old receipts must
never be enriched or reinterpreted through current configuration. Production
activation remains separate. Mike's portrait rendering check stays pending,
as does actual screen-reader acceptance. The D74 joint design review remains
required before release to friends, testers or the public.
