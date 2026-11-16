#!/bin/sh
# Private loopback nodes and published fixtures only. No real wallet or keys.
set -eu
cd "$(dirname "$0")/.."
: "${SLUG_DESKTOP_NODE:?Set SLUG_DESKTOP_NODE to an explicit normal-PoW local-wallet-pilot-capable slugd executable}"

fixture_bin=$(mktemp -d "${TMPDIR:-/tmp}/slug-account-node.XXXXXX")
trap 'rm -rf "$fixture_bin"' EXIT HUP INT TERM
python3 - "$SLUG_DESKTOP_NODE" "$fixture_bin/slugd" <<'PY'
import hashlib, os, pathlib, sys
source, destination = map(pathlib.Path, sys.argv[1:])
if not source.is_absolute() or not source.is_file():
    raise SystemExit("explicit normal node executable required")
data = source.read_bytes()
destination.write_bytes(data)
os.chmod(destination, 0o500)
digest = hashlib.sha256(data).hexdigest()
if hashlib.sha256(source.read_bytes()).hexdigest() != digest:
    raise SystemExit("node binary changed while freezing it")
print("Frozen normal node SHA256:", digest)
PY
export SLUG_DESKTOP_NODE="$fixture_bin/slugd"
export GOFLAGS=
export GOWORK="$PWD/go.work"
python3 ../gymaccount/testdata/reference_enrollment.py --check
python3 ../../slugwallet/recovery/testdata/reference_recovery.py --check
go test -tags=desktopintegration -run '^TestAccountPilotNormalPoW$' \
  -count=1 -timeout=5m -v ./accountpilot
