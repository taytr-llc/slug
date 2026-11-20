package gymaccount

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

func pinnedAccountFiles(t *testing.T, root string) map[string][32]byte {
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

func TestIdentityPinnedRejectsBeforeStorage(t *testing.T) {
	for _, id := range []string{"", strings.Repeat("0", 64), strings.Repeat("A", 64), strings.Repeat("1", 63), strings.Repeat("g", 64)} {
		path := filepath.Join(t.TempDir(), "new")
		if s, err := CreateIdentityPinned(path, identityTestConfig(), id); err == nil || s != nil {
			t.Fatal("invalid setup identity created authority")
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("invalid setup identity touched storage")
		}
		if s, err := OpenIdentityPinned(path, identityTestConfig(), id); err == nil || s != nil {
			t.Fatal("invalid setup identity opened authority")
		}
	}
}

func TestIdentityPinnedReopenAndMismatchNeverMutates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authority")
	id := strings.Repeat("1", 64) // Public, preselected initializer identity.
	config := identityTestConfig()
	s, err := CreateIdentityPinned(path, config, id)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.AuthorityInfo(context.Background())
	if err != nil || s.AuthorityID() != id || info.AuthorityID != id || info.Version != 2 {
		t.Fatal("authority did not retain exact initializer identity", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	before := pinnedAccountFiles(t, path)
	if s, err = OpenIdentityPinned(path, config, strings.Repeat("2", 64)); err == nil || s != nil {
		t.Fatal("foreign authority pin accepted")
	}
	wrong := config
	wrong.MaxLoginAttempts++
	if s, err = OpenIdentityPinned(path, wrong, id); err == nil || s != nil {
		t.Fatal("foreign configuration accepted")
	}
	if s, err = CreateIdentityPinned(path, config, id); err == nil || s != nil {
		t.Fatal("create adopted an established store")
	}
	if !reflect.DeepEqual(before, pinnedAccountFiles(t, path)) {
		t.Fatal("pin/config refusal opened or changed established LevelDB")
	}
	s, err = OpenIdentityPinned(path, config, id)
	if err != nil {
		t.Fatal(err)
	}
	if s.AuthorityID() != id {
		t.Fatal("reopen changed the retained pin")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// Legacy callers still open the same complete schema, without changing it.
	s, err = OpenIdentity(path, config)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}
