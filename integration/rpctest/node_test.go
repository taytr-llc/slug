package rpctest

import (
	"strings"
	"testing"
)

// A failed nested build must never start an unrelated daemon from PATH, which
// could silently use a different proof-of-work profile than the test process.
func TestNodeConfigDoesNotFallBackAfterBuildFailure(t *testing.T) {
	compileMtx.Lock()
	savedPath := executablePath
	executablePath = ""
	compileMtx.Unlock()
	t.Cleanup(func() {
		compileMtx.Lock()
		executablePath = savedPath
		compileMtx.Unlock()
	})
	t.Setenv("PATH", t.TempDir())

	cfg, err := newConfig(t.TempDir(), "", "", nil, "")
	if err == nil || !strings.Contains(err.Error(), "failed to build btcd") {
		t.Fatalf("expected daemon build error, got config %v, error %v", cfg, err)
	}
	if cfg != nil {
		t.Fatal("failed build returned a runnable node config")
	}
}
