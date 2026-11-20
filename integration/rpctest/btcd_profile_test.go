// Copyright (c) 2026 The SLUG developers. ISC license.

package rpctest

import (
	"errors"
	"reflect"
	"runtime/debug"
	"testing"
)

func TestBtcdCompilerProfileOriginalFlags(t *testing.T) {
	const noCSE = "all=-d=ssa/generic_cse/off,ssa/lowered_cse/off"
	for _, tc := range []struct {
		name, version, flags string
	}{
		{"minimum", "go1.26.10", ""},
		{"default", "go1.27.4", ""},
		{"candidate", "go1.27.4", noCSE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &debug.BuildInfo{GoVersion: tc.version, Settings: []debug.BuildSetting{{Key: "-compiler", Value: "gc"}}}
			if tc.flags != "" {
				info.Settings = append(info.Settings, debug.BuildSetting{Key: "-gcflags", Value: tc.flags})
			}
			original, err := btcdCompilerProfileFromBuildInfo(info)
			if err != nil || original != (btcdCompilerProfile{goVersion: tc.version, compiler: "gc", gcflags: tc.flags}) {
				t.Fatalf("original profile was not retained: %+v, %v", original, err)
			}
			args := btcdBuildArgs("test daemon", original)
			if args[4] != "-compiler=gc" || args[5] != "-gcflags="+tc.flags {
				t.Fatalf("original compiler flags were not explicit: %q", args)
			}
			for _, cheap := range []bool{false, true} {
				child := &debug.BuildInfo{GoVersion: tc.version, Settings: append([]debug.BuildSetting(nil), info.Settings...)}
				if cheap {
					child.Settings = append(child.Settings, debug.BuildSetting{Key: "-tags", Value: "slugtestpow"})
				}
				if err := btcdVerifyBuildInfo(child, original, cheap); err != nil {
					t.Fatal("matching child refused", err)
				}
				if !errors.Is(btcdVerifyBuildInfo(child, original, !cheap), errBtcdCompilerProfile) {
					t.Fatal("different child proof profile accepted")
				}
			}
		})
	}
}

func TestBtcdCompilerProfileMismatchRefusals(t *testing.T) {
	const noCSE = "all=-d=ssa/generic_cse/off,ssa/lowered_cse/off"
	original := btcdCompilerProfile{goVersion: "go1.27.4", compiler: "gc", gcflags: noCSE}
	base := func() *debug.BuildInfo {
		return &debug.BuildInfo{GoVersion: "go1.27.4", Settings: []debug.BuildSetting{
			{Key: "-compiler", Value: "gc"}, {Key: "-gcflags", Value: noCSE},
		}}
	}
	cases := map[string]*debug.BuildInfo{"missing": nil, "empty": {}}
	child := base()
	child.GoVersion = "go1.26.10"
	cases["other-version"] = child
	child = base()
	child.Settings[0].Value = "gccgo"
	cases["other-compiler"] = child
	child = base()
	child.Settings = child.Settings[1:]
	cases["missing-compiler"] = child
	child = base()
	child.Settings = child.Settings[:1]
	cases["missing-flags"] = child
	child = base()
	child.Settings[1].Value = "all=-N -l"
	cases["other-flags"] = child
	for _, key := range []string{"-compiler", "-gcflags"} {
		child = base()
		child.Settings = append(child.Settings, debug.BuildSetting{Key: key, Value: "gc"})
		cases["duplicate-"+key] = child
	}
	for _, key := range []string{"-asmflags", "-gccgoflags", "-toolexec", "-pgo", "GOEXPERIMENT"} {
		child = base()
		child.Settings = append(child.Settings, debug.BuildSetting{Key: key, Value: "unexpected"})
		cases["extra-"+key] = child
	}
	child = base()
	child.Settings = append(child.Settings, debug.BuildSetting{Key: "-tags", Value: "rpctest"})
	cases["extra-tag"] = child
	child = base()
	child.Settings = append(child.Settings, debug.BuildSetting{Key: "-tags", Value: ""}, debug.BuildSetting{Key: "-tags", Value: ""})
	cases["duplicate-tags"] = child
	for name, info := range cases {
		t.Run(name, func(t *testing.T) {
			if !errors.Is(btcdVerifyBuildInfo(info, original, false), errBtcdCompilerProfile) {
				t.Fatal("mismatched or unavailable profile accepted")
			}
		})
	}
}

func TestBtcdCompilerProfileMatchesOriginalBinary(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("original test binary has no build information")
	}
	p, err := btcdOriginalCompilerProfile()
	if err != nil {
		t.Fatal(err)
	}
	var compiler, flags string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "-compiler":
			compiler = setting.Value
		case "-gcflags":
			flags = setting.Value
		}
	}
	if p.goVersion != info.GoVersion || p.compiler != compiler || p.gcflags != flags {
		t.Fatal("nested compiler selection differs from original test binary")
	}
}

func TestBtcdCompilerProfileChildEnvironment(t *testing.T) {
	for _, flags := range []string{
		"-gcflags=all=-N",
		"-gcflags=github.com/taytr-llc/slug/wire/v2=-N",
		"-gcflags=all=-N -gcflags=github.com/taytr-llc/slug/wire/v2=-l -tags=conflicting",
	} {
		t.Run(flags, func(t *testing.T) {
			original := []string{
				"PATH=/original/toolchain", "GOWORK=/original/workspace/go.work",
				"GOFLAGS=" + flags, "GOENV=/alternate/go-env", "GOEXPERIMENT=loopvar",
				"GOTOOLCHAIN=auto", "GOCACHE=/original/cache", "GOMODCACHE=/original/modules",
				"CGO_ENABLED=1", "GOOS=linux", "GOARCH=amd64",
				"GOFLAGS=-gcflags=all=-l", "GoFlags=-gcflags=all=-N",
				"goenv=/other/env", "goexperiment=other", "gotoolchain=path",
			}
			before := append([]string(nil), original...)
			want := []string{
				"PATH=/original/toolchain", "GOWORK=/original/workspace/go.work",
				"GOCACHE=/original/cache", "GOMODCACHE=/original/modules",
				"CGO_ENABLED=1", "GOOS=linux", "GOARCH=amd64",
				"GOENV=off", "GOFLAGS=", "GOEXPERIMENT=", "GOTOOLCHAIN=local",
			}
			got := btcdBuildEnv(original)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("child environment = %q, want %q", got, want)
			}
			if !reflect.DeepEqual(original, before) {
				t.Fatal("original environment changed")
			}
			got[0] = "changed child entry"
			if !reflect.DeepEqual(original, before) {
				t.Fatal("child environment aliases original")
			}
		})
	}
}
