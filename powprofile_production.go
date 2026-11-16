//go:build !slugtestpow

package main

const (
	powProfileAppName    = "slug"
	powProfilePreRelease = ""
)

func validatePoWProfile(_ *config) error {
	return nil
}
