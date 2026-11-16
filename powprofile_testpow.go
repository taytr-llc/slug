//go:build slugtestpow

package main

import "errors"

// Cheap PoW builds use a separate default config, database and log namespace.
// The profile is selected at compile time and cannot be enabled by node flags.
const (
	powProfileAppName    = "slug-testpow"
	powProfilePreRelease = "slugtestpow"
)

func validatePoWProfile(cfg *config) error {
	if (!cfg.RegressionTest && !cfg.SimNet) || cfg.TestNet3 || cfg.TestNet4 || cfg.SigNet {
		return errors.New("slugtestpow is for isolated tests only; explicitly select --regtest or --simnet")
	}
	return nil
}
