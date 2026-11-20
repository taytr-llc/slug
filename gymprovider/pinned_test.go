package gymprovider

import (
	"context"
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func pinnedCoordinatorFiles(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	out := make(map[string][32]byte)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		out[name] = sha256.Sum256(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCoordinatorPinnedRejectsBeforeStorage(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	for _, id := range []string{"", strings.Repeat("0", 64), strings.Repeat("A", 64), strings.Repeat("1", 63), strings.Repeat("g", 64)} {
		path := filepath.Join(t.TempDir(), "new")
		if s, err := CreatePinned(ctx, path, f.Config, f.Authority, f.Verifier, id); err == nil || s != nil {
			t.Fatal("invalid setup identity created coordinator")
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("invalid setup identity touched storage")
		}
		if s, err := OpenPinned(ctx, path, f.Config, f.Authority, f.Verifier, id); err == nil || s != nil {
			t.Fatal("invalid setup identity opened coordinator")
		}
	}
}

func TestCoordinatorPinnedReopenAndMismatchNeverMutates(t *testing.T) {
	f := newProviderFixture(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "coordinator")
	id := strings.Repeat("2", 64)
	s, err := CreatePinned(ctx, path, f.Config, f.Authority, f.Verifier, id)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := s.IssueLogin(ctx)
	if err != nil || s.CoordinatorID() != id || flow.CoordinatorID != id {
		t.Fatal("coordinator did not retain exact setup pin", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	before := pinnedCoordinatorFiles(t, path)
	if s, err = OpenPinned(ctx, path, f.Config, f.Authority, f.Verifier, strings.Repeat("3", 64)); err == nil || s != nil {
		t.Fatal("foreign coordinator pin accepted")
	}
	wrong := f.Config
	wrong.MaxLoginFlows++
	if s, err = OpenPinned(ctx, path, wrong, f.Authority, f.Verifier, id); err == nil || s != nil {
		t.Fatal("foreign configuration accepted")
	}
	if s, err = CreatePinned(ctx, path, f.Config, f.Authority, f.Verifier, id); err == nil || s != nil {
		t.Fatal("create adopted established store")
	}
	if !reflect.DeepEqual(before, pinnedCoordinatorFiles(t, path)) {
		t.Fatal("pin/config refusal opened or changed established LevelDB")
	}
	s, err = OpenPinned(ctx, path, f.Config, f.Authority, f.Verifier, id)
	if err != nil {
		t.Fatal(err)
	}
	if s.CoordinatorID() != id {
		t.Fatal("reopen changed coordinator identity")
	}
	status, err := s.Status(ctx, "login", flow.ID)
	if err != nil || status.CoordinatorID != id || status.ID != flow.ID {
		t.Fatal("reopen lost original flow", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path, f.Config, f.Authority, f.Verifier)
	if err != nil {
		t.Fatal(err)
	}
	if s.CoordinatorID() != id {
		t.Fatal("legacy open lost immutable identity accessor")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}
