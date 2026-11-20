//go:build slugtestpow

package gymstable

import (
	"github.com/taytr-llc/slug/internal/careerstate"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveCheapProfileBeforeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	s, e := OpenSharedCareerAudit(path, Pin{}, nil, careerstate.Activation{})
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
