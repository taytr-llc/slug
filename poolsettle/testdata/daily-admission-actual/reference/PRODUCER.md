# Set78 actual fixture producer — source only

`model.py` and `recipe.json` are frozen before execution; no expected JSON or Go
test has been generated. Root reviews the proposal and exact lock, copies it with
`approved_by: root`, then runs both existing Python3.14 and3.9 interpreters:

`PYTHON model.py --lock /absolute/producer-lock.approved.json --out /absolute/fresh/reference.json`

Use fresh files and a60-second outer bound,1MiB output and64KiB logs. Require
byte-identical outputs and final source hashes unchanged; preserve any failure.
The model uses exclusive output creation and rechecks all inputs before writing.

Inputs are unchanged public independent Set76 setup, wallet, gym/crypto, shape,
model and output bytes plus the qualified Set78 capacity oracle/report. The
copied original model is loaded for its pure native transaction shape helper;
its main entry point is not called. The two copied public crypto sources derive
only the published scalar1/2 public identities and synthetic online public keys.
No signature function is called, no real key is loaded, no mined proof is made.

The two-account source/config/guard composition uses a fresh fixture namespace
and the exact frozen kernel capacities. Complete guard/binding bytes and IDs are
emitted. One fee is derived from native shape/fixed fee rate; actual B includes
that fee, while the controlled kernel's B excludes A and uses fee1000. Exact
Fraction arithmetic derives both lifetime earnings and outstanding report
fields, including full frontier conservation. Neither parent-calculated amounts
nor product quotes are used as the expected arithmetic oracle.

The output has only these scoped identities, reservation, actual/kernel
economics, report projections and phase/count requirements. It does not claim
runtime, session, PoW, expiry, funding, custody or cleanup qualification.
Standard dependencies: argparse, copy, fractions, hashlib, json, pathlib, runpy,
struct; copied helpers additionally use hmac/functools. No third-party modules.
