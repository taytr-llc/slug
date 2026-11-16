# SLUG local account gym pilot

This new Go desktop module is a Wails v3.1.0 shell around the SLUG supervisor.
It uses the separate `taytr-llc/slugwallet` fork through its authenticated,
loopback btcwallet RPC surface for payout addresses, balances and history.
Paymetheus is not the base. Private gym signing and recovery operations stay
inside the Go helper.

Set 27 adds explicit Demo A/B accounts backed by the independently published
`zero256` and `incrementing256` recovery fixtures. Each has one retained personal
gym, wallet, private normal-PoW chain, prospect authority and durable outbox.
All keys are public fixtures; rewards have no value. The local mock provider is
not real signup or a production custody service. There is no public-seed option.

Choose **Sign in as Demo A/B**. First login initializes the wallet and enrolls
its public identity with an automatically signed, single-use account challenge.
Initialization obtains no mining job, hashes no candidate and broadcasts nothing.
Choose **Start mining** explicitly. **Stop mining** drains current work. Sign-out
revokes the login and its mining sessions, then drains and closes that gym.
Reopening requires another fixture sign-in and retains the same account/wallet.
Closing the window hides it; **Quit** cancels and joins owned work and processes.

**Simulate verified password reset** exercises D72 using the mock provider.
It revokes existing sessions, preserves the wallet and gym, and proves the
original key again before enabling mining. For 24 hours, the account authority
blocks withdrawals, fighter transfers, recovery export, recovery-authority
changes, marketplace purchases and listings. Viewing and existing-wallet mining
remain available. Another distinct reset restarts the hold; exact event replay
retains its original result. The demo does not implement withdrawals, transfers
or marketplace actions, but their capability checks are covered by store tests.
The service retains recovery authority: backup export is not exclusive migration.
The hold cannot freeze independent copies of previously exported wallet keys.

With mining stopped and a fresh authorized sign-in, **Export recovery backup**
opens a native save dialog. The helper exports actual retained recovery material
and cursors, writing atomically with mode0600. The file contains 24 English BIP39
words and versioned SLUG metadata, with no extra phrase passphrase. Its contents
never cross the helper response pipe or enter the webview. The approved reset
hold blocks this operation in the account layer as well as the UI.

**Verify backup** chooses a native file and asynchronously restores it into an
empty scratch wallet. It compares identity, payout, cursors, funds and signing;
mining stays stopped and Quit cancels/drains verification. It does not replace
the retained wallet or restore off-chain prospect receipts. Only the current
account's published fixture identity is accepted. Public receipt export is separate.

## Retained data and boundaries

Default storage is `os.UserConfigDir()/SLUG/LocalAccountPilot`. Set-22 `LocalPilot`
and set-26 `LocalWalletPilot` remain separate and cannot be adopted implicitly.
Account schema1 stores public identity, hashed sessions, challenges, reset/audit
records and the durable hold. Each child's v3 manifest binds account/fixture,
network/genesis/activation62, payout and immutable wallet/gym identity. Missing,
foreign or incomplete established storage fails closed without substitution.
Use an absolute `--data-dir` or `SLUG_PILOT_DATA_DIR` for isolated checks.

The webview exposes only `Snapshot`, `Start`, `Stop`, `ExportReceipt(sequence)`,
`ExportRecovery()`, `VerifyBackup()`, `LoginFixture(allowlistedLabel)`, `Logout()`
and `ResetFixtureAccount()`. It cannot provide issuer/subject, reset assurance,
clock, session credentials, keys, native paths or RPC requests. Native dialogs
choose paths; both the full account root and child data are protected. The
wallet RPC retains its minimal allowlist and ephemeral private credentials.
Embedded assets use a restrictive CSP and no remote frontend override.

Account authorization is held across final registry guards and synchronized
writes. Each child admission service pins its account ID; another account's
valid session cannot use that registry. Logout/reset does not erase original
births, and authorized historical recovery does not renew eligibility. See
`../GYM_ACCOUNTS.md` for exact bytes, sessions, holds and production limits.

## Build on macOS

Go 1.27.3 or later and the macOS command-line SDK are required. Check out the
paired `slugwallet` topic beside `slug`; desktop has its own workspace covering
both repositories, while the ten-module chain workspace stays independent.

```sh
cd desktop
./scripts/build-macos.sh /Users/taytrrip/slug/artifacts/set29
```

The build clears inherited `GOFLAGS`, builds the node/helper with empty tags,
and applies only Wails' `production` tag to the shell. `SLUG Account Pilot.app`
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
GOMODCACHE=/private/tmp/slug-desktop-modcache go test -race ./internal/bridge ./pilot ./accountpilot
MACOSX_DEPLOYMENT_TARGET=13.0 \
CGO_CFLAGS='-O2 -g -mmacosx-version-min=13.0' \
CGO_LDFLAGS='-mmacosx-version-min=13.0' \
GOMODCACHE=/private/tmp/slug-desktop-modcache go test .
node --test frontend/app.test.mjs
```

Pilot tests reserve loopback ports. The separate normal-PoW integration launcher
copies and hashes a frozen node binary before running mining, wallet and fresh
recovery checks. See `../CHANGESET_27_NOTES.md` for account acceptance evidence. Sets25/26 retain
funded recovery and four-script spending evidence; those do not substitute for
set27 account/session and native-login verification.

Set 29 adds an optional private `AccountConfig.AdmissionSigner` adapter for the
same automatic worker. It releases the account lease during the signing call,
then rechecks the original mining session and verifies the returned typed gym
signature. Failure does not fall back to the local wallet. This adapter has no
webview parameter or command-line switch and does not change the Account Pilot's
default local fixture wallet. The separate wallet vault acceptance exercises
gym-only signing during a reset hold, while full recovery export and wrapping-key
changes remain denied. It does not ship a new-device watch-only wallet or a
production identity/KMS connection.
