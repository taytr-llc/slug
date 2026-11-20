# Independent provision bytes

`reference-v1.json` is the unchanged independently produced corpus: 543,515 bytes,
SHA256 `8c3cde66c35b69dbc0f4daf516c271fb91358a988b591c363f65a106f25a401f`.
It contains six positives, 61 structured refusals and 18 raw Policy cases.
Python 3.14.3 and 3.9.6 produced identical bytes before Go consumed expectations.
The original producer source, immutable public inputs and approval lock are in
`reference/`; generation/reproduction reports preserve their original paths.
The material and wallet phrases in this corpus are published synthetic fixtures.
They must never become deployment defaults or inputs to the public entropy API.

The frozen grammar is in BYTES.md and lifecycle contract in CONTRACT.md. This
corpus proves bytes/derivations only; concrete filesystem, wallet and runtime
composition tests supply their own separately reported evidence.

Lifecycle contract erratum: the frozen CONTRACT.md names `wallet/wallet.db`.
The paired wallet implementation's public `wallet.WalletDBName` is
`slugwallet.db`; the concrete inventory and current operator documentation use
`wallet/slugwallet.db`. No derivation, serialized fixture or backup bytes changed.
The original contract remains preserved as an input to the published corpus.
