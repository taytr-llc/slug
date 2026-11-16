# ADR-004: Exercise the desktop miner on a disposable local chain

**Status:** Implemented for the local macOS pilot

**Date:** November 16, 2026

**Context:** Mike authorized continued work toward the desktop mining pilot.

## Proposed integration

The initial prototype uses a native Wails v3.1.0 shell with embedded web assets and the existing Go
worker. Wails supports native windows and tray integration while keeping mining
and node code in the existing language. Pin the dependency and keep the desktop
in its own module/workspace; the ten-module chain workspace and Railway node
build remain independent. The initial Mac bundle includes the normal-profile
node and a small supervisor alongside the UI executable.

Mike confirmed on November 16 that the gym shell is new work in Go against the
slugd and btcwallet RPC surfaces, as specified by the gym client workstream.
There is no existing desktop application to adopt, and Paymetheus is not the
base. btcwallet is a separate repository to fork when the wallet workstream
starts. This slice uses slugd RPC through the existing node adapter; the narrow
supervisor boundary leaves wallet RPC and custody integration for that workstream.

The pilot visibly uses a disposable local chain and public fixture gym identity.
Two newly initialized nodes peer only over explicit loopback addresses. A local
v2 authority and the production worker perform actual Argon2id mining, immediate
block broadcast, automatic admission and durable receipt recovery. There is no
public-node endpoint or real-wallet input in this pilot. Mining starts only on
an explicit Start action. Closing the window keeps the menu-bar/tray app alive;
Quit drains the worker and stops its owned processes.

A separate Go supervisor owns nodes, authority and storage. The desktop controls
it through bounded JSON lines over inherited pipes. EOF cancels and drains it if
the UI exits unexpectedly. Preserve both node datadirs, registry and outbox across
restarts. Pin the local authority port/origin in pilot metadata; a busy port is
an explicit startup failure, never a silent authority change. UI methods expose
only controls, public status, roster summaries and native public-receipt export.

## Consequences

This provides a concrete native start/mining/prospect/restart loop before real
account custody is provisioned. It does not satisfy D48's hosted wallet, normal
login or tested full seed export. Fixture identity and receipt export must never
be represented as those features. Production prospect calibration, account/key
recovery, public authority operations and release signing/notarization remain
separate work. Windows cross-compilation is not a Windows runtime test.

The Mac build sets an explicit deployment target to avoid mixing SDK-default
object requirements. Current-host compilation alone does not establish support
for older macOS releases. Direct release signing uses Mike's developer accounts
later; this pilot may use ad-hoc local signing without accessing a private key.

## Sources

- [Wails v3.1.0 release](https://github.com/wailsapp/wails/releases/tag/v3.1.0)
- [Wails installation requirements](https://v3.wails.io/getting-started/installation/)
- [Wails system tray API](https://v3.wails.io/features/menus/systray/)
