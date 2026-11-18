// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/taytr-llc/slug/gymresult"
	"os"
	"path/filepath"
	"testing"
)

func resultProofFixture(t *testing.T) (*ManifestController, string) {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range []string{"private", "bin", "export"} {
		if e = os.Mkdir(filepath.Join(root, d), 0700); e != nil {
			t.Fatal(e)
		}
	}
	c := &ManifestController{config: Config{DataDir: filepath.Join(root, "private"), NodeBinary: filepath.Join(root, "bin", "slugd")}}
	return c, filepath.Join(root, "export", "proof.json")
}
func TestResultProofFileExclusiveFencedStreaming(t *testing.T) {
	for _, mode := range []string{"success", "refused", "cancelled", "replaced-parent", "destination-race", "wrong-summary"} {
		t.Run(mode, func(t *testing.T) {
			c, path := resultProofFixture(t)
			f, e := c.newResultProofFile(path)
			if e != nil {
				t.Fatal(e)
			}
			raw := []byte("{\"public\":true}\n")
			if _, e = f.Write(raw); e != nil {
				t.Fatal(e)
			}
			i, e := os.Stat(filepath.Join(filepath.Dir(path), f.name))
			if e != nil || i.Mode().Perm() != 0600 {
				t.Fatal("private temporary file", e)
			}
			sum := sha256.Sum256(raw)
			summary := gymresult.ProofSummary{Bytes: uint32(len(raw)), SHA256: hex.EncodeToString(sum[:])}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "wrong-summary" {
				summary.Bytes++
			}
			commit, e := f.prepare(ctx, summary)
			if mode == "wrong-summary" {
				if e == nil {
					t.Fatal("bad summary accepted")
				}
				if e = f.close(); e != nil {
					t.Fatal(e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			called := false
			guard := func() error {
				called = true
				if mode == "refused" {
					return gymresult.ErrUnauthorized
				}
				return nil
			}
			switch mode {
			case "cancelled":
				cancel()
			case "destination-race":
				if e = os.WriteFile(path, []byte("original"), 0600); e != nil {
					t.Fatal(e)
				}
			case "replaced-parent":
				if e = os.Rename(filepath.Dir(path), filepath.Dir(path)+"-old"); e != nil {
					t.Fatal(e)
				}
				if e = os.Mkdir(filepath.Dir(path), 0700); e != nil {
					t.Fatal(e)
				}
			}
			e = commit(guard)
			if (e == nil) != (mode == "success") {
				t.Fatal("unexpected commit", mode, e)
			}
			if mode == "success" && !called {
				t.Fatal("missing final original-authority guard")
			}
			if ce := f.close(); ce != nil {
				t.Fatal(ce)
			}
			got, re := os.ReadFile(path)
			switch mode {
			case "success":
				if re != nil || string(got) != string(raw) {
					t.Fatal("published bytes", re)
				}
			case "destination-race":
				if re != nil || string(got) != "original" {
					t.Fatal("overwrote destination")
				}
			default:
				if !os.IsNotExist(re) {
					t.Fatal("refusal published a file", re)
				}
			}
			parents := []string{filepath.Dir(path)}
			if mode == "replaced-parent" {
				parents = append(parents, filepath.Dir(path)+"-old")
			}
			for _, parent := range parents {
				entries, e := os.ReadDir(parent)
				if e != nil {
					t.Fatal("read export directory", parent, e)
				}
				for _, x := range entries {
					if x.Name() == f.name {
						t.Fatal("temporary file retained", parent)
					}
				}
			}
		})
	}
}
func TestResultProofFileBoundsAndProtectedRoots(t *testing.T) {
	c, path := resultProofFixture(t)
	for _, p := range []string{filepath.Join(c.config.DataDir, "proof.json"), filepath.Join(filepath.Dir(c.config.NodeBinary), "proof.json")} {
		if f, e := c.newResultProofFile(p); e == nil || f != nil {
			t.Fatal("protected path accepted")
		}
	}
	f, e := c.newResultProofFile(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.close()
	f.count = gymresult.MaxProofSize
	if n, e := f.Write([]byte{1}); n != 0 || !errors.Is(e, gymresult.ErrCapacity) {
		t.Fatal("over-cap stream admitted", n, e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("stream published before authorization")
	}
}
