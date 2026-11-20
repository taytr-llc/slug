//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var opSetupTestPrivateBytes = []byte("PUBLIC SYNTHETIC OPERATOR INPUT\n")

func opSetupTestPrivateFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(opSetupTestDir(t), "selected-input")
	if e := os.WriteFile(path, opSetupTestPrivateBytes, 0600); e != nil {
		t.Fatal(e)
	}
	return path
}
func opSetupTestSelect(t *testing.T, path string, limit int) *privateInput {
	t.Helper()
	s, e := selectPrivateInput(context.Background(), path, limit)
	if e != nil {
		if s != nil {
			_ = s.close()
		}
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.close(); e != nil {
			t.Error(e)
		}
	})
	return s
}
func opSetupTestPrivateRefused(t *testing.T, ctx context.Context, path string, limit int) {
	t.Helper()
	s, e := selectPrivateInput(ctx, path, limit)
	if s != nil {
		if ce := s.close(); ce != nil {
			t.Fatal("partial input cleanup", ce)
		}
	}
	if e == nil {
		t.Fatal("invalid private selection admitted")
	}
	if strings.Contains(e.Error(), path) || strings.Contains(e.Error(), string(opSetupTestPrivateBytes)) {
		t.Fatal("private selection disclosed in error")
	}
}

