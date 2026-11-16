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
	want := []string{"build", "-o", "test daemon", tags, "github.com/taytr-llc/slug"}
	if got := btcdBuildArgs("test daemon"); !reflect.DeepEqual(got, want) {
		t.Fatalf("daemon build arguments = %q, want %q", got, want)
	}
}
