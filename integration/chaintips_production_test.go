//go:build !slugtestpow

package integration

import "testing"

func chainTipsFixturesForProfile(_ *testing.T, fixtures []string) []string {
	return fixtures
}
