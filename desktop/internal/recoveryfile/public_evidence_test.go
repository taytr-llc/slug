//go:build darwin || linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newPublicOutput(t *testing.T) (*Selection, *PublicEvidenceFile, string) {
	t.Helper()
	path := filepath.Join(canonicalTemp(t), "public.json")
	s, err := Capture(path, nil, CreateMode)
	if err != nil {
		t.Fatal(err)
	}
	p, err := OpenPublicEvidence(s.Pin(), nil)
	if err != nil {
		_ = s.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close(); _ = s.Close() })
	return s, p, path
}

func noPublicTemps(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".slug-evidence-") {
			t.Fatal("public staging file retained")
		}
	}
}

func TestPublicEvidenceBulkOutsideFenceAndExactPublication(t *testing.T) {
	_, p, path := newPublicOutput(t)
	inside := false
	old := p.h.ops
	p.h.ops.create = func(d *os.File, n string) (*os.File, error) {
		if inside {
			t.Fatal("bulk creation under account lease")
		}
		return old.create(d, n)
	}
	p.h.ops.write = func(f *os.File, b []byte) (int, error) {
		if inside {
			t.Fatal("bulk write under account lease")
		}
		return old.write(f, b)
	}
	p.h.ops.syncFile = func(f *os.File) error {
		if inside {
			t.Fatal("file sync under account lease")
		}
		return old.syncFile(f)
	}
	p.h.ops.closeFile = func(f *os.File) error {
		if inside {
			t.Fatal("staging close under account lease")
		}
		return old.closeFile(f)
	}
	links := 0
	p.h.ops.link = func(d *os.File, a, b string) error {
		if !inside {
			t.Fatal("final publication outside account lease")
		}
		links++
		return old.link(d, a, b)
	}
	p.h.ops.remove = func(d *os.File, n string) error {
		if inside {
			t.Fatal("staging cleanup under account lease")
		}
		return old.remove(d, n)
	}
	raw := bytes.Repeat([]byte("x"), PublicEvidenceMaxSize)
	want := append([]byte(nil), raw...)
	if err := p.Stage(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	clear(raw)
	absent(t, path)
	fence := func(commit func(func() error) error) error {
		inside = true
		defer func() { inside = false }()
		return commit(func() error { return nil })
	}
	if err := p.Publish(context.Background(), fence); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) || links != 1 {
		t.Fatal("wrong public output", err, links)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("output mode", err)
	}
	noPublicTemps(t, path)
	if err := p.Publish(context.Background(), allow); !errors.Is(err, ErrClosed) {
		t.Fatal("closed publication reused", err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	// The same bytes remain forbidden to the unchanged recovery API.
	_, legacy, legacyPath := newOutput(t)
	if err := legacy.Publish(context.Background(), want, allow); !errors.Is(err, ErrInvalid) {
		t.Fatal("recovery limit widened", err)
	}
	absent(t, legacyPath)
}

func TestPublicEvidenceBoundsAndOriginalSelection(t *testing.T) {
	s, p, path := newPublicOutput(t)
	if err := p.Stage(context.Background(), make([]byte, PublicEvidenceMaxSize+1)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	absent(t, path)
	noPublicTemps(t, path)
	if _, err := OpenPublicEvidence(s.Pin(), []string{filepath.Dir(path)}); !errors.Is(err, ErrInvalid) {
		t.Fatal("protected root opened", err)
	}
	bad := s.Pin()
	bad.Mode = ReadMode
	if _, err := OpenPublicEvidence(bad, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("read mode opened", err)
	}
	if err := p.Stage(context.Background(), []byte("public")); err != nil {
		t.Fatal(err)
	}
	if err := p.Stage(context.Background(), []byte("replacement")); !errors.Is(err, ErrInvalid) {
		t.Fatal("stage reused", err)
	}
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(context.Background(), allow); !errors.Is(err, ErrConflict) || errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal("exclusive conflict", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "existing" {
		t.Fatal("existing file replaced")
	}
	if err := p.Publish(context.Background(), allow); !errors.Is(err, ErrInvalid) {
		t.Fatal("failed publication reused", err)
	}
	noPublicTemps(t, path)
}

func TestPublicEvidenceFailureBoundaries(t *testing.T) {
	for _, stage := range []string{"create", "short-write", "write", "file-sync", "file-close", "link", "post-link-error", "directory-sync", "remove", "close-after-publish"} {
		t.Run(stage, func(t *testing.T) {
			_, p, path := newPublicOutput(t)
			old := p.h.ops
			switch stage {
			case "create":
				p.h.ops.create = func(*os.File, string) (*os.File, error) { return nil, fault }
			case "short-write":
				p.h.ops.write = func(f *os.File, b []byte) (int, error) { return old.write(f, b[:len(b)-1]) }
			case "write":
				p.h.ops.write = func(*os.File, []byte) (int, error) { return 0, fault }
			case "file-sync":
				p.h.ops.syncFile = func(*os.File) error { return fault }
			case "file-close":
				p.h.ops.closeFile = func(f *os.File) error { return errors.Join(old.closeFile(f), fault) }
			}
			err := p.Stage(context.Background(), []byte("original public bytes"))
			stageFailure := stage == "create" || stage == "short-write" || stage == "write" || stage == "file-sync" || stage == "file-close"
			if stageFailure {
				if err == nil || p.staged || errors.Is(err, ErrOutcomeUnknown) {
					t.Fatal("failed stage became usable/uncertain", err)
				}
				if err := p.Publish(context.Background(), allow); !errors.Is(err, ErrInvalid) {
					t.Fatal("failed stage published", err)
				}
				absent(t, path)
				noPublicTemps(t, path)
				closeErr := p.Close()
				if errors.Is(closeErr, ErrCleanup) != (stage == "file-close") {
					t.Fatal("cleanup failure not preserved", closeErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch stage {
			case "link":
				p.h.ops.link = func(*os.File, string, string) error { return fault }
			case "post-link-error":
				p.h.ops.link = func(d *os.File, a, b string) error { return errors.Join(old.link(d, a, b), fault) }
			case "directory-sync":
				p.h.ops.syncDirectory = func(*os.File) error { return fault }
			case "remove":
				p.h.ops.remove = func(*os.File, string) error { return fault }
			}
			err = p.Publish(context.Background(), allow)
			if stage == "close-after-publish" {
				if err != nil {
					t.Fatal(err)
				}
				// Model a descriptor-close failure after known publication.
				if err := p.h.dir.Close(); err != nil {
					t.Fatal(err)
				}
				err = p.Close()
			}
			if !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatal("possible publication uncertainty lost", stage, err)
			}
			if stage == "link" {
				absent(t, path)
			} else {
				got, re := os.ReadFile(path)
				if re != nil || string(got) != "original public bytes" {
					t.Fatal("published file lost", re)
				}
			}
			p.h.ops = old
			closeErr := p.Close()
			if stage == "remove" || stage == "directory-sync" || stage == "close-after-publish" {
				if !errors.Is(closeErr, ErrCleanup) || !errors.Is(closeErr, ErrOutcomeUnknown) {
					t.Fatal("sticky cleanup missing", closeErr)
				}
			} else if closeErr != nil {
				t.Fatal(closeErr)
			}
			noPublicTemps(t, path)
		})
	}
}

func TestPublicEvidenceCloseJoinsCancelledStage(t *testing.T) {
	_, p, path := newPublicOutput(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseWork := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseWork()
	old := p.h.ops.write
	p.h.ops.write = func(f *os.File, raw []byte) (int, error) { close(entered); <-release; return old(f, raw) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	staged, closed := make(chan error, 1), make(chan error, 1)
	go func() { staged <- p.Stage(ctx, []byte("joined work")) }()
	select {
	case <-entered:
	case err := <-staged:
		t.Fatal("stage failed before blocked write", err)
	case <-time.After(2 * time.Second):
		t.Fatal("stage did not reach write")
	}
	go func() { closed <- p.Close() }()
	if err := p.Publish(context.Background(), allow); !errors.Is(err, ErrBusy) {
		t.Fatal("operation did not reserve writer", err)
	}
	select {
	case err := <-closed:
		t.Fatal("close abandoned active write", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	releaseWork()
	if err := <-staged; !errors.Is(err, context.Canceled) || errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	absent(t, path)
	noPublicTemps(t, path)
}

func TestPublicEvidenceCancellationAfterLinkIsUncertain(t *testing.T) {
	_, p, path := newPublicOutput(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := p.Stage(ctx, []byte("already linked")); err != nil {
		t.Fatal(err)
	}
	old := p.h.ops.link
	p.h.ops.link = func(d *os.File, a, b string) error { err := old(d, a, b); cancel(); return err }
	err := p.Publish(ctx, allow)
	if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "already linked" {
		t.Fatal("published bytes removed", err)
	}
	noPublicTemps(t, path)
}

func TestPublicEvidenceParentAndStagingSubstitution(t *testing.T) {
	t.Run("parent", func(t *testing.T) {
		root := canonicalTemp(t)
		parent := filepath.Join(root, "selected")
		moved := filepath.Join(root, "original")
		if err := os.Mkdir(parent, 0700); err != nil {
			t.Fatal(err)
		}
		s, err := Capture(filepath.Join(parent, "out.json"), nil, CreateMode)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		p, err := OpenPublicEvidence(s.Pin(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		if err := p.Stage(context.Background(), []byte("owned stage")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(parent, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(parent, 0700); err != nil {
			t.Fatal(err)
		}
		if err := p.Publish(context.Background(), allow); !errors.Is(err, ErrInvalid) || errors.Is(err, ErrOutcomeUnknown) {
			t.Fatal(err)
		}
		absent(t, filepath.Join(parent, "out.json"))
		absent(t, filepath.Join(moved, "out.json"))
		noPublicTemps(t, filepath.Join(moved, "out.json"))
	})
	t.Run("staging-inode", func(t *testing.T) {
		_, p, path := newPublicOutput(t)
		if err := p.Stage(context.Background(), []byte("original")); err != nil {
			t.Fatal(err)
		}
		temporary := filepath.Join(filepath.Dir(path), p.h.temporary)
		if err := os.Rename(temporary, temporary+"-owned-moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(temporary, []byte("foreign"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := p.Publish(context.Background(), allow); !errors.Is(err, ErrCleanup) || errors.Is(err, ErrOutcomeUnknown) {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(temporary); err != nil || string(got) != "foreign" {
			t.Fatal("foreign staging file deleted", err)
		}
		if err := p.Close(); !errors.Is(err, ErrCleanup) {
			t.Fatal("cleanup failure disappeared", err)
		}
		absent(t, path)
	})
}

func TestPublicEvidenceFenceCannotHideOrRepeatCommit(t *testing.T) {
	for _, kind := range []string{"omitted", "swallowed", "repeated"} {
		t.Run(kind, func(t *testing.T) {
			_, p, path := newPublicOutput(t)
			if err := p.Stage(context.Background(), []byte("fenced")); err != nil {
				t.Fatal(err)
			}
			fence := func(commit func(func() error) error) error {
				switch kind {
				case "omitted":
					return nil
				case "swallowed":
					_ = commit(func() error { return fault })
					return nil
				default:
					_ = commit(func() error { return nil })
					_ = commit(func() error { return nil })
					return nil
				}
			}
			err := p.Publish(context.Background(), fence)
			if err == nil || errors.Is(err, ErrOutcomeUnknown) != (kind == "repeated") {
				t.Fatal("broken fence accepted", err)
			}
			if kind != "repeated" {
				absent(t, path)
			}
			noPublicTemps(t, path)
		})
	}
}
