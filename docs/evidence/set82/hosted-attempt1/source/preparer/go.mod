module github.com/taytr-llc/slug/desktop/hostedfixture

go 1.27.3

require (
 github.com/taytr-llc/slug v0.27.0
 github.com/taytr-llc/slug/desktop v0.0.0
 github.com/taytr-llc/slugwallet v0.0.0
 github.com/taytr-llc/slug/btcutil/v2 v2.0.2
 github.com/taytr-llc/slug/chaincfg/v2 v2.0.0
 github.com/taytr-llc/slug/wire/v2 v2.0.2
)

// Build in an explicitly composed workspace containing the reviewed paired
// chain, desktop, wallet and nested modules. No remote replacement or fallback.
