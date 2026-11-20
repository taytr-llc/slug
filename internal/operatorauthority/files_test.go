//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package operatorauthority

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestOperatorAuthorityFileSelectionRefusals(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, Binding, *OnlineSeedFiles)
	}{
		{"short", func(t *testing.T, b Binding, p *OnlineSeedFiles) {
			seed := seedFor(t, b.Keys.Admission)
			mustWrite(t, p.Admission, seed[:31])
		}},
		{"long", func(t *testing.T, b Binding, p *OnlineSeedFiles) {
			seed := seedFor(t, b.Keys.Admission)
			mustWrite(t, p.Admission, append(seed[:], 0))
		}},
		{"newline", func(t *testing.T, b Binding, p *OnlineSeedFiles) {
			seed := seedFor(t, b.Keys.Admission)
			mustWrite(t, p.Admission, append(seed[:], '\n'))
		}},
		{"key_mismatch", func(t *testing.T, b Binding, p *OnlineSeedFiles) {
			seed := seedFor(t, b.Keys.Publication)
			mustWrite(t, p.Admission, seed[:])
		}},
		{"world_read", func(t *testing.T, _ Binding, p *OnlineSeedFiles) { mustChmod(t, p.Admission, 0644) }},
		{"group_read", func(t *testing.T, _ Binding, p *OnlineSeedFiles) { mustChmod(t, p.Admission, 0640) }},
		{"executable", func(t *testing.T, _ Binding, p *OnlineSeedFiles) { mustChmod(t, p.Admission, 0700) }},
		{"parent_not_private", func(t *testing.T, _ Binding, p *OnlineSeedFiles) { mustChmod(t, filepath.Dir(p.Admission), 0755) }},
		{"hard_link", func(t *testing.T, _ Binding, p *OnlineSeedFiles) {
			if e := os.Link(p.Admission, p.Admission+".alias"); e != nil {
				t.Fatal(e)
			}
		}},
		{"leaf_symlink", func(t *testing.T, _ Binding, p *OnlineSeedFiles) {
			if e := os.Rename(p.Admission, p.Admission+".original"); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(p.Admission+".original", p.Admission); e != nil {
				t.Fatal(e)
			}
		}},
		{"ancestor_symlink", func(t *testing.T, _ Binding, p *OnlineSeedFiles) {
			dir := filepath.Dir(p.Admission)
			if e := os.Rename(dir, dir+".original"); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(dir+".original", dir); e != nil {
				t.Fatal(e)
			}
		}},
		{"directory", func(t *testing.T, _ Binding, p *OnlineSeedFiles) { p.Admission = filepath.Dir(p.Admission) }},
		{"fifo", func(t *testing.T, _ Binding, p *OnlineSeedFiles) {
			if e := os.Remove(p.Admission); e != nil {
				t.Fatal(e)
			}
			if e := unix.Mkfifo(p.Admission, 0600); e != nil {
				t.Fatal(e)
			}
		}},
		{"relative", func(_ *testing.T, _ Binding, p *OnlineSeedFiles) { p.Admission = "relative.seed" }},
		{"unclean", func(_ *testing.T, _ Binding, p *OnlineSeedFiles) {
			p.Admission = filepath.Dir(p.Admission) + "/./" + filepath.Base(p.Admission)
		}},
		{"nul", func(_ *testing.T, _ Binding, p *OnlineSeedFiles) { p.Admission += "\x00" }},
		{"path_bound", func(_ *testing.T, _ Binding, p *OnlineSeedFiles) { p.Admission = "/" + strings.Repeat("x", 4096) }},
		{"depth_bound", func(_ *testing.T, _ Binding, p *OnlineSeedFiles) {
			p.Admission = "/" + strings.Repeat("d/", 65) + "seed"
		}},
		{"same_path_different_roles", func(_ *testing.T, _ Binding, p *OnlineSeedFiles) { p.Publication = p.Admission }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := firstBinding(t)
			p := onlineFiles(t, b)
			c.change(t, b, &p)
			o, e := OpenOnline(t.Context(), b, p)
			if o != nil {
				ce := o.Close()
				t.Fatalf("invalid selection retained non-clean owner: %v / %v", e, ce)
			}
			if e == nil {
				t.Fatal("invalid selection accepted")
			}
			for _, s := range []string{p.Admission, p.Publication, p.Cutoff} {
				if s != "" && strings.Contains(e.Error(), s) {
					t.Fatal("private path leaked")
				}
			}
		})
	}
}
func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func mustChmod(t *testing.T, p string, m os.FileMode) {
	t.Helper()
	if e := os.Chmod(p, m); e != nil {
		t.Fatal(e)
	}
}

