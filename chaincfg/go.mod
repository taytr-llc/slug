module github.com/taytr-llc/slug/chaincfg/v2

go 1.26.10

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/stretchr/testify v1.12.1
	github.com/taytr-llc/slug/chainhash/v2 v2.0.0
	github.com/taytr-llc/slug/wire/v2 v2.0.0
)

require (
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

// Resolve unpublished SLUG modules from this checkout until fork tags exist.
replace (
	github.com/taytr-llc/slug/chainhash/v2 => ../chainhash
	github.com/taytr-llc/slug/wire/v2 => ../wire
)
