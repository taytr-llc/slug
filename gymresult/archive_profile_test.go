//go:build slugtestpow

package gymresult

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveCheapProfileBeforeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	s, e := OpenCareerAudit(context.Background(), path, Pin{}, nil)
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