func TestOperatorAuthoritySelectedIdentityCannotReload(t *testing.T) {
	mutations := []struct {
		name   string
		change func(*testing.T, Binding, OnlineSeedFiles)
	}{
		{"content", func(t *testing.T, b Binding, p OnlineSeedFiles) {
			seed := seedFor(t, b.Keys.Publication)
			mustWrite(t, p.Admission, seed[:])
			if e := os.Chtimes(p.Admission, time.Unix(123, 0), time.Unix(123, 0)); e != nil {
				t.Fatal(e)
			}
		}},
		{"permissions", func(t *testing.T, _ Binding, p OnlineSeedFiles) { mustChmod(t, p.Admission, 0644) }},
		{"hardlink", func(t *testing.T, _ Binding, p OnlineSeedFiles) {
			if e := os.Link(p.Admission, p.Admission+".alias"); e != nil {
				t.Fatal(e)
			}
		}},
		{"removed", func(t *testing.T, _ Binding, p OnlineSeedFiles) {
			if e := os.Remove(p.Admission); e != nil {
				t.Fatal(e)
			}
		}},
		{"same_bytes_new_inode", func(t *testing.T, b Binding, p OnlineSeedFiles) {
			if e := os.Rename(p.Admission, p.Admission+".old"); e != nil {
				t.Fatal(e)
			}
			seed := seedFor(t, b.Keys.Admission)
			mustWrite(t, p.Admission, seed[:])
		}},
		{"renamed_parent", func(t *testing.T, b Binding, p OnlineSeedFiles) {
			dir := filepath.Dir(p.Admission)
			if e := os.Rename(dir, dir+".old"); e != nil {
				t.Fatal(e)
			}
			if e := os.Mkdir(dir, 0700); e != nil {
				t.Fatal(e)
			}
			for i, path := range []string{p.Admission, p.Publication, p.Cutoff} {
				seed := seedFor(t, roleKeys(b)[i])
				mustWrite(t, path, seed[:])
			}
		}},
	}
	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			o, b, p := openOnlineTest(t)
			op, e := o.Begin(t.Context(), b)
			if e != nil {
				t.Fatal(e)
			}
			defer op.Close()
			m.change(t, b, p)
			body, _ := admissionBody(t, b)
			sig, e := op.SignAdmission(body)
			if e == nil || sig != ([64]byte{}) || !o.poisoned.Load() {
				t.Fatal("changed original selection signed or reloaded", e)
			}
			if e = o.Check(t.Context(), b); e != ErrIntegrity {
				t.Fatal("integrity poison not sticky", e)
			}
		})
	}
}

