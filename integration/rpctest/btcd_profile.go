// Copyright (c) 2026 The SLUG developers. ISC license.

package rpctest

import (
	"errors"
	"runtime/debug"
	"strings"
)

// This is the original harness binary's compiler selection, not a profile
// inferred from its environment or from the child compiler's defaults.
type btcdCompilerProfile struct {
	goVersion string
	compiler  string
	gcflags   string
}

var errBtcdCompilerProfile = errors.New("nested btcd compiler profile differs from the original harness")

func btcdCompilerProfileFromBuildInfo(info *debug.BuildInfo) (btcdCompilerProfile, error) {
	if info == nil || info.GoVersion == "" {
		return btcdCompilerProfile{}, errBtcdCompilerProfile
	}
	p := btcdCompilerProfile{goVersion: info.GoVersion}
	seen := make(map[string]bool, 2)
	for _, setting := range info.Settings {
		switch setting.Key {
		case "-compiler", "-gcflags":
			if seen[setting.Key] {
				return btcdCompilerProfile{}, errBtcdCompilerProfile
			}
			seen[setting.Key] = true
			if setting.Key == "-compiler" {
				p.compiler = setting.Value
			} else {
				p.gcflags = setting.Value
			}
		case "-asmflags", "-gccgoflags", "-toolexec", "-pgo", "GOEXPERIMENT":
			// Neither the candidate profile nor plain compatibility builds
			// select these. Refuse instead of silently dropping an override.
			return btcdCompilerProfile{}, errBtcdCompilerProfile
		}
	}
	if p.compiler != "gc" {
		return btcdCompilerProfile{}, errBtcdCompilerProfile
	}
	return p, nil
}

func btcdOriginalCompilerProfile() (btcdCompilerProfile, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return btcdCompilerProfile{}, errBtcdCompilerProfile
	}
	return btcdCompilerProfileFromBuildInfo(info)
}

func btcdVerifyBuildInfo(info *debug.BuildInfo, original btcdCompilerProfile, testPoW bool) error {
	actual, err := btcdCompilerProfileFromBuildInfo(info)
	if err != nil || actual != original {
		return errBtcdCompilerProfile
	}
	tags, seen := "", false
	for _, setting := range info.Settings {
		if setting.Key != "-tags" {
			continue
		}
		if seen {
			return errBtcdCompilerProfile
		}
		tags, seen = setting.Value, true
	}
	expectedTags := ""
	if testPoW {
		expectedTags = "slugtestpow"
	}
	if tags != expectedTags {
		return errBtcdCompilerProfile
	}
	return nil
}

// btcdBuildEnv changes only the child environment. A final explicit -gcflags
// does not erase earlier per-package matchers from GOFLAGS; additionally,
// BuildInfo retains only the final raw setting. Clear that hidden input before
// applying the original binary's compiler selection. Match case-insensitively
// so duplicate Windows environment names cannot preserve an alternate value.
func btcdBuildEnv(original []string) []string {
	out := make([]string, 0, len(original)+4)
	for _, entry := range original {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GOENV", "GOFLAGS", "GOEXPERIMENT", "GOTOOLCHAIN":
			continue
		}
		out = append(out, entry)
	}
	return append(out, "GOENV=off", "GOFLAGS=", "GOEXPERIMENT=", "GOTOOLCHAIN=local")
}
