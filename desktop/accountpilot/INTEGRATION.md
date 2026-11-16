# Account pilot acceptance

The normal-PoW gate uses only the independently published `zero256` and
`incrementing256` fixtures with isolated loopback nodes. It requires a normal
activation-62 node that supports the restricted `--localwalletpilot` option.
It never connects to the public seed or reads a real wallet.

```sh
SLUG_DESKTOP_NODE=/absolute/path/to/frozen/slugd \
  ./accountpilot/run-integration.sh
```

Run from `desktop`, or invoke the script by its full path. It copies the node
executable once, records its SHA256, verifies the independent Python fixtures,
and runs the explicitly tagged Go test. Optional `SLUG_ACCOUNT_TEST_ARTIFACTS`
selects an existing absolute directory for retained public-fixture runtime
artifacts and a compact `result.json`; otherwise Go uses temporary directories.
The local node configuration contains disposable RPC credentials and is not
printed or published in the result.

The test checks two distinct wallet/gym identities against the independent
recovery oracle, login initialization without mining, one actual wallet-paid
normal-PoW admission and automatic signature, retained native backup export and
file-only verification, logout/relogin/restart identity preservation, verified
reset with a 24-hour export hold, permitted mining during that hold after the
original identity is reproved, old-session revocation, immutable historical
receipts, cross-account backup/receipt isolation, and refusal to regenerate a
missing established wallet. It then restores Alice from the actual retained
file with the original database inaccessible and different fixture passwords,
and reproduces the independent enrollment signature.

This bounded account gate does not repeat 100-block maturity or a funded spend.
Those remain covered by set 25's four-script recovery/spend gate and set 26's
wallet-paid desktop maturity, file-only restoration and confirmed second spend.
The new wallet signer tests separately cover both fixtures, encryption-password
change, reopen and file-only restore. A local fixture reset is not production
provider verification, a KMS design, a production derivation allocation, or a
consensus-level freeze of keys already held by a user.

Ordinary `go test ./accountpilot` covers provider input rejection, durable
authority identity, namespace isolation, and per-child account binding even
when two mining sessions from the same account store are simultaneously valid.
Store tests own exact hold boundaries, fresh-reset versus replay behavior,
clock rollback, commit/reset races and uncertain-write recovery. The pure
signature oracle deliberately does not claim live session or reset authority.
