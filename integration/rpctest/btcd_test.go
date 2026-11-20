package rpctest

import (
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestBtcdBuildArgsSelectPoWProfile(t *testing.T) {
	// The harness may have been compiled with an explicit -tags flag that
	// overrode GOFLAGS. Its nested build must make the same choice.
	t.Setenv("GOFLAGS", "-tags=slugtestpow")
	tags := "-tags="
	if wire.TestPoW {
		tags += "slugtestpow"
		t.Setenv("GOFLAGS", "-tags=rpctest")
	}
	profile := btcdCompilerProfile{goVersion: "go1.26.10", compiler: "gc"}
	want := []string{"build", "-o", "test daemon", tags, "-compiler=gc", "-gcflags=", "github.com/taytr-llc/slug"}
	if got := btcdBuildArgs("test daemon", profile); !reflect.DeepEqual(got, want) {
		t.Fatalf("daemon build arguments = %q, want %q", got, want)
	}
}
