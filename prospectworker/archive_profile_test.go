//go:build slugtestpow

package prospectworker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveCheapProfileBeforeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	s, e := OpenPoolOutboxAudit(path, PoolConfig{})
	if e == nil || s != nil {
		if s != nil {
			s.Close()
		}
		t.Fatal("cheap archive open admitted")
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("refusal touched archive", e)
	}
}
