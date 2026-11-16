package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestPoWProfileSafety(t *testing.T) {
	for _, test := range []struct {
		name   string
		cfg    config
		public bool
	}{
		{name: "default mainnet", public: true},
		{name: "testnet3", cfg: config{TestNet3: true}, public: true},
		{name: "testnet4", cfg: config{TestNet4: true}, public: true},
		{name: "signet", cfg: config{SigNet: true}, public: true},
		{name: "mixed public and private", cfg: config{SimNet: true, TestNet3: true}, public: true},
		{name: "regtest", cfg: config{RegressionTest: true}},
		{name: "simnet", cfg: config{SimNet: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validatePoWProfile(&test.cfg)
			if got, want := err != nil, wire.TestPoW && test.public; got != want {
				t.Fatalf("profile rejection = %v, want %v: %v", got, want, err)
			}
		})
	}

	appName := "slug"
	if wire.TestPoW {
		appName = "slug-testpow"
	}
	if want := btcutil.AppDataDir(appName, false); defaultHomeDir != want {
		t.Fatalf("default home = %q, want %q", defaultHomeDir, want)
	}
	for _, path := range []string{defaultDataDir, defaultLogDir, defaultConfigFile, defaultRPCKeyFile, defaultRPCCertFile} {
		rel, err := filepath.Rel(defaultHomeDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("default path %q is outside profile home: %v", path, err)
		}
	}
	if got := strings.Contains(version(), "slugtestpow"); got != wire.TestPoW {
		t.Fatalf("version %q does not identify the selected PoW profile", version())
	}
}
