// Copyright (c) 2026 The SLUG developers. ISC license.
package poolselection

import (
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"path/filepath"
	"testing"
)

func TestHostedOptionsAlwaysRequireExplicitCanonicalNamespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosted")
	for _, v := range []string{"1", "2"} {
		for _, tc := range []struct {
			path     string
			explicit bool
		}{{path, false}, {"relative", true}, {path + "/../hosted", true}, {path + "\n", true}} {
			if _, err := HostedOptions(v, true, tc.path, tc.explicit); err == nil {
				t.Fatal("implicit or noncanonical hosted namespace accepted", v, tc)
			}
		}
		if _, err := HostedOptions(v, true, path, true); err != nil {
			t.Fatal("explicit hosted selection refused", v, err)
		}
	}
	if _, err := HostedOptions("2", false, path, true); err == nil {
		t.Fatal("implicit name2 accepted")
	}
	if _, err := HostedOptions("3", true, path, true); err == nil {
		t.Fatal("unknown name version accepted")
	}
}

func TestExplicitNameVersionRequiresExactPoolAndPathSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh")
	for _, tc := range []struct {
		name               string
		pool, poolExplicit bool
		version            string
		versionExplicit    bool
		path               string
		dataExplicit       bool
	}{
		{"unknown", true, true, "3", true, path, true}, {"empty", true, true, "", true, path, true}, {"legacy", false, false, "2", true, path, true}, {"false-pool", false, true, "2", true, path, true}, {"packaged-default", true, false, "2", true, path, true}, {"environment-data", true, true, "2", true, path, false}, {"relative", true, true, "2", true, "relative", true}, {"unclean", true, true, "2", true, path + "/../fresh", true}, {"implicit-v2", true, true, "2", false, path, true}, {"nonpool-explicit-v1", false, false, "1", true, path, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := Options(tc.pool, tc.poolExplicit, tc.version, tc.versionExplicit, tc.path, tc.dataExplicit); e == nil {
				t.Fatal("unsafe selector accepted")
			}
		})
	}
	v1, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	for _, pool := range []bool{false, true} {
		got, e := Options(pool, false, "1", false, "", false)
		if e != nil || got != v1 {
			t.Fatal("default changed", e)
		}
	}
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil {
		t.Fatal(e)
	}
	got, e := Options(true, true, "2", true, path, true)
	if e != nil || got.CandidateDescriptorID != v2 {
		t.Fatal("explicit v2 refused", e)
	}
}
