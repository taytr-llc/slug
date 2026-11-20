# Small Set52 pool-share reference

Run `PYTHONDONTWRITEBYTECODE=1 python3 -B reference.py` to check the frozen JSON.
The default does not overwrite it. `--write` creates it exclusively.

Five declared headers use normal Argon2id v19 (262144 KiB, t=1, p=1, 32 bytes).
Python derives nonce-adjusted bytes, SHA256 duplicate keys and integer target
comparisons. The standalone x/crypto oracle imports no SLUG package. There is
no miner loop, runtime adapter, pool/session input, signing, node or database.

Four cases preserve the frozen ManifestJob candidate exactly except nonce:
coinbase-only 0, 1 and 0xffffffff, plus witness candidate 0. Each checks pool
target hash−1, hash and hash+1, retaining independent prospect/block flags.
The fifth uses the old independent Python constructors to derive a supplied
context with prospect/block/pool targets 1 and nonce 0. It misses all three and
has no candidate. It is modeled input, not evidence of actual node-valid issuance.

The first four-case generation remains in the external preparation directory
`set52-pool-share-reference/before-modeled-miss/`; the final
five-case generation repeats those four fixed headers. Neither run searches.
The JSON and producer freeze precedes any new ManifestWork runtime code.
