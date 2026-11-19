package main

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/poolselection"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPoolNameFlagsRejectBeforeEffects(t *testing.T) {
	for _, args := range [][]string{{"--candidate-name-version=2"}, {"--candidate-pool=false", "--candidate-name-version=2"}, {"--candidate-pool", "--candidate-name-version=3"}, {"--candidate-manifests", "--candidate-name-version=2"}, {"--candidate-pool", "--candidate-name-version=2", "positional"}} {
		root := filepath.Join(t.TempDir(), "absent")
		args = append([]string{"--data-dir", root, "--node-binary", "/not-a-node"}, args...)
		if e := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard); e == nil {
			t.Fatal("invalid name route accepted", args)
		}
		if _, e := os.Lstat(root); !os.IsNotExist(e) {
			t.Fatal("invalid name route touched disk", args, e)
		}
	}
}

func TestPoolNameFlagAcceptsBothTrueBooleanSpellings(t *testing.T) {
	for _, poolFlag := range []string{"--candidate-pool", "--candidate-pool=true"} {
		root := filepath.Join(t.TempDir(), "absent")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		e := run(ctx, []string{poolFlag, "--candidate-name-version=2", "--data-dir", root, "--node-binary", "/not-a-node"}, strings.NewReader(""), io.Discard, io.Discard)
		if e == nil || errors.Is(e, poolselection.ErrSelection) {
			t.Fatal("valid selection did not reach the cancelled constructor", poolFlag, e)
		}
		if _, e := os.Lstat(root); !os.IsNotExist(e) {
			t.Fatal("cancelled constructor changed disk")
		}
	}
}