func TestOperatorPrivateInputOwnedReadAndLifetime(t *testing.T) {
	for _, mode := range []os.FileMode{0400, 0600} {
		t.Run(mode.String(), func(t *testing.T) {
			path := opSetupTestPrivateFile(t)
			if e := os.Chmod(path, mode); e != nil {
				t.Fatal(e)
			}
			selected := opSetupTestSelect(t, path, len(opSetupTestPrivateBytes))
			if selected.original.uid != uint32(os.Geteuid()) || selected.original.links != 1 {
				t.Fatal("original owner identity")
			}
			first, e := selected.read(context.Background())
			if e != nil || !bytes.Equal(first, opSetupTestPrivateBytes) {
				t.Fatal("selected read", e)
			}
			first[0] ^= 1
			second, e := selected.read(context.Background())
			if e != nil || !bytes.Equal(second, opSetupTestPrivateBytes) {
				t.Fatal("read aliases previous disclosure", e)
			}
			clear(first)
			clear(second)
			// Changes to parent contents do not change its retained identity.
			if e = os.WriteFile(filepath.Join(filepath.Dir(path), "unrelated"), []byte("public sibling"), 0600); e != nil {
				t.Fatal(e)
			}
			if e = selected.check(context.Background()); e != nil {
				t.Fatal("ordinary parent contents changed identity", e)
			}
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			if raw, e := selected.read(canceled); !errors.Is(e, context.Canceled) || raw != nil {
				t.Fatal("canceled read disclosed", e)
			}
			if e = selected.close(); e != nil || selected.close() != nil {
				t.Fatal("exact repeated close", e)
			}
			if raw, e := selected.read(context.Background()); e == nil || raw != nil {
				t.Fatal("closed selection revived")
			}
		})
	}
}
func TestOperatorPrivateInputAdmissionRefusals(t *testing.T) {
	for _, name := range []string{"empty", "oversize", "broad-file", "broad-parent", "symlink-file", "symlink-parent", "hardlink", "directory", "fifo"} {
		t.Run(name, func(t *testing.T) {
			path := opSetupTestPrivateFile(t)
			limit := len(opSetupTestPrivateBytes)
			switch name {
			case "empty":
				if e := os.WriteFile(path, nil, 0600); e != nil {
					t.Fatal(e)
				}
			case "oversize":
				limit--
			case "broad-file":
				if e := os.Chmod(path, 0644); e != nil {
					t.Fatal(e)
				}
			case "broad-parent":
				if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
					t.Fatal(e)
				}
			case "symlink-file":
				original := path + ".original"
				if e := os.Rename(path, original); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(original, path); e != nil {
					t.Fatal(e)
				}
			case "symlink-parent":
				alias := filepath.Join(opSetupTestDir(t), "alias")
				if e := os.Symlink(filepath.Dir(path), alias); e != nil {
					t.Fatal(e)
				}
				path = filepath.Join(alias, filepath.Base(path))
			case "hardlink":
				if e := os.Link(path, path+".other"); e != nil {
					t.Fatal(e)
				}
			case "directory":
				if e := os.Remove(path); e != nil {
					t.Fatal(e)
				}
				if e := os.Mkdir(path, 0700); e != nil {
					t.Fatal(e)
				}
			case "fifo":
				if e := os.Remove(path); e != nil {
					t.Fatal(e)
				}
				if e := unix.Mkfifo(path, 0600); e != nil {
					t.Fatal(e)
				}
			}
			opSetupTestPrivateRefused(t, context.Background(), path, limit)
		})
	}
	path := opSetupTestPrivateFile(t)
	for _, input := range []struct {
		path  string
		limit int
	}{{"relative", 32}, {path, 0}, {path, (1 << 20) + 1}, {path + "\x00tail", 32}, {filepath.Dir(path) + "/./selected-input", 32}} {
		opSetupTestPrivateRefused(t, context.Background(), input.path, input.limit)
	}
	opSetupTestPrivateRefused(t, nil, path, 32)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	opSetupTestPrivateRefused(t, canceled, path, 32)
	// UID policy is checked without requiring privileged chown or another
	// user's file. Successful real selection above checks the actual current UID.
	s := opSetupTestSelect(t, path, 64)
	foreign := s.original
	foreign.uid++
	if inputProtected(foreign, uint32(os.Geteuid()), 64) {
		t.Fatal("foreign UID policy accepted")
	}
}
func TestOperatorPrivateInputOriginalIdentityRefusals(t *testing.T) {
	for _, name := range []string{"append", "replace-same-bytes", "file-mode", "parent-mode", "renamed-parent"} {
		t.Run(name, func(t *testing.T) {
			path := opSetupTestPrivateFile(t)
			selected := opSetupTestSelect(t, path, 64)
			switch name {
			case "append":
				if e := os.WriteFile(path, append(append([]byte(nil), opSetupTestPrivateBytes...), byte('!')), 0600); e != nil {
					t.Fatal(e)
				}
			case "replace-same-bytes":
				if e := os.Rename(path, path+".old"); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(path, opSetupTestPrivateBytes, 0600); e != nil {
					t.Fatal(e)
				}
			case "file-mode":
				if e := os.Chmod(path, 0400); e != nil {
					t.Fatal(e)
				}
			case "parent-mode":
				if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
					t.Fatal(e)
				}
			case "renamed-parent":
				parent := filepath.Dir(path)
				if e := os.Rename(parent, parent+".old"); e != nil {
					t.Fatal(e)
				}
				defer os.RemoveAll(parent + ".old")
				if e := os.Mkdir(parent, 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(path, opSetupTestPrivateBytes, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if raw, e := selected.read(context.Background()); !errors.Is(e, ErrIntegrity) || raw != nil {
				t.Fatal("changed original selected input disclosed", e)
			}
		})
	}
}
func TestOperatorPrivateInputCleanupFailureRetained(t *testing.T) {
	for _, name := range []string{"file", "parent"} {
		t.Run(name, func(t *testing.T) {
			path := opSetupTestPrivateFile(t)
			selected, e := selectPrivateInput(context.Background(), path, 64)
			if e != nil {
				opSetupTestForcePrivateClose(selected)
				t.Fatal(e)
			}
			defer opSetupTestForcePrivateClose(selected)
			failed := selected.file
			if name == "parent" {
				failed = selected.dirs[len(selected.dirs)-1].file
			}
			if e = failed.Close(); e != nil {
				t.Fatal(e)
			}
			first := selected.close()
			if !errors.Is(first, ErrCleanup) || selected.close() != first {
				t.Fatal("ambiguous cleanup relabeled successful", first)
			}
			found := selected.file == failed
			for _, d := range selected.dirs {
				found = found || d.file == failed
			}
			if !found {
				t.Fatal("original failed descriptor owner discarded")
			}
			if raw, e := selected.read(context.Background()); e == nil || raw != nil {
				t.Fatal("failed cleanup selection reused")
			}
		})
	}
}

// This actual Darwin ACL check is intentionally not inferred from SDK symbols.
// Linux's ACL platform execution remains its own qualification gate.
func opSetupTestACLCommand(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/bin/chmod", args...).CombinedOutput()
}
func TestOperatorPrivateInputDarwinACLRefusal(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("actual Darwin ACL semantics")
	}
	for _, name := range []string{"file-before", "parent-before", "file-after"} {
		t.Run(name, func(t *testing.T) {
			path := opSetupTestPrivateFile(t)
			target := path
			if name == "parent-before" {
				target = filepath.Dir(path)
			}
			var selected *privateInput
			if name == "file-after" {
				selected = opSetupTestSelect(t, path, 64)
			}
			// Only a disposable file containing the literal public fixture is affected.
			if output, e := opSetupTestACLCommand("+a", "everyone allow read", target); e != nil {
				t.Fatalf("synthetic ACL setup failed: %v (%d diagnostic bytes)", e, len(output))
			}
			t.Cleanup(func() {
				if output, e := opSetupTestACLCommand("-N", target); e != nil {
					t.Errorf("synthetic ACL cleanup failed: %v (%d diagnostic bytes)", e, len(output))
				}
			})
			if selected == nil {
				opSetupTestPrivateRefused(t, context.Background(), path, 64)
			} else if raw, e := selected.read(context.Background()); !errors.Is(e, ErrIntegrity) || raw != nil {
				t.Fatal("post-selection ACL not refused", e)
			}
		})
	}
}
