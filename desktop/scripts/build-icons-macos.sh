#!/bin/sh
# Copyright (c) 2026 The SLUG developers. ISC license.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output=${1:?Supply the absolute new ICNS output path.}
case "$output" in /*.icns) ;; *) printf '%s\n' 'Icon output must be an absolute .icns path.' >&2; exit 1;; esac
if [ "$(uname -s)" != Darwin ]; then printf '%s\n' 'Icon packaging requires macOS.' >&2; exit 1; fi
if [ -e "$output" ] || [ -L "$output" ]; then printf '%s\n' 'Icon output already exists.' >&2; exit 1; fi
stage=$(mktemp -d "$(dirname -- "$output")/.slug-icon.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
iconset="$stage/SLUG.iconset"
mkdir "$iconset"
# Standard macOS 1x/2x representations; only resampling, no redraw or cropping.
for size in 16 32 128 256 512; do
 sips -z "$size" "$size" "$here/icons/slug-app.png" --out "$iconset/icon_${size}x${size}.png" >/dev/null
 retina=$((size * 2))
 sips -z "$retina" "$retina" "$here/icons/slug-app.png" --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$stage/SLUG.icns"
mv "$stage/SLUG.icns" "$output"
