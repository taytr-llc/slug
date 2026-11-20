#!/bin/sh
# Complete normal module checks plus explicit test-PoW chain suites.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
mode=${1:-}
case "$mode" in unit|unit-race|unit-cover) ;; *) exit 2;; esac
profile=${GO_PROFILE:-candidate}
case "$profile" in candidate|compatibility) ;; *) exit 2;; esac

# Enumerate before filtering so a failed go list can never become an empty,
# successful test run. Only these two complete package families move profiles.
if [ "$mode" = unit ]; then
  packages=$(go list -tags=rpctest ./...)
else
  packages=$(go list -tags= ./...)
fi
normal=$(printf '%s\n' "$packages" | awk '
  !/^github.com\/taytr-llc\/slug\/(blockchain|integration)(\/|$)/ { printf "%s ", $0 }
')
test -n "$normal"
printf '%s\n' "$packages" | awk '
  $0 == "github.com/taytr-llc/slug/blockchain" { chain = 1 }
  $0 == "github.com/taytr-llc/slug/integration" { integration = 1 }
  END { if (!chain || !integration) exit 1 }
'
make GO_PROFILE="$profile" ROOT_UNIT_PACKAGES="$normal" "$mode"

profile_go() {
  if [ "$profile" = candidate ]; then
    sh "$root/scripts/candidate-go.sh" "$@"
  else
    go "$@"
  fi
}
# The independent 256 MiB production digest stays on the normal profile.
profile_go test -tags= -count=1 -timeout=2m -run '^TestProductionProofVector$' ./blockchain
case "$mode" in
  unit-race)
    make GO_PROFILE="$profile" CHAIN_TEST_FLAGS=-race test-chain
    ;;
  unit-cover)
    # Separate coverage files identify the incompatible test profile explicitly.
    profile_go test -tags=slugtestpow -count=1 -timeout=30m -covermode=atomic -coverprofile=coverage-chain-testpow.txt ./blockchain/...
    profile_go test -tags=slugtestpow,rpctest -count=1 -timeout=30m -covermode=atomic -coverprofile=coverage-integration-testpow.txt ./integration/...
    ;;
  unit)
    make GO_PROFILE="$profile" test-chain
    ;;
esac
