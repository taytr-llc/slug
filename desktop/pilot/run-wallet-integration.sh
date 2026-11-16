#!/bin/sh
# Public fixtures only. No real wallet, seed, key or public peer is used.
set -eu
cd "$(dirname "$0")/.."
: "${SLUG_DESKTOP_NODE:?Set SLUG_DESKTOP_NODE to an explicit normal-PoW activation-62 slugd executable}"

# Freeze the executable for this run. A parallel release rebuild must not
# silently change the node used by later phases of this integration gate.
fixture_bin=$(mktemp -d "${TMPDIR:-/tmp}/slug-wallet-desktop-bin.XXXXXX")
trap 'rm -rf "$fixture_bin"' EXIT HUP INT TERM
python3 - "$SLUG_DESKTOP_NODE" "$fixture_bin/slugd" <<'PY'
import hashlib
import os
import pathlib
import sys

source, destination = map(pathlib.Path, sys.argv[1:])
if not source.is_absolute() or not source.is_file():
    raise SystemExit("explicit normal node executable required")
data = source.read_bytes()
destination.write_bytes(data)
os.chmod(destination, 0o500)
digest = hashlib.sha256(data).hexdigest()
if hashlib.sha256(source.read_bytes()).hexdigest() != digest:
    raise SystemExit("node executable changed while freezing fixture build")
print("Frozen normal node SHA256:", digest)
PY
export SLUG_DESKTOP_NODE="$fixture_bin/slugd"
export GOFLAGS=
export GOWORK="$PWD/go.work"
python3 ../../slugwallet/recovery/testdata/reference_recovery.py --check
python3 ../../slugwallet/integration/testdata/reference_recovery_highwater.py --check
go test -tags=desktopintegration -run '^TestWalletPilot(Recovery|EarlyClose|StaleTip|DefaultGBTBarrier)NormalPoW$' \
  -count=1 -timeout=20m -v ./pilot
