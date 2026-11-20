# Explicit existing checkpoint custody boundary

The existing official checkpoint private seed stays on the Mac. Root never
reads it, and it is never exported to the pool host. Public pin v2 still names
the independently expected checkpoint public key. The online owner loads only
admission/publication/cutoff keys and imports/verifies exact signed public
checkpoint envelopes. Optional separately configured local checkpoint tooling
does not authorize online official signing. Any new online official-key
delegation requires a later explicit user decision; default is no delegation.

This corpus derives only published synthetic and known-fixture keys from fixed
public recipes. Its checkpoint signatures test the unchanged byte grammar, not
access to the official private key or production checkpoint issuance.

Recorded from root's explicit source-preparation instruction, November 20, 2026.
