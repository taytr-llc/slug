// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import _ "embed"

// The app uses the requested original detailed logo. The separate
// monochrome fist is a template image: macOS uses its alpha mask for light/dark
// menu bars.

//go:embed icons/slug-app.png
var appIcon []byte

//go:embed icons/slug-menubar-template.png
var menuBarIcon []byte
