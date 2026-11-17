# Native icon sources

`slug-app.png` is an unchanged copy of `slug-app-generated.png` (1254 × 1254,
alpha), the simplified fist with SLUG lettering Mike requested on November 17
after approving the letterless menu-bar fist.
SHA256: `825633571834f68e64595eabdc1feac5e569de577351ebc868bec6ca0b02e2ff`.

The earlier detailed artwork, `ChatGPT Image Nov 16, 2026, 03_25_01 PM.png`,
remains unchanged in the workspace source assets; it is no longer the app icon.
Its SHA256 is `50ceada462548c677fb6e68e454625ca359aced2954daad5fb13abb9db85f693`.

`slug-menubar-template.png` is an unchanged copy of the separately generated,
approved letterless, reduced-detail fist (`slug-menubar-generated.png`,
1254 × 1254, alpha).
SHA256: `09d4f611cb9c2976b2b0403f8d6c768356ea2e1339f96c437d77dac7c8bb51a9`.

The app embeds both sources. The macOS build creates standard 16–1024 pixel
ICNS representations from the app image using `sips` and `iconutil`; source
artwork is not rewritten. The menu bar uses the separate alpha mask as a native
template image, allowing the system to choose its light/dark tint.

These native assets do not replace the existing frontend logo or fighter
portrait assets. Build verification does not establish GUI or accessibility
acceptance; the full joint D74 review remains pending. Wails v3.1.0 sets the
menu image canvas to the system status-bar thickness and marks it as a template.
The 1254-pixel source supplies ample Retina resolution; its transparent margin
remains intact. Wails' tooltip setter is a macOS no-op, so no working macOS
tooltip or screen-reader behavior is claimed.
