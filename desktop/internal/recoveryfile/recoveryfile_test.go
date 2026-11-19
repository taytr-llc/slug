//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var fault = errors.New("injected filesystem failure")

func canonicalTemp(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func allow(commit func(func() error) error) error { return commit(func() error { return nil }) }
func newOutput(t *testing.T) (*Selection, *Handle, string) {
	t.Helper()
	path := filepath.Join(canonicalTemp(t), "recovery.json")
	s, e := Capture(path, nil, CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	h, e := Open(s.Pin(), nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = h.Close(); _ = s.Close() })
	return s, h, path
}
func noTemps(t *testing.T, path string) {
	t.Helper()
	entries, e := os.ReadDir(filepath.Dir(path))
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".slug-recovery-") {
			t.Fatal("owned temporary file retained")
		}
	}
}
func absent(t *testing.T, path string) {
	t.Helper()
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("unexpected destination", e)
	}
}

func TestPublicationExactBytesAndFence(t *testing.T) {
	s, h, path := newOutput(t)
	raw := []byte("bounded public test bytes\n")
	active := false
	original := h.ops
	h.ops.create = func(d *os.File, n string) (*os.File, error) {
		if !active {
			t.Fatal("create outside capability")
		}
		return original.create(d, n)
	}
	h.ops.write = func(f *os.File, b []byte) (int, error) {
		if !active {
			t.Fatal("write outside capability")
		}
		return original.write(f, b)
	}
	h.ops.syncFile = func(f *os.File) error {
		if !active {
			t.Fatal("file sync outside capability")
		}
		return original.syncFile(f)
	}
	h.ops.link = func(d *os.File, a, b string) error {
		if !active {
			t.Fatal("link outside capability")
		}
		return original.link(d, a, b)
	}
	h.ops.syncDirectory = func(f *os.File) error {
		if !active {
			t.Fatal("directory sync outside capability")
		}
		return original.syncDirectory(f)
	}
	fence := func(commit func(func() error) error) error {
		active = true
		defer func() { active = false }()
		return commit(func() error {
			if !active {
				return fault
			}
			return nil
		})
	}
	if e := h.Publish(context.Background(), raw, fence); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(got, raw) {
		t.Fatal("published bytes differ", e)
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private mode missing", e)
	}
	noTemps(t, path)
	if e = h.Publish(context.Background(), []byte("replacement"), allow); !errors.Is(e, ErrInvalid) {
		t.Fatal("publication reused", e)
	}
	if _, e = Capture(path, nil, CreateMode); !errors.Is(e, ErrConflict) {
		t.Fatal("existing file adopted", e)
	}
	if e = h.Close(); e != nil {
		t.Fatal(e)
	}
	if e = h.Close(); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestSelectedAliasesAndOwnedPins(t *testing.T) {
	root := canonicalTemp(t)
	actual := filepath.Join(root, "actual")
	alias := filepath.Join(root, "alias")
	if e := os.Mkdir(actual, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(actual, alias); e != nil {
		t.Fatal(e)
	}
	selected, e := Capture(filepath.Join(alias, "out.json"), nil, CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	defer selected.Close()
	if selected.Pin().Path != filepath.Join(actual, "out.json") {
		t.Fatal("legitimate alias not canonicalized")
	}
	path := filepath.Join(actual, "input.json")
	if e = os.WriteFile(path, []byte("input"), 0600); e != nil {
		t.Fatal(e)
	}
	input, e := Capture(path, nil, ReadMode)
	if e != nil {
		t.Fatal(e)
	}
	defer input.Close()
	p := input.Pin()
	p.File.Size++
	if input.Pin().File.Size == p.File.Size {
		t.Fatal("pin leaked owned pointer")
	}
	if _, e = Capture(filepath.Join(alias, "other"), []string{actual}, CreateMode); !errors.Is(e, ErrInvalid) {
		t.Fatal("alias into protected directory accepted", e)
	}
}
func TestParentHandoffSubstitutionRefused(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := canonicalTemp(t)
			parent := filepath.Join(root, "selected")
			if e := os.Mkdir(parent, 0700); e != nil {
				t.Fatal(e)
			}
			s, e := Capture(filepath.Join(parent, "out.json"), nil, CreateMode)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			if e = os.Rename(parent, filepath.Join(root, "original")); e != nil {
				t.Fatal(e)
			}
			if kind == "directory" {
				e = os.Mkdir(parent, 0700)
			} else {
				replacement := filepath.Join(root, "replacement")
				e = os.Mkdir(replacement, 0700)
				if e == nil {
					e = os.Symlink(replacement, parent)
				}
			}
			if e != nil {
				t.Fatal(e)
			}
			if h, e := Open(s.Pin(), nil); e == nil {
				h.Close()
				t.Fatal("substituted parent opened")
			}
			absent(t, filepath.Join(parent, "out.json"))
			absent(t, filepath.Join(root, "original", "out.json"))
		})
	}
}
func TestPinnedParentReplacementBeforeCommitRefused(t *testing.T) {
	root := canonicalTemp(t)
	parent := filepath.Join(root, "selected")
	if e := os.Mkdir(parent, 0700); e != nil {
		t.Fatal(e)
	}
	s, e := Capture(filepath.Join(parent, "out.json"), nil, CreateMode)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	h, e := Open(s.Pin(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	fence := func(commit func(func() error) error) error {
		if e := os.Rename(parent, filepath.Join(root, "original")); e != nil {
			t.Fatal(e)
		}
		if e := os.Mkdir(parent, 0700); e != nil {
			t.Fatal(e)
		}
		return commit(func() error { return nil })
	}
	if e = h.Publish(context.Background(), []byte("bytes"), fence); !errors.Is(e, ErrInvalid) {
		t.Fatal("replacement committed", e)
	}
	absent(t, filepath.Join(parent, "out.json"))
	absent(t, filepath.Join(root, "original", "out.json"))
}
func TestManagedRootsAndMalformedPinsRefused(t *testing.T) {
	for _, marker := range managedMarkers {
		t.Run(marker, func(t *testing.T) {
			root := canonicalTemp(t)
			if e := os.WriteFile(filepath.Join(root, marker), []byte("{}"), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := Capture(filepath.Join(root, "out.json"), nil, CreateMode); !errors.Is(e, ErrInvalid) {
				t.Fatal("managed marker ignored", e)
			}
		})
	}
	for _, base := range []string{"LocalPilot", "LocalWalletPilot", "LocalAccountPilot", "LocalManifestPilot", "LocalPoolPilot"} {
		root := filepath.Join(canonicalTemp(t), base)
		if e := os.Mkdir(root, 0700); e != nil {
			t.Fatal(e)
		}
		if _, e := Capture(filepath.Join(root, "out.json"), nil, CreateMode); e == nil {
			t.Fatal("managed basename ignored")
		}
	}
	s, _, _ := newOutput(t)
	for _, mutate := range []func(*Pin){func(p *Pin) { p.Version++ }, func(p *Pin) { p.Mode = ReadMode }, func(p *Pin) { p.Parent.Inode = "0000000000000000" }, func(p *Pin) { p.Parent.Device = "A000000000000000" }, func(p *Pin) { p.Path += "/../out.json" }, func(p *Pin) { p.File = &FileIdentity{} }, func(p *Pin) { p.Path = "relative" }} {
		p := s.Pin()
		mutate(&p)
		if ValidatePin(p, CreateMode) == nil {
			t.Fatal("malformed pin accepted")
		}
	}
}
func TestExistingDestinationsAndRacedLinkRefused(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root := canonicalTemp(t)
			path := filepath.Join(root, "out.json")
			var e error
			switch kind {
			case "file":
				e = os.WriteFile(path, []byte("existing"), 0600)
			case "directory":
				e = os.Mkdir(path, 0700)
			case "symlink":
				e = os.Symlink("missing", path)
			case "hardlink":
				source := filepath.Join(root, "source")
				e = os.WriteFile(source, []byte("existing"), 0600)
				if e == nil {
					e = os.Link(source, path)
				}
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = Capture(path, nil, CreateMode); !errors.Is(e, ErrConflict) {
				t.Fatal("existing destination accepted", e)
			}
		})
	}
	_, h, path := newOutput(t)
	old := h.ops.link
	h.ops.link = func(d *os.File, a, b string) error {
		if e := os.WriteFile(path, []byte("racing destination"), 0600); e != nil {
			t.Fatal(e)
		}
		return old(d, a, b)
	}
	if e := h.Publish(context.Background(), []byte("new bytes"), allow); !errors.Is(e, ErrConflict) || errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("race not classified as conflict", e)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "racing destination" {
		t.Fatal("existing file overwritten")
	}
	noTemps(t, path)
}
func TestPublicationFailureBoundaries(t *testing.T) {
	for _, stage := range []string{"create", "short-write", "write", "file-sync", "file-close", "link", "post-link-error", "directory-sync", "remove"} {
		t.Run(stage, func(t *testing.T) {
			_, h, path := newOutput(t)
			old := h.ops
			switch stage {
			case "create":
				h.ops.create = func(*os.File, string) (*os.File, error) { return nil, fault }
			case "short-write":
				h.ops.write = func(f *os.File, b []byte) (int, error) { return old.write(f, b[:len(b)-1]) }
			case "write":
				h.ops.write = func(*os.File, []byte) (int, error) { return 0, fault }
			case "file-sync":
				h.ops.syncFile = func(*os.File) error { return fault }
			case "file-close":
				h.ops.closeFile = func(f *os.File) error { return errors.Join(old.closeFile(f), fault) }
			case "link":
				h.ops.link = func(*os.File, string, string) error { return fault }
			case "post-link-error":
				h.ops.link = func(d *os.File, a, b string) error { return errors.Join(old.link(d, a, b), fault) }
			case "directory-sync":
				h.ops.syncDirectory = func(*os.File) error { return fault }
			case "remove":
				h.ops.remove = func(*os.File, string) error { return fault }
			}
			e := h.Publish(context.Background(), []byte("bounded bytes"), allow)
			if e == nil {
				t.Fatal("failure disappeared")
			}
			uncertain := stage == "link" || stage == "post-link-error" || stage == "directory-sync" || stage == "remove"
			if errors.Is(e, ErrOutcomeUnknown) != uncertain {
				t.Fatal("incorrect publication certainty", stage, e)
			}
			if stage == "post-link-error" || stage == "directory-sync" || stage == "remove" {
				got, re := os.ReadFile(path)
				if re != nil || string(got) != "bounded bytes" {
					t.Fatal("possible final file erased", re)
				}
			} else {
				absent(t, path)
			}
			if stage == "remove" {
				if !errors.Is(e, ErrCleanup) || !errors.Is(h.Close(), ErrCleanup) {
					t.Fatal("cleanup failure not sticky", e)
				}
				_ = os.Remove(filepath.Join(filepath.Dir(path), h.temporary))
			} else {
				noTemps(t, path)
			}
		})
	}
}
func TestRevocationAndMisbehavingFences(t *testing.T) {
	for _, kind := range []string{"omitted", "nil-check", "before", "after-sync", "after-link", "double", "swallowed", "outer-error", "late-callback"} {
		t.Run(kind, func(t *testing.T) {
			_, h, path := newOutput(t)
			valid := true
			check := func() error {
				if !valid {
					return fault
				}
				return nil
			}
			var late func(func() error) error
			fence := Fence(func(commit func(func() error) error) error { return commit(check) })
			switch kind {
			case "omitted":
				fence = func(func(func() error) error) error { return nil }
			case "nil-check":
				fence = func(c func(func() error) error) error { return c(nil) }
			case "before":
				valid = false
			case "after-sync":
				old := h.ops.syncFile
				h.ops.syncFile = func(f *os.File) error { e := old(f); valid = false; return e }
			case "after-link":
				old := h.ops.syncDirectory
				h.ops.syncDirectory = func(f *os.File) error { e := old(f); valid = false; return e }
			case "double":
				fence = func(c func(func() error) error) error { _ = c(check); _ = c(check); return nil }
			case "swallowed":
				h.ops.write = func(*os.File, []byte) (int, error) { return 0, fault }
				fence = func(c func(func() error) error) error { _ = c(check); return nil }
			case "outer-error":
				fence = func(c func(func() error) error) error { _ = c(check); return fault }
			case "late-callback":
				fence = func(c func(func() error) error) error { late = c; return nil }
			}
			e := h.Publish(context.Background(), []byte("bytes"), fence)
			if e == nil {
				t.Fatal("invalid fence returned success")
			}
			after := kind == "after-link" || kind == "double" || kind == "outer-error"
			if errors.Is(e, ErrOutcomeUnknown) != after {
				t.Fatal("wrong fence certainty", kind, e)
			}
			if late != nil {
				if e = late(check); e == nil {
					t.Fatal("retained callback accepted")
				}
			}
			if !after {
				absent(t, path)
			}
			noTemps(t, path)
		})
	}
}
func TestPayloadOwnershipSizeAndCancellation(t *testing.T) {
	_, h, path := newOutput(t)
	payload := []byte("original")
	fence := func(commit func(func() error) error) error {
		copy(payload, []byte("modified"))
		return commit(func() error { return nil })
	}
	if e := h.Publish(context.Background(), payload, fence); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Fatal("callback mutated retained bytes")
	}
	for _, size := range []int{0, MaxSize + 1} {
		_, h, path := newOutput(t)
		if e := h.Publish(context.Background(), make([]byte, size), allow); !errors.Is(e, ErrInvalid) {
			t.Fatal("bad size accepted", e)
		}
		absent(t, path)
	}
	_, h, path = newOutput(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := h.Publish(ctx, []byte("bytes"), allow); e == nil {
		t.Fatal("cancelled publication accepted")
	}
	absent(t, path)
	noTemps(t, path)
}
func newInput(t *testing.T) (*Selection, *Handle, string, []byte) {
	t.Helper()
	path := filepath.Join(canonicalTemp(t), "input.json")
	raw := []byte("owned test input bytes")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	s, e := Capture(path, nil, ReadMode)
	if e != nil {
		t.Fatal(e)
	}
	h, e := Open(s.Pin(), nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = h.Close(); _ = s.Close() })
	return s, h, path, raw
}
func TestReadPinnedBytesOnlyInsideOriginalFence(t *testing.T) {
	_, h, _, want := newInput(t)
	active := false
	old := h.ops.read
	h.ops.read = func(f *os.File, b []byte) (int, error) {
		if !active {
			t.Fatal("read escaped account fence")
		}
		return old(f, b)
	}
	fence := func(c func(func() error) error) error {
		active = true
		defer func() { active = false }()
		return c(func() error { return nil })
	}
	got, e := h.Read(context.Background(), fence)
	if e != nil || !bytes.Equal(got, want) {
		t.Fatal("read differs", e)
	}
	clear(got)
	if _, e = h.Read(context.Background(), allow); !errors.Is(e, ErrInvalid) {
		t.Fatal("read handle reused", e)
	}
}
func TestReadSubstitutionMutationAndFailureRefused(t *testing.T) {
	for _, kind := range []string{"replace-before-open", "mutate-before-open", "replace-before-read", "mutate-during-read", "revoke-after-read", "swallowed-read-error"} {
		t.Run(kind, func(t *testing.T) {
			s, h, path, _ := newInput(t)
			switch kind {
			case "replace-before-open":
				if e := os.Rename(path, path+".old"); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(path, []byte("replacement"), 0600); e != nil {
					t.Fatal(e)
				}
			case "mutate-before-open":
				if e := os.WriteFile(path, []byte("changed"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if strings.HasSuffix(kind, "before-open") {
				if next, e := Open(s.Pin(), nil); e == nil {
					next.Close()
					t.Fatal("changed selected input reopened")
				}
				return
			}
			valid := true
			fence := Fence(func(c func(func() error) error) error {
				return c(func() error {
					if !valid {
						return fault
					}
					return nil
				})
			})
			if kind == "replace-before-read" {
				if e := os.Rename(path, path+".old"); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(path, []byte("replacement"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			old := h.ops.read
			h.ops.read = func(f *os.File, b []byte) (int, error) {
				n, e := old(f, b)
				switch kind {
				case "mutate-during-read":
					if we := os.WriteFile(path, []byte("changed bytes"), 0600); we != nil {
						t.Fatal(we)
					}
				case "revoke-after-read":
					valid = false
				case "swallowed-read-error":
					return 0, fault
				}
				return n, e
			}
			if kind == "swallowed-read-error" {
				fence = func(c func(func() error) error) error { _ = c(func() error { return nil }); return nil }
			}
			got, e := h.Read(context.Background(), fence)
			if e == nil || got != nil {
				t.Fatal("failed input disclosed bytes", e)
			}
		})
	}
}
func TestReadRejectsSymlinksAndNonregularFiles(t *testing.T) {
	root := canonicalTemp(t)
	target := filepath.Join(root, "real")
	if e := os.WriteFile(target, []byte("content"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "link")
	if e := os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{link, root} {
		if s, e := Capture(path, nil, ReadMode); e == nil {
			s.Close()
			t.Fatal("nonregular input accepted")
		}
	}
	empty := filepath.Join(root, "empty")
	if e := os.WriteFile(empty, nil, 0600); e != nil {
		t.Fatal(e)
	}
	if s, e := Capture(empty, nil, ReadMode); e == nil {
		s.Close()
		t.Fatal("empty input accepted")
	}
}
func TestConcurrentCloseJoinsPublication(t *testing.T) {
	_, h, path := newOutput(t)
	started, release := make(chan struct{}), make(chan struct{})
	old := h.ops.write
	h.ops.write = func(f *os.File, b []byte) (int, error) { close(started); <-release; return old(f, b) }
	done := make(chan error, 1)
	go func() { done <- h.Publish(context.Background(), []byte("bytes"), allow) }()
	<-started
	if e := h.Publish(context.Background(), []byte("second"), allow); !errors.Is(e, ErrBusy) {
		t.Fatal("concurrent publication accepted", e)
	}
	closed := make(chan error, 1)
	go func() { closed <- h.Close() }()
	select {
	case <-closed:
		t.Fatal("close did not join active commit")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
	if e := h.Publish(context.Background(), []byte("third"), allow); !errors.Is(e, ErrClosed) {
		t.Fatal("closed handle reopened", e)
	}
	noTemps(t, path)
}
func TestLateConcurrentFenceCallbacksCannotPublish(t *testing.T) {
	_, h, path := newOutput(t)
	var saved func(func() error) error
	if e := h.Publish(context.Background(), []byte("bytes"), func(c func(func() error) error) error { saved = c; return nil }); e == nil {
		t.Fatal("omitted callback succeeded")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := saved(func() error { return nil }); !errors.Is(e, ErrInvalid) {
				t.Error("late callback accepted", e)
			}
		}()
	}
	wg.Wait()
	absent(t, path)
}
func TestZeroProgressReadRefused(t *testing.T) {
	_, h, _, _ := newInput(t)
	h.ops.read = func(*os.File, []byte) (int, error) { return 0, nil }
	raw, e := h.Read(context.Background(), allow)
	if !errors.Is(e, io.ErrNoProgress) || raw != nil {
		t.Fatal("zero progress disclosed data", e)
	}
}

func TestIntermediateParentSubstitutionIsNotAnAlias(t *testing.T) {
	for _, afterLink := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-create", true: "after-link"}[afterLink], func(t *testing.T) {
			root := canonicalTemp(t)
			a := filepath.Join(root, "a")
			parent := filepath.Join(a, "b")
			if e := os.MkdirAll(parent, 0700); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(parent, "out.json")
			s, e := Capture(path, nil, CreateMode)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			h, e := Open(s.Pin(), nil)
			if e != nil {
				t.Fatal(e)
			}
			defer h.Close()
			substitute := func() {
				if e := os.Rename(a, filepath.Join(root, "original")); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(filepath.Join(root, "original"), a); e != nil {
					t.Fatal(e)
				}
			}
			if afterLink {
				old := h.ops.syncDirectory
				h.ops.syncDirectory = func(d *os.File) error { e := old(d); substitute(); return e }
			} else {
				substitute()
			}
			e = h.Publish(context.Background(), []byte("bytes"), allow)
			if e == nil || errors.Is(e, ErrOutcomeUnknown) != afterLink {
				t.Fatal("substitution certainty incorrect", e)
			}
			if afterLink {
				raw, e := os.ReadFile(filepath.Join(root, "original", "b", "out.json"))
				if e != nil || string(raw) != "bytes" {
					t.Fatal("uncertain final file lost", e)
				}
			} else {
				absent(t, path)
			}
		})
	}
}

func TestExactMaximumAndOversizedInput(t *testing.T) {
	_, h, path := newOutput(t)
	raw := bytes.Repeat([]byte{0x61}, MaxSize)
	if e := h.Publish(context.Background(), raw, allow); e != nil {
		t.Fatal(e)
	}
	selection, e := Capture(path, nil, ReadMode)
	if e != nil {
		t.Fatal(e)
	}
	defer selection.Close()
	input, e := Open(selection.Pin(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer input.Close()
	got, e := input.Read(context.Background(), allow)
	if e != nil || !bytes.Equal(got, raw) {
		t.Fatal("exact maximum did not round-trip", e)
	}
	clear(got)
	big := filepath.Join(canonicalTemp(t), "oversized")
	if e = os.WriteFile(big, make([]byte, MaxSize+1), 0600); e != nil {
		t.Fatal(e)
	}
	if selection, e := Capture(big, nil, ReadMode); e == nil {
		selection.Close()
		t.Fatal("oversized selected input accepted")
	}
}
func TestPrepublicationCleanupFailureIsSticky(t *testing.T) {
	_, h, path := newOutput(t)
	old := h.ops.write
	h.ops.write = func(f *os.File, b []byte) (int, error) { n, e := old(f, b[:2]); return n, errors.Join(e, fault) }
	h.ops.remove = func(*os.File, string) error { return fault }
	e := h.Publish(context.Background(), []byte("bounded bytes"), allow)
	if !errors.Is(e, ErrCleanup) || errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("prepublication cleanup certainty incorrect", e)
	}
	absent(t, path)
	temp := filepath.Join(filepath.Dir(path), h.temporary)
	info, e := os.Stat(temp)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("failed cleanup did not retain private owned file", e)
	}
	if !errors.Is(h.Close(), ErrCleanup) || !errors.Is(h.Close(), ErrCleanup) {
		t.Fatal("cleanup failure disappeared on Close")
	}
	if e = os.Remove(temp); e != nil {
		t.Fatal(e)
	}
}
