// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"debug/buildinfo"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/taytr-llc/slug/wire/v2"
)

var (
	// compileMtx guards access to the executable path so that the project is
	// only compiled once.
	compileMtx sync.Mutex

	// executablePath is the path to the compiled executable. This is the empty
	// string until btcd is compiled. This should not be accessed directly;
	// instead use the function btcdExecutablePath().
	executablePath string
)

// btcdBuildArgs selects the original compiler settings and the same PoW
// profile as the harness. Its command additionally uses btcdBuildEnv so no
// inherited per-package GOFLAGS matcher can alter dependency compilation.
func btcdBuildArgs(outputPath string, profile btcdCompilerProfile) []string {
	tags := ""
	if wire.TestPoW {
		tags = "slugtestpow"
	}
	return []string{
		"build", "-o", outputPath, "-tags=" + tags,
		"-compiler=" + profile.compiler, "-gcflags=" + profile.gcflags,
		"github.com/taytr-llc/slug",
	}
}

// btcdExecutablePath returns a path to the btcd executable to be used by
// rpctests. To ensure the code tests against the most up-to-date version of
// btcd, this method compiles btcd the first time it is called. After that, the
// generated binary is used for subsequent test harnesses. The executable file
// is not cleaned up, but since it lives at a static path in a temp directory,
// it is not a big deal.
func btcdExecutablePath() (string, error) {
	compileMtx.Lock()
	defer compileMtx.Unlock()

	// If btcd has already been compiled, just use that.
	if len(executablePath) != 0 {
		return executablePath, nil
	}

	testDir, err := baseDir()
	if err != nil {
		return "", err
	}

	// Build btcd to a random path so concurrent `go test` processes
	// (e.g. when test packages run in parallel under `make unit`) do
	// not race on the same output file. Each test process pays a
	// one-time compile cost; within a process the compileMtx-guarded
	// cache keeps it to one build.
	outputPath := filepath.Join(testDir, fmt.Sprintf("btcd-%d", rand.Uint32()))
	if runtime.GOOS == "windows" {
		outputPath += ".exe"
	}
	profile, err := btcdOriginalCompilerProfile()
	if err != nil {
		return "", err
	}
	command := exec.Command("go", btcdBuildArgs(outputPath, profile)...)
	command.Env = btcdBuildEnv(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to build btcd: %w\n%s", err, output)
	}

	info, err := buildinfo.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("read nested btcd build information: %w", err)
	}
	if err := btcdVerifyBuildInfo(info, profile, wire.TestPoW); err != nil {
		return "", err
	}

	// Save executable path so future calls do not recompile.
	executablePath = outputPath
	return executablePath, nil
}
