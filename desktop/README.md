# SLUG local wallet gym pilot

This new Go desktop module is a Wails v3.1.0 shell around the SLUG supervisor.
It uses the separate `taytr-llc/slugwallet` fork through its authenticated,
loopback btcwallet RPC surface for payout addresses, balances and history.
Paymetheus is not the base. Private gym signing and recovery operations stay
inside the Go helper.

The helper owns two isolated normal-PoW nodes, a local v2 prospect authority,
a durable worker outbox and the published `zero256` recovery-wallet fixture.
The all-`ff` prospect target is for demonstration only. All keys in this pilot
are public test fixtures; coins and prospects belong to its disposable local
chain. There is no public-seed option, real-wallet issuance, hosted login or
valuable asset custody.

Use **Start mining** to initialize the wallet and discover prospects. Opening
or reopening the app never mines. The first Start allocates one legacy reward
address through the wallet RPC, then retains that address and the wallet-derived
gym identity across restarts. Rewards appear as immature until 100 blocks have
elapsed. **Stop mining** drains the current hash, wallet work and durable writes.
Closing the window hides it; **Quit** cancels and joins owned work and processes.

With mining stopped, **Save recovery file** opens a native save dialog. The
helper exports its current encrypted retained recovery material and derivation
cursors, then writes the selected file atomically with mode 0600. The file
contains the approved 24-word English phrase and SLUG metadata, with no extra
phrase passphrase. These are this public test wallet's credentials only.
Recovery contents never cross the helper response pipe or enter the webview.
Public prospect receipts have their own separate export action.

**Verify backup** opens a native file chooser and starts an asynchronous restore
into a fresh scratch wallet. It verifies the published fixture identity, payout,
derivation cursors, funds and signing against the retained local chain. Mining
remains stopped during verification; progress is polled and Quit cancels and
joins it. The existing wallet and prospect evidence are preserved. A wallet
backup does not restore off-chain prospect receipts or transfer fighters.
Only this published fixture profile is accepted; this is not a general seed
import or a reviewed real-custody reveal interface.

## Retained data and boundaries

The default is `os.UserConfigDir()/SLUG/LocalWalletPilot` (on macOS,
`~/Library/Application Support/SLUG/LocalWalletPilot`). The set-22 `LocalPilot`
directory is preserved and cannot be silently upgraded. The v2 manifest binds
network, genesis, block-62 activation, wallet/gym identity, payout and authority.
Fresh chain directories must be empty; pre-upgrade cached chains cannot be
adopted. Missing or incomplete established data fails closed without replacement.
For isolated checks, use an absolute `--data-dir` or `SLUG_PILOT_DATA_DIR`.

The webview has only `Snapshot`, `Start`, `Stop`, `ExportReceipt(sequence)`,
`ExportRecovery()` and `VerifyBackup()` on `main.Gym`. It cannot supply paths,
process commands, keys or RPC requests. The native shell chooses paths; the
helper checks sizes, symlinks and managed data boundaries. Its owned wallet RPC
uses ephemeral credentials and a minimal copied method allowlist; secret,
import, signing, stop and node-passthrough RPCs are denied, as is WebSocket use.
Embedded assets have a restrictive CSP and no remote frontend override.

## Build on macOS

Go 1.27.3 or later and the macOS command-line SDK are required. Check out the
paired `slugwallet` topic beside `slug`; desktop has its own workspace covering
both repositories, while the ten-module chain workspace stays independent.

```sh
cd desktop
./scripts/build-macos.sh /Users/taytrrip/slug/artifacts/set26
```

The build clears inherited `GOFLAGS`, builds the node/helper with empty tags,
and applies only Wails' `production` tag to the shell. `SLUG Wallet Pilot.app`
targets macOS 13.0 and is ad hoc signed, **not notarized**. Compilation does not
prove runtime compatibility on every older OS. Previous artifacts are retained.
`BUILD.txt` records both repository revisions, dirty paths and binary hashes.
The helper checks actual activation metadata; the unchanged node version string
alone cannot prove the block-62 upgrade. Its owned nodes use the explicit
`--localwalletpilot` mode: connect-only literal loopback peers/listeners/RPC,
no DNS seeding or discovered outbound connections, proxy, UPnP or legacy checkpoint floor. Runtime template checks
require handshaken local peers, no advertised higher height and the same best
header/block tip. This replaces only the old-tip currentness heuristic so an
idle private chain can resume through the normal job/outbox miner. Full block
validation and production PoW remain unchanged; the seed does not use this mode.
The private wallet adapter likewise verifies identity, activation and coherent
local peers before allowing an old-tip rescan. No hidden bootstrap blocks are
mined during backup verification.

No installer, auto-update service, startup item or wallet Railway service is
installed. Windows runtime remains unverified.

## Checks

```sh
GOMODCACHE=/private/tmp/slug-desktop-modcache go test -race ./internal/bridge ./pilot
MACOSX_DEPLOYMENT_TARGET=13.0 \
CGO_CFLAGS='-O2 -g -mmacosx-version-min=13.0' \
CGO_LDFLAGS='-mmacosx-version-min=13.0' \
GOMODCACHE=/private/tmp/slug-desktop-modcache go test .
node --test frontend/app.test.mjs
```

Pilot tests reserve loopback ports. The separate normal-PoW integration launcher
copies and hashes a frozen node binary before running mining, wallet and fresh
recovery checks. See `../CHANGESET_26_NOTES.md` for exact evidence and remaining
acceptance work; set-22 GUI evidence does not substitute for this wallet build.
