//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeRemoteSelectedFileIdentityAndCleanup(t *testing.T) {
	d, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(d, "config")
	if e = os.WriteFile(p, []byte("public config only"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := selectNativeRemoteInput(context.Background(), p, 128)
	if s != nil {
		t.Cleanup(func() {
			if e := s.close(); e != nil {
				t.Error(e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(p, p+".old"); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, []byte("public config only"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.read(context.Background()); !errors.Is(e, errNativeRemoteIntegrity) {
		t.Fatal("replacement adopted", e)
	}
}
func TestNativeRemoteFailedOriginalCloseRetained(t *testing.T) {
	d, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(d, "config")
	if e = os.WriteFile(p, []byte("public config only"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := selectNativeRemoteInput(context.Background(), p, 128)
	if e != nil {
		if s != nil {
			_ = s.close()
		}
		t.Fatal(e)
	}
	life, cancel := context.WithCancel(context.Background())
	o := &nativeRemoteOwner{life: life, cancel: cancel, input: s}
	// Exercise the real sticky OS-close failure leaf, not a generic fake owner.
	if e = s.file.Close(); e != nil {
		t.Fatal(e)
	}
	err := o.failure(errors.New("PRIVATE-PATH-CAUSE"), true)
	var retained *nativeRemoteError
	if !errors.As(err, &retained) || retained.retained != o || o.input != s || !errors.Is(err, errNativeRemoteCleanup) {
		t.Fatal("lost original cleanup owner", err)
	}
	if o.close() != o.closeErr || !s.closeDone {
		t.Fatal("failed close retried or forgotten")
	}
}
