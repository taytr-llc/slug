# Set48 clarification 2 — closed result carrier admission

This addendum supplements contract freeze
`11c8322b53d79ddcfefea05f37433eb5807fb3a52541b0e467df314d7ce27e15`.
The original core and helper documents remain byte-identical.

The Result publisher admits exactly a73-byte carrier: eight ASCII bytes
`SLUGRSLT`, version byte1, a nonzero32-byte ResultKey and a nonzero32-byte
StatementID. All-zero commitment halves are invalid, preserving the existing
seed adapter's closed commitment convention. No trailing bytes, other version,
seed prefix or missing commitment is admitted by Build, Submit, Attest or
standalone Publication validation.

The full result Record additionally recomputes both identities from its exact
signed statement and fixed authority. Nonzero structural admission never
establishes that a supplied commitment matches a saved result. An equation-only
crypto helper may expose malformed-input construction for negative fixtures,
but its output cannot be labeled an accepted adapter/publication expectation.

This clarifies purpose-specific structural validation before reference vectors
are frozen. It changes no JSON shape, byte cap, old seed behavior, chain
consensus or player policy. Bind it in the reference input index and review.
