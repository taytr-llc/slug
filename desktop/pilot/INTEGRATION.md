# Wallet-backed desktop fixture gate

Build a normal `slugd` with activation62 and the explicit isolated-wallet-pilot
mode (set26 or later), then run from the chain checkout:

```sh
SLUG_DESKTOP_NODE=/absolute/path/to/slugd \
  ./desktop/pilot/run-wallet-integration.sh
```

The launcher freezes a private executable copy for the whole run, reports its
SHA256, checks the independently generated wallet recovery oracles, and uses
the separate desktop workspace. It clears inherited Go flags; no cheap-PoW
profile, public peer or real wallet is used. Missing explicit node selection
is an error. The test is tagged `desktopintegration` and is absent from ordinary
unit-test runs.

A short separate case issues Start followed immediately by Stop/Quit on a
fresh directory. It must drain real node/wallet initialization, then reopen
the identical ready fixture with zero hash attempts and no mined block.

A second short case explicitly constructs and submits one valid normal-PoW
fixture block with a timestamp older than 26 hours. Backup verification must
restore and synchronize that chain without changing its tip. Only a subsequent
explicit Start may resume mining. The case uses the isolated pilot's node mode
and real owned loopback peers; it does not weaken public node synchronization
rules or secretly generate a bootstrap block during recovery.
The same binary without isolated mode must still reject the old tip's template
request with the normal initial-download error. Older set25 binaries do not
understand the explicit pilot mode and are intentionally rejected at startup.

The gate starts the actual pilot controller with the published `zero256` wallet
fixture, observes explicit Start and normal-PoW mining through reward maturity,
then checks the worker's coinbase payout, gym signature, production proof and
durable receipt. Stop/Close/reopen must preserve payout, identity, attempts,
balances and receipt count without starting mining automatically.

It reopens the same fixture through the real restricted wallet HTTP adapter,
advances the legacy receive cursor to independent index25, and records actual
RPC balance, unspent outputs and transaction history. Private method counters
verify that ordinary address/fund/history operations used HTTP. After another
controller reopen, recovery export must contain current retained metadata with
cursor26, not a creation-time bundle. The helper's asynchronous backup verifier
must succeed without mining or returning recovery material in its snapshot.

For the separate restore proof, the test closes the controller and renames the
original wallet directory away from its managed path. A new wallet is restored
using only the exported file and different published fixture passwords. It
rescans the retained isolated chain and must reproduce exact mature balance,
UTXOs, transaction identities, index25 ownership, gym identity and the identical
automatic authorization for existing public evidence. A separately owned,
loopback-only test wallet RPC server then makes an actual confirmed spend. This
test server does not widen the desktop's restricted method allowlist or add a
send-payment UI.

The payout remains legacy receive0. This gate proves later retained cursor
export and restoration beyond gap20; set25 separately proves actual funding,
restoration and spending at index25 for all four supported scopes. Native
Save/Open dialogs, UI rendering and cancellation are separate GUI/bridge gates.
All test wallets, RPC listeners and node processes have checked shutdown.

This is a public-fixture integration, not hosted login, real custody, secure
recovery reveal, a production derivation allocation, imported-key export or
fighter transfer. Wallet backup does not reconstruct off-chain authority or
outbox records. Those stores remain separately retained.
