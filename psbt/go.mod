module github.com/taytr-llc/slug/psbt/v2

go 1.26.10

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/stretchr/testify v1.12.1
	github.com/taytr-llc/slug/address/v2 v2.0.0
	github.com/taytr-llc/slug/btcec/v2 v2.5.0
	github.com/taytr-llc/slug/btcutil/v2 v2.0.0
	github.com/taytr-llc/slug/chainhash/v2 v2.0.0
	github.com/taytr-llc/slug/txscript/v2 v2.0.0
	github.com/taytr-llc/slug/wire/v2 v2.0.0
)

require (
	github.com/btcsuite/btclog v1.0.0 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/taytr-llc/slug/chaincfg/v2 v2.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

// Resolve unpublished SLUG modules from this checkout until fork tags exist.
replace (
	github.com/taytr-llc/slug/address/v2 => ../address
	github.com/taytr-llc/slug/btcec/v2 => ../btcec
	github.com/taytr-llc/slug/btcutil/v2 => ../btcutil
	github.com/taytr-llc/slug/chaincfg/v2 => ../chaincfg
	github.com/taytr-llc/slug/chainhash/v2 => ../chainhash
	github.com/taytr-llc/slug/txscript/v2 => ../txscript
	github.com/taytr-llc/slug/wire/v2 => ../wire
)
