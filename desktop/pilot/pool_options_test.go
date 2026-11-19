// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/poolshare"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func poolSelectionTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		if d.IsDir() {
			result[rel] = "directory"
			return nil
		}
		raw, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		result[rel] = fmt.Sprintf("%x", sha256.Sum256(raw))
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	return result
}
func TestPoolOptionsExactTwoDescriptors(t *testing.T) {
	v1, e := ResolvePoolOptions(PoolOptions{})
	if e != nil || v1.CandidateDescriptorID != manifestDescriptorID {
		t.Fatal("default changed", e)
	}
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{v1.CandidateDescriptorID, v2} {
		got, e := ResolvePoolOptions(PoolOptions{CandidateDescriptorID: id})
		if e != nil || got.CandidateDescriptorID != id {
			t.Fatal("exact supported tuple", e)
		}
	}
	for _, id := range []string{strings.Repeat("f", 64), strings.ToUpper(v2), "latest", "2"} {
		if _, e := ResolvePoolOptions(PoolOptions{CandidateDescriptorID: id}); e == nil {
			t.Fatal("unknown selected", id)
		}
	}
	root := filepath.Join(t.TempDir(), "must-not-exist")
	if _, e := NewPoolRuntimeWithOptions(context.Background(), Config{DataDir: root, NodeBinary: "/not-a-node"}, nil, PoolOptions{CandidateDescriptorID: "unknown"}); e == nil {
		t.Fatal("unknown runtime options")
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("unknown options touched path", e)
	}
}
func TestPoolRuntimeSelectionRejectsBeforeMutation(t *testing.T) {
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil {
		t.Fatal(e)
	}
	for _, selected := range []string{manifestDescriptorID, v2} {
		t.Run(selected[:8], func(t *testing.T) {
			r := poolRuntimeTest(t)
			r.config.DataDir, e = filepath.EvalSymlinks(r.config.DataDir)
			if e != nil {
				t.Fatal(e)
			}
			r.marker.DescriptorID = selected
			r.source.AuthorityPin.DescriptorID = selected
			if e := writePoolRuntimeRecord(filepath.Join(r.config.DataDir, poolRuntimeMarkerName), r.marker); e != nil {
				t.Fatal(e)
			}
			if e := writePoolRuntimeRecord(filepath.Join(r.config.DataDir, poolRuntimeSourceName), r.source); e != nil {
				t.Fatal(e)
			}
			before := poolSelectionTree(t, r.config.DataDir)
			matching := PoolOptions{CandidateDescriptorID: selected}
			if e := CheckPoolRuntimeSelection(r.config.DataDir, r.marker.AccountAuthorityID, matching); e != nil {
				t.Fatal("matching reopen", e)
			}
			other := v2
			if selected == v2 {
				other = manifestDescriptorID
			}
			for _, options := range []PoolOptions{{CandidateDescriptorID: other}, {CandidateDescriptorID: "unknown"}} {
				if e := CheckPoolRuntimeSelection(r.config.DataDir, r.marker.AccountAuthorityID, options); e == nil {
					t.Fatal("mismatched selection accepted")
				}
				if !reflect.DeepEqual(before, poolSelectionTree(t, r.config.DataDir)) {
					t.Fatal("selection preflight changed bytes or created a lock")
				}
			}
			if selected == v2 {
				if e := CheckPoolRuntimeSelection(r.config.DataDir, r.marker.AccountAuthorityID, PoolOptions{}); e == nil {
					t.Fatal("default auto-selected v2")
				}
			}
			bad := r.source
			bad.AuthorityPin.DescriptorID = other
			rawPath := filepath.Join(r.config.DataDir, poolRuntimeSourceName)
			if e := os.Remove(rawPath); e != nil {
				t.Fatal(e)
			}
			if e := writePoolRuntimeRecord(rawPath, bad); e != nil {
				t.Fatal(e)
			}
			before = poolSelectionTree(t, r.config.DataDir)
			if e := CheckPoolRuntimeSelection(r.config.DataDir, r.marker.AccountAuthorityID, matching); e == nil {
				t.Fatal("marker/source descriptor mismatch accepted")
			}
			if !reflect.DeepEqual(before, poolSelectionTree(t, r.config.DataDir)) {
				t.Fatal("mismatched source changed disk")
			}
			if r.marker.validSource(bad) {
				t.Fatal("issuer opening would accept a different descriptor")
			}
		})
	}
}
func TestPoolIssuerV2RequiresExactSourceOnReopen(t *testing.T) {
	r := poolRuntimeTest(t)
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil {
		t.Fatal(e)
	}
	r.marker.DescriptorID = v2
	r.source = poolshare.Config{}
	if e = r.openPoolIssuer(false); e != nil {
		t.Fatal(e)
	}
	original := r.source
	if original.AuthorityPin.DescriptorID != v2 {
		t.Fatal("v2 selection ignored")
	}
	if e = r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer = nil
	if e = r.openPoolIssuer(true); e != nil || r.source != original {
		t.Fatal("exact v2 source reopen", e)
	}
	if e = r.issuer.Close(); e != nil {
		t.Fatal(e)
	}
	r.issuer = nil
	r.marker.DescriptorID = manifestDescriptorID
	before := poolSelectionTree(t, r.config.DataDir)
	if e = r.openPoolIssuer(true); e == nil || r.issuer != nil {
		t.Fatal("v1 marker adopted v2 issuer")
	}
	if !reflect.DeepEqual(before, poolSelectionTree(t, r.config.DataDir)) {
		t.Fatal("rejected issuer reopen changed bytes")
	}
}
