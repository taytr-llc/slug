# Public ownership implementation mappings

These additive mappings implement `PUBLIC_FIGHTER_OWNERSHIP.md` without changing its public signatures or wire objects.

The registry's checkpoint deduplication key hashes the inherited canonical SignedCheckpoint with the standard public-object hash and domain `SLUG fighter ownership checkpoint`. Its value is the first publication sequence encoded as exactly four big-endian bytes. Every public checkpoint entry has exactly one such index and both directions are audited. The index is bookkeeping, not proof of canonicality or finality.

Neutral signed DTOs live in `registry`; `fighterownership` aliases the public-facing names. Concrete replay, source, head and prepared-transfer capabilities remain in `fighterownership`, preventing an import cycle without adding verification flags or signer callbacks. Core creates chain/frame/cutoff signatures only after its retained concrete-source checks. Registry accepts externally signed entries only through its own exact prepared intent, checking signature, original pin/prefix and retained cutoff intent. The existing concrete registry publisher signs admission and checkpoint entries itself.

The independent Python corpus contains exact codec and conditional-model cases. Its convenience checks are not exhaustive Go validation rules; Go's closed typed codecs must separately reject wrong JSON types, nulls, aliases and bounds. Modeled branch hashes do not establish normal work or historical canonicality. The store accounting fixture is not a complete verified store.
