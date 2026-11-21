module github.com/taytr-llc/slug/desktop/issuerecoveryactual

go 1.27.3

require (
 github.com/taytr-llc/slug v0.27.0
 github.com/taytr-llc/slug/desktop v0.0.0
 github.com/taytr-llc/slugwallet v0.0.0
 github.com/taytr-llc/slug/btcec/v2 v2.5.1
 github.com/taytr-llc/slug/btcutil/v2 v2.0.2
 github.com/taytr-llc/slug/chaincfg/v2 v2.0.0
 github.com/taytr-llc/slug/chainhash/v2 v2.0.0
 github.com/taytr-llc/slug/wire/v2 v2.0.2
)

// Source-only rehearsal. Resolve exact reviewed local paired checkouts; the
// runner additionally supplies the complete root/desktop/wallet Go workspace.
replace github.com/taytr-llc/slug => /Users/taytrrip/slug/slug
replace github.com/taytr-llc/slug/desktop => /Users/taytrrip/slug/slug/desktop
replace github.com/taytr-llc/slugwallet => /Users/taytrrip/slug/slugwallet
replace github.com/taytr-llc/slug/address/v2 => /Users/taytrrip/slug/slug/address
replace github.com/taytr-llc/slug/btcec/v2 => /Users/taytrrip/slug/slug/btcec
replace github.com/taytr-llc/slug/btcutil/v2 => /Users/taytrrip/slug/slug/btcutil
replace github.com/taytr-llc/slug/chaincfg/v2 => /Users/taytrrip/slug/slug/chaincfg
replace github.com/taytr-llc/slug/chainhash/v2 => /Users/taytrrip/slug/slug/chainhash
replace github.com/taytr-llc/slug/txscript/v2 => /Users/taytrrip/slug/slug/txscript
replace github.com/taytr-llc/slug/wire/v2 => /Users/taytrrip/slug/slug/wire
