# SLUG local gym pilot

This new Go desktop module is a Wails v3.1.0 shell around the SLUG pilot
supervisor. It does not use Paymetheus. The longer term gym connects to
`slugd` and the planned `btcwallet` fork; this milestone has no wallet, account,
seed import/export, production credentials, or valuable assets.

The bundled supervisor owns two isolated loopback normal-PoW nodes, a local
v2 prospect authority, the durable worker outbox, and a **public fixture signer**.
It uses an explicit all-`ff` prospect target for local demonstration. These
prospects and coins belong only to the disposable local chain. The app never
connects these nodes to the public seed, and opening the app does not mine.

Use **Start mining** to begin. **Stop mining** cancels new work and drains any
started hash and durable writes. Closing the window hides it; the SLUG tray
menu shows the gym or controls mining. **Quit** closes the helper's control
pipe and waits for the helper, worker, and both child nodes to finish. A failed
shutdown is reported rather than discarding retained data.

The default data location is `os.UserConfigDir()/SLUG/LocalPilot` (on macOS,
`~/Library/Application Support/SLUG/LocalPilot`). Both chain stores, the receipt
registry, and the outbox persist. Missing established data fails closed; no
automatic reset erases evidence. For isolated verification, supply an absolute
`--data-dir` argument or `SLUG_PILOT_DATA_DIR`. Do not select a wallet directory.

The only webview methods are `main.Gym.Snapshot`, `Start`, `Stop`, and
`ExportReceipt(sequence)`. The last opens a native save dialog for a public
receipt and writes atomically with mode 0600. The frontend cannot supply paths,
process commands, or keys. Embedded assets have a restrictive CSP and no remote
frontend override. Wails v3.1.0 has no desktop navigation-decision hook used here;
there are no external links or URL inputs. This is not a claim that arbitrary
untrusted web content can safely be loaded into the window.

## Build on macOS

Go 1.27 and the macOS command-line SDK are required. The desktop has its own
`go.work`; the root chain workspace is unchanged. The pinned module cache used
during development is `/private/tmp/slug-desktop-modcache`; override `GOMODCACHE`
if desired.

```sh
cd desktop
./scripts/build-macos.sh /Users/taytrrip/slug/artifacts/set22
```

The script builds the normal node and helper with explicit empty build tags,
the Wails shell with only its `production` tag, and bundles all three binaries
in `SLUG Local Pilot.app`. It clears inherited `GOFLAGS`. Native CGO and linker
flags target macOS 13.0; this proves compilation on the build host, not runtime
compatibility with every older macOS release. The bundle is ad hoc signed with
no signing account and is **not notarized**. A previous artifact is retained
when rebuilding. `BUILD.txt` records the source base and binary SHA-256 hashes.

Windows runtime behavior is unverified; this milestone's delivered bundle is
macOS. No installer, auto-update service, startup item, or public mining mode is
installed.

## Checks

```sh
GOMODCACHE=/private/tmp/slug-desktop-modcache go test -race ./internal/bridge ./pilot
MACOSX_DEPLOYMENT_TARGET=13.0 \
CGO_CFLAGS='-O2 -g -mmacosx-version-min=13.0' \
CGO_LDFLAGS='-mmacosx-version-min=13.0' \
GOMODCACHE=/private/tmp/slug-desktop-modcache go test .
node frontend/app.test.mjs
```

Pilot tests reserve local loopback ports. Bridge tests use a fixture subprocess
and cover malformed/oversized responses, correlation, timeout, and EOF drain.
Native launch, Start/Stop, prospect rendering, save-dialog cancellation/export,
application-menu controls, Quit during mining and saved-roster reopening were
verified on the Mac build. Window-close automation timed out and subsequent
targeted observations reopened the window; hidden-window visibility and the
system-tray Show action remain unverified interactively. See
`../CHANGESET_22_NOTES.md` for the full evidence and limits.
