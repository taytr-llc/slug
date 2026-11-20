//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

func TestOwnershipOperatorCheapRuntimeRefusal(t *testing.T) {
	profile, e := registry.NewOperatorProfile(operatorCoreCorpus(t).Compositions["v1/operator_v2_distinct"].Pin)
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(t.TempDir(), "never-created")
	if _, e = CreateOperatorReplica(root, profile, StoreIdentity{strings.Repeat("a7", 32)}); !errors.Is(e, ErrProfile) {
		t.Fatal("cheap store admitted", e)
	}
	if _, e = os.Lstat(root); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("cheap store changed disk", e)
	}
	if _, e = NewOperatorOwnedChainSource(context.Background(), profile, prospectnode.RPCConfig{}, context.Background()); !errors.Is(e, ErrProfile) {
		t.Fatal("cheap node admitted", e)
	}
	if _, e = NewOperatorPublicReader(context.Background(), profile, PublicReadConfig{}, context.Background()); !errors.Is(e, ErrProfile) {
		t.Fatal("cheap reader admitted", e)
	}
}
