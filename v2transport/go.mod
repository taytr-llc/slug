module github.com/taytr-llc/slug/v2transport

go 1.26.10

require (
	github.com/btcsuite/btclog v1.0.0
	github.com/taytr-llc/slug/btcec/v2 v2.5.0
	golang.org/x/crypto v0.57.0
)

require (
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/taytr-llc/slug/chainhash/v2 v2.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

// Resolve unpublished SLUG modules from this checkout until fork tags exist.
replace (
	github.com/taytr-llc/slug/btcec/v2 => ../btcec
	github.com/taytr-llc/slug/chainhash/v2 => ../chainhash
)
