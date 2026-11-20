//go:build slugtestpow

package pilot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoredGymCheapProfileBeforeFilesystem(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "must-not-exist")
	c := RestoredGymConfig{Directory: root, RecoveryFile: filepath.Join(dir, "not-a-recovery-file"), PublicConfigFile: filepath.Join(dir, "not-a-public-file"), NodeConfigFile: filepath.Join(dir, "not-a-private-file")}
	if s, err := CreateRestoredGymSession(context.Background(), c); err == nil {
		s.Close()
		t.Fatal("cheap create accepted")
	}
	if s, err := OpenRestoredGymSession(context.Background(), c); err == nil {
		s.Close()
		t.Fatal("cheap open accepted")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("cheap profile touched filesystem")
	}
}