func TestOperatorAuthorityReadOnlyModeAndRoleAlias(t *testing.T) {
	b := firstBinding(t)
	p := onlineFiles(t, b)
	for _, path := range []string{p.Admission, p.Publication, p.Cutoff} {
		mustChmod(t, path, 0400)
	}
	owner, e := OpenOnline(t.Context(), b, p)
	if e != nil {
		t.Fatal(e)
	}
	if owner.Capabilities() != (Capabilities{Admission: true, Publication: true, Cutoff: true}) {
		t.Fatal("online role set")
	}
	if e = owner.Close(); e != nil {
		t.Fatal(e)
	}
	ref, _ := reference(t)
	found := false
	for _, row := range ref.Positives {
		if row.Family == "pin" && strings.HasSuffix(row.Name, "/operator_v2_shared") {
			b = referenceBinding(t, row.Name)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing independently valid shared profile")
	}
	p = onlineFiles(t, b)
	owner, e = OpenOnline(t.Context(), b, p)
	if e != nil {
		t.Fatal(e)
	}
	if len(owner.files) != 1 {
		t.Fatal("shared exact key/path opened more than once")
	}
	if e = owner.Close(); e != nil {
		t.Fatal(e)
	}
	alias := p.Admission + ".second"
	seed := seedFor(t, b.Keys.Admission)
	mustWrite(t, alias, seed[:])
	p.Publication = alias
	if owner, e = OpenOnline(t.Context(), b, p); e == nil || owner != nil {
		if owner != nil {
			owner.Close()
		}
		t.Fatal("same key selected through different paths")
	}
}

func TestOperatorAuthorityUIDAndPartialSelectionCleanup(t *testing.T) {
	b := firstBinding(t)
	p := onlineFiles(t, b)
	selected, seed, e := loadSeed(t.Context(), p.Admission)
	if selected != nil {
		t.Cleanup(func() {
			if e := selected.close(); e != nil {
				t.Error(e)
			}
		})
	}
	clear(seed[:])
	if e != nil {
		t.Fatal(e)
	}
	wrong := selected.original
	wrong.uid++
	if seedProtected(wrong, uint32(os.Geteuid())) {
		t.Fatal("foreign owner admitted")
	}
	if e = selected.close(); e != nil {
		t.Fatal(e)
	}
	for _, d := range selected.dirs {
		if _, e = d.file.Stat(); !errors.Is(e, os.ErrClosed) {
			t.Fatal("directory not closed", e)
		}
	}
	seed = seedFor(t, b.Keys.Admission)
	mustWrite(t, p.Admission, seed[:31])
	clear(seed[:])
	partial, seed, e := loadSeed(t.Context(), p.Admission)
	if partial != nil {
		t.Cleanup(func() {
			if e := partial.close(); e != nil {
				t.Error(e)
			}
		})
	}
	clear(seed[:])
	if e == nil || partial == nil {
		t.Fatal("expected retained partial selection")
	}
	if e = partial.close(); e != nil {
		t.Fatal(e)
	}
	for _, d := range partial.dirs {
		if _, e = d.file.Stat(); !errors.Is(e, os.ErrClosed) {
			t.Fatal("partial descriptor leaked", e)
		}
	}
}

func TestOperatorAuthorityCleanupFailureAndPrivacy(t *testing.T) {
	b := firstBinding(t)
	p := onlineFiles(t, b)
	owner, e := OpenOnline(t.Context(), b, p)
	if e != nil {
		t.Fatal(e)
	}
	// This test deliberately injects ErrCleanup below. Still close on any
	// earlier assertion failure, without treating that expected sticky error as
	// a second unrelated test failure.
	t.Cleanup(func() {
		if e := owner.Close(); e != nil && e != ErrCleanup {
			t.Error(e)
		}
	})
	retained := append([]byte(nil), owner.keys[0]...)
	defer clear(retained)
	// The saved slice observes this owner's buffer, not a whole-process erasure claim.
	original := owner.keys[0]
	if e := owner.files[0].file.Close(); e != nil {
		t.Fatal(e)
	}
	if e := owner.Close(); e != ErrCleanup {
		t.Fatal("cleanup failure hidden", e)
	}
	if e := owner.Close(); e != ErrCleanup {
		t.Fatal("cleanup failure not sticky", e)
	}
	if !bytes.Equal(original, make([]byte, len(original))) {
		t.Fatal("retained key buffer not cleared")
	}
	for _, f := range owner.files {
		for _, d := range f.dirs {
			if _, e := d.file.Stat(); !errors.Is(e, os.ErrClosed) {
				t.Fatal("later resource close skipped", e)
			}
		}
	}
	text := fmt.Sprintf("%v %+v %#v %v", owner, p, p, ErrCleanup)
	if strings.Contains(text, p.Admission) || strings.Contains(text, hex.EncodeToString(retained)) {
		t.Fatal("private formatting leaked")
	}
	if _, e := owner.Begin(t.Context(), b); e != ErrClosed {
		t.Fatal("cleanup-failed owner reopened")
	}
}

func TestOperatorAuthorityExtendedACLRefusal(t *testing.T) {
	for _, which := range []string{"file", "parent"} {
		t.Run(which, func(t *testing.T) {
			b := firstBinding(t)
			p := onlineFiles(t, b)
			path := p.Admission
			if which == "parent" {
				path = filepath.Dir(path)
			}
			addSyntheticACL(t, path)
			owner, e := OpenOnline(t.Context(), b, p)
			if e == nil || owner != nil {
				if owner != nil {
					owner.Close()
				}
				t.Fatal("extended ACL accepted")
			}
		})
	}
	t.Run("added_after_open", func(t *testing.T) {
		owner, b, p := openOnlineTest(t)
		op, e := owner.Begin(t.Context(), b)
		if e != nil {
			t.Fatal(e)
		}
		defer op.Close()
		addSyntheticACL(t, p.Admission)
		body, _ := admissionBody(t, b)
		if sig, e := op.SignAdmission(body); e == nil || sig != ([64]byte{}) || !owner.poisoned.Load() {
			t.Fatal("changed ACL signed", e)
		}
	})
}
