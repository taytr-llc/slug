module github.com/taytr-llc/slug/desktop/hostedaudit

go 1.27.3

require (
 github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d
 github.com/taytr-llc/slug v0.27.0
 github.com/taytr-llc/slug/desktop v0.0.0
 github.com/taytr-llc/slug/chainhash/v2 v2.0.0
 github.com/taytr-llc/slug/wire/v2 v2.0.2
)

// Root composes the exact reviewed chain/desktop/wallet/nested workspace.
// No remote replacement or fallback; this file does not modify product modules.
