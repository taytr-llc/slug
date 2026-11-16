module github.com/taytr-llc/slug/btcec/v2

go 1.26.10

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1
	github.com/stretchr/testify v1.12.1
	github.com/taytr-llc/slug/chainhash/v2 v2.0.0
)

require (
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

// Resolve unpublished SLUG modules from this checkout until fork tags exist.
replace github.com/taytr-llc/slug/chainhash/v2 => ../chainhash
