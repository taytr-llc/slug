module github.com/taytr-llc/slug/desktop/hostedclient

go 1.27.3

require (
 github.com/taytr-llc/slug v0.27.0
 github.com/taytr-llc/slug/desktop v0.0.0
 github.com/taytr-llc/slug/wire/v2 v2.0.2
)

// Explicit composed GOWORK contains the exact paired source modules plus this
// helper, with versioned local replacements for its unpublished requirements.
