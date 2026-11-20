#!/bin/sh
# Copyright (c) 2026 The SLUG developers. ISC license.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
root=$(CDPATH= cd -- "$here/.." && pwd)
profile_go="$root/scripts/candidate-go.sh"
export GOENV=off GOTOOLCHAIN=local GOFLAGS= GOEXPERIMENT=
sh "$profile_go" check
output=${1:-"$root/../artifacts/set74-remote"}
case "$output" in /*) ;; *) printf '%s\n' 'Build output must be an absolute directory.' >&2; exit 1;; esac
if [ "$(uname -s)" != Darwin ]; then printf '%s\n' 'This script builds the native macOS bundle on macOS.' >&2; exit 1; fi
mkdir -p "$output"
stage=$(mktemp -d "$output/.slug-build.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
bundle="$stage/SLUG Remote Pool Pilot.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources/bin"
sh "$here/scripts/build-icons-macos.sh" "$bundle/Contents/Resources/SLUG.icns"
# Explicit tags exclude diagnostic PoW/clock. Both executables require CGO1
# for the retained-descriptor Darwin ACL check. No local node is bundled.
export GOFLAGS=
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS='-O2 -g -mmacosx-version-min=13.0'
export CGO_LDFLAGS='-mmacosx-version-min=13.0'
export GOCACHE=${GOCACHE:-/private/tmp/slug-go-build}
export GOMODCACHE=${GOMODCACHE:-/private/tmp/slug-desktop-modcache}
(
 cd "$here"
 GOWORK="$here/go.work" CGO_ENABLED=1 sh "$profile_go" build -trimpath -tags= -o "$bundle/Contents/Resources/bin/slugpilot" ./cmd/slugpilot
 GOWORK="$here/go.work" CGO_ENABLED=1 sh "$profile_go" build -trimpath -tags=production -ldflags '-extldflags=-mmacosx-version-min=13.0 -X main.remoteBuild=1' -o "$bundle/Contents/MacOS/SLUGPilot" .
)
# Verify both exact artifacts; no diagnostic constructor may be included.
: > "$stage/build-info-all.txt"
for binary in "$bundle/Contents/Resources/bin/slugpilot" "$bundle/Contents/MacOS/SLUGPilot"; do
 sh "$profile_go" verify-binary "$binary" > "$stage/build-info.txt"
 cat "$stage/build-info.txt" >> "$stage/build-info-all.txt"
 if ! rg -q "CGO_ENABLED=1" "$stage/build-info.txt"; then
  printf '%s\n' 'Remote protected-file artifacts require CGO1 on macOS.' >&2; exit 1
 fi
 if rg -q 'slugtestclock|slugtestpow' "$stage/build-info.txt"; then
  printf '%s\n' 'A diagnostic build tag reached a release artifact.' >&2; exit 1
 fi
 go tool nm "$binary" > "$stage/symbols.txt"
 if rg -q 'ForTestClock|UseOwnedCalendarOffsetForTest|UseCalendarClockForTest|UseOwnedClockForTest|UseAuthorityClockForTest' "$stage/symbols.txt"; then
  printf '%s\n' 'A test-clock constructor reached a release artifact.' >&2; exit 1
 fi
done
cat > "$bundle/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>local.slug.gym.remotepoolpilot</string>
<key>CFBundleName</key><string>SLUG Remote Pool Pilot</string>
<key>CFBundleDisplayName</key><string>SLUG Remote Pool Pilot</string>
<key>CFBundleExecutable</key><string>SLUGPilot</string>
<key>CFBundleIconFile</key><string>SLUG.icns</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.6.0</string>
<key>CFBundleVersion</key><string>74</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSUIElement</key><false/>
</dict></plist>
PLIST
plutil -lint "$bundle/Contents/Info.plist"
codesign --force --sign - "$bundle/Contents/Resources/bin/slugpilot"
codesign --force --sign - "$bundle/Contents/MacOS/SLUGPilot"
codesign --force --sign - "$bundle"
codesign --verify --deep --strict "$bundle"
final="$output/SLUG Remote Pool Pilot.app"
if [ -e "$final" ]; then mv "$final" "$output/SLUG Remote Pool Pilot.previous.$(date +%Y%m%d-%H%M%S).app"; fi
mv "$bundle" "$final"
cp "$root/build-profile.json" "$output/BUILD-PROFILE.json"
cp "$stage/build-info-all.txt" "$output/BUILD-INFO.txt"
{
 printf 'Chain source: '; git -C "$root" rev-parse HEAD
 printf 'Chain tracked changes: '; git -C "$root" diff HEAD --stat
 printf 'Wallet source: '; git -C "$root/../slugwallet" rev-parse HEAD
 printf 'Wallet tracked changes: '; git -C "$root/../slugwallet" diff HEAD --stat
 printf 'Wallet untracked paths: '; git -C "$root/../slugwallet" ls-files --others --exclude-standard
 printf 'Chain untracked paths: '; git -C "$root" ls-files --others --exclude-standard
 printf '%s\n' 'Build profile: slug-go1274-no-cse-v1 (candidate_engineering; qualification recorded separately).'
 printf 'Build profile SHA256: '; shasum -a 256 "$output/BUILD-PROFILE.json"
 printf 'Go: '; go version
 printf 'Platform: '; uname -sm
 printf '%s\n' 'Explicit remote bundle: GUI and helper CGO1; no node, wallet, config or credentials bundled.' 'Launch requires --candidate-remote-pool and all six selected paths; no Finder default account creation.' 'Native macOS build; ad hoc signed, not notarized.' 'Deployment target 13.0; older OS runtime compatibility is not proven by compilation.'
 shasum -a 256 "$final/Contents/MacOS/SLUGPilot" "$final/Contents/Resources/bin/slugpilot"
} > "$output/BUILD.txt"
printf '%s\n' "$final"
