//go:build slugtestpow

package pilot

import "testing"

func TestCheapProfileCannotInitializePilot(t *testing.T) {
	if c, err := New(Config{}); err == nil || c != nil {
		t.Fatal("cheap profile initialized pilot")
	}
}
