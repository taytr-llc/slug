#!/bin/sh
# Copyright (c) 2026 The SLUG developers. ISC license.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
root=$(CDPATH= cd -- "$here/.." && pwd)
output=${1:-"$root/../artifacts/set22"}
case "$output" in /*) ;; *) printf '%s\n' 'Build output must be an absolute directory.' >&2; exit 1;; esac
if [ "$(uname -s)" != Darwin ]; then printf '%s\n' 'This script builds the native macOS bundle on macOS.' >&2; exit 1; fi
mkdir -p "$output"
stage=$(mktemp -d "$output/.slug-build.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
bundle="$stage/SLUG Local Pilot.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources/bin"
# Explicit tags override GOFLAGS, so a developer test profile cannot leak into
# the bundled node/helper. The GUI production tag belongs only to Wails.
chain_cache=${SLUG_CHAIN_GOMODCACHE:-$(go env GOMODCACHE)}
export GOFLAGS=
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS='-O2 -g -mmacosx-version-min=13.0'
export CGO_LDFLAGS='-mmacosx-version-min=13.0'
export GOCACHE=${GOCACHE:-/private/tmp/slug-go-build}
export GOMODCACHE=${GOMODCACHE:-/private/tmp/slug-desktop-modcache}
(
 cd "$root"
 GOMODCACHE="$chain_cache" GOWORK="$root/go.work" CGO_ENABLED=0 go build -trimpath -tags= -o "$bundle/Contents/Resources/bin/slugd" .
)
(
 cd "$here"
 GOWORK="$here/go.work" CGO_ENABLED=0 go build -trimpath -tags= -o "$bundle/Contents/Resources/bin/slugpilot" ./cmd/slugpilot
 GOWORK="$here/go.work" CGO_ENABLED=1 go build -trimpath -tags=production -ldflags '-extldflags=-mmacosx-version-min=13.0' -o "$bundle/Contents/MacOS/SLUGPilot" .
)
if [ "$("$bundle/Contents/Resources/bin/slugd" --version)" != 'slugd version 0.27.0' ]; then
 printf '%s\n' 'Bundled node is not the expected normal production PoW build.' >&2; exit 1
fi
cat > "$bundle/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>local.slug.gym.pilot</string>
<key>CFBundleName</key><string>SLUG Local Pilot</string>
<key>CFBundleDisplayName</key><string>SLUG Local Pilot</string>
<key>CFBundleExecutable</key><string>SLUGPilot</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>0.1.0</string>
<key>CFBundleVersion</key><string>22</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSUIElement</key><false/>
</dict></plist>
PLIST
plutil -lint "$bundle/Contents/Info.plist"
codesign --force --sign - "$bundle/Contents/Resources/bin/slugd"
codesign --force --sign - "$bundle/Contents/Resources/bin/slugpilot"
codesign --force --sign - "$bundle/Contents/MacOS/SLUGPilot"
codesign --force --sign - "$bundle"
codesign --verify --deep --strict "$bundle"
final="$output/SLUG Local Pilot.app"
if [ -e "$final" ]; then mv "$final" "$output/SLUG Local Pilot.previous.$(date +%Y%m%d-%H%M%S).app"; fi
mv "$bundle" "$final"
{
 printf 'Source base: '; git -C "$root" rev-parse HEAD
 printf 'Go: '; go version
 printf 'Platform: '; uname -sm
 printf '%s\n' 'Native macOS build; ad hoc signed, not notarized.' 'Deployment target 13.0; older OS runtime compatibility is not proven by compilation.'
 shasum -a 256 "$final/Contents/MacOS/SLUGPilot" "$final/Contents/Resources/bin/slugpilot" "$final/Contents/Resources/bin/slugd"
} > "$output/BUILD.txt"
printf '%s\n' "$final"
