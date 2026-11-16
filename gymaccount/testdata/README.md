# Public account enrollment fixtures

`enrollment_v1.json` contains four independent vectors: `enroll` and `recover`
for the published `zero256` and `incrementing256` recovery wallets. Every
`test_only_private_key` is deliberately public, derived from those published
phrases; none is an operator key or suitable for real funds.

Regenerate and check without a node, wallet, network, or third-party Python
package:

```sh
python3 gymaccount/testdata/reference_enrollment.py --check
python3 gymaccount/testdata/reference_enrollment.py --generate
```

The reference derives the BIP39 seed with the empty extra passphrase, the
standard BIP32 root, and hardened gym path `m/1397511495'/1'/0'`. It compares
the resulting public tuple to the independent wallet fixtures, then constructs
the canonical binary enrollment message and signs its single SHA256 digest
using independent secp256k1/RFC6979 arithmetic and strict low-S DER. The exact
payload, digest, signature and mathematically valid high-S negative signature
are retained. It was written and run before inspecting the Go digest code;
expected values were never obtained from that implementation.

The identity source is the paired wallet repository's
`recovery/testdata/recovery_v1.json` and its independent Python reference. This
script embeds the two published phrases and public tuples so regeneration does
not require another checkout. Curve and signature math follows the independent
reference already used in `gymauth/testdata/reference_authorizations.py`.

Fixture IDs are deterministic synthetic values and generations intentionally
exceed 32 bits. The 60-second challenge interval tests timestamp encoding; it
is not the reset hold. These signatures prove only the signed key-control
statement. Live identity-provider assurance, stored challenge consumption,
generation, expiry and the approved 24-hour reset hold are separate service
checks.
