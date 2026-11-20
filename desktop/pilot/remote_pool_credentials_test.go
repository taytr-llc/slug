//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

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

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
)

func remotePoolTestFiles(t *testing.T) (RemotePoolConfig, remotePoolCredentialSelection) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal profile custody")
	}
	r := remotePoolTestReference(t)
	c := r.Positives[0].Config
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	s := remotePoolCredentialSelection{filepath.Join(dir, "pool.json"), filepath.Join(dir, "admission.json")}
	for _, v := range r.Credentials {
		if v.ConfigID != r.Positives[0].ConfigID {
			continue
		}
		path := s.PoolPath
		if v.Audience == "admission" {
			path = s.AdmissionPath
		}
		if e = os.WriteFile(path, remotePoolTestHex(t, v.CanonicalHex), 0600); e != nil {
			t.Fatal(e)
		}
	}
	return c, s
}
func remotePoolTestOpen(t *testing.T, c RemotePoolConfig, s remotePoolCredentialSelection) *remotePoolCredentials {
	t.Helper()
	o, e := openRemotePoolCredentials(context.Background(), c, s)
	if e != nil {
		if o != nil {
			_ = o.Close()
		}
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := o.Close(); e != nil {
			t.Error(e)
		}
	})
	return o
}
func TestRemotePoolCredentialsOriginalLeasesAndReplacement(t *testing.T) {
	c, s := remotePoolTestFiles(t)
	o := remotePoolTestOpen(t, c, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, e := o.beginPool(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	a, e := o.beginAdmission(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if _, e = o.beginPool(context.Background()); !errors.Is(e, errRemotePoolCredentialBusy) {
		t.Fatal("overlapping audience admitted", e)
	}
	if p.PublicKey() != c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey || p.SourceConfig() != c.Source || a.AuthorityPin() != c.Source.AuthorityPin || p.AuthorityOrigin() != c.Origin || a.AuthorityOrigin() != c.Origin {
		t.Fatal("original public binding")
	}
	if _, e = a.IssueManifestJob(context.Background(), p.PublicKey()); e == nil {
		t.Fatal("admission issued work")
	}
	c.Miners.Enrollments[0].PayeeScript = "changed caller copy"
	if o.config.Miners.Enrollments[0].PayeeScript == c.Miners.Enrollments[0].PayeeScript {
		t.Fatal("caller slice alias")
	}
	cancel()
	called := false
	_, e = remotePoolLeaseCall(context.Background(), p.lease, false, func(context.Context) (int, error) { called = true; return 1, nil })
	if e == nil || called {
		t.Fatal("original cancelled lifetime rescued")
	}
	_ = p.Close()
	_ = a.Close()
	if e = o.Close(); e != nil {
		t.Fatal(e)
	}
	c = remotePoolTestReference(t).Positives[0].Config
	next := remotePoolTestOpen(t, c, s)
	if next == o {
		t.Fatal("replacement pointer")
	}
	_, e = remotePoolLeaseCall(context.Background(), p.lease, false, func(context.Context) (int, error) { called = true; return 1, nil })
	if e == nil || called {
		t.Fatal("new owner revived old lease")
	}
}
func TestRemotePoolCredentialsCloseJoinsAndKeepsUnknown(t *testing.T) {
	for _, audience := range []int{0, 1} {
		name := "pool"
		if audience == 1 {
			name = "admission"
		}
		t.Run(name, func(t *testing.T) {
			c, s := remotePoolTestFiles(t)
			o := remotePoolTestOpen(t, c, s)
			lease, e := o.begin(context.Background(), audience)
			if e != nil {
				t.Fatal(e)
			}
			entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); _ = o.Close() })
			callDone := make(chan error, 1)
			go func() {
				_, e := remotePoolLeaseCall(context.Background(), lease, true, func(ctx context.Context) (int, error) {
					close(entered)
					<-ctx.Done()
					close(cancelled)
					<-release
					return 1, nil
				})
				callDone <- e
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("call not admitted")
			}
			closeDone := make(chan error, 1)
			go func() { closeDone <- o.Close() }()
			select {
			case <-cancelled:
			case <-time.After(2 * time.Second):
				t.Fatal("close did not cancel")
			}
			select {
			case <-closeDone:
				t.Fatal("close did not join original call")
			default:
			}
			if _, e = o.slots[audience].file.file.Stat(); e != nil {
				t.Fatal("descriptor closed before admitted call joined")
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case e = <-callDone:
				if audience == 0 {
					var ce *poolservice.ClientError
					if !errors.As(e, &ce) || !ce.OutcomeUnknown || ce.Code != "outcome_unknown" {
						t.Fatal("pool mutation uncertainty", e)
					}
				} else {
					var ce *prospectservice.ClientError
					if !errors.As(e, &ce) || !ce.OutcomeUnknown || ce.Code != "outcome_unknown" {
						t.Fatal("admission mutation uncertainty", e)
					}
				}
			case <-time.After(2 * time.Second):
				t.Fatal("call not joined")
			}
			select {
			case e = <-closeDone:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("owner not joined")
			}
			if _, e = o.begin(context.Background(), audience); e == nil {
				t.Fatal("closed owner reopened")
			}
		})
	}
}

func TestRemotePoolCredentialsSelectedFileRefusals(t *testing.T) {
	for _, mode := range []string{"audience", "same-token", "expired", "permission", "symlink", "mutated-after-open"} {
		t.Run(mode, func(t *testing.T) {
			c, s := remotePoolTestFiles(t)
			if mode == "mutated-after-open" {
				o := remotePoolTestOpen(t, c, s)
				p, e := o.beginPool(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				defer p.Close()
				old, e := os.ReadFile(s.PoolPath)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(s.PoolPath, append(old, ' '), 0600); e != nil {
					t.Fatal(e)
				}
				called := false
				_, e = remotePoolLeaseCall(context.Background(), p.lease, false, func(context.Context) (int, error) { called = true; return 0, nil })
				if e == nil || called {
					t.Fatal("changed selected file used")
				}
				return
			}
			path := s.PoolPath
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "audience":
				b = bytes.Replace(b, []byte(`"audience":"pool"`), []byte(`"audience":"admission"`), 1)
			case "same-token":
				path = s.AdmissionPath
				b, e = os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				b = bytes.Replace(b, []byte(strings.Repeat("98", 32)), []byte(strings.Repeat("97", 32)), 1)
			case "expired":
				b = bytes.Replace(b, []byte(`"expires_at_ms":2000000000000`), []byte(`"expires_at_ms":1`), 1)
			case "permission":
				if e = os.Chmod(path, 0644); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				other := path + "-original"
				if e = os.Rename(path, other); e != nil {
					t.Fatal(e)
				}
				if e = os.Symlink(other, path); e != nil {
					t.Fatal(e)
				}
			}
			if mode != "permission" && mode != "symlink" {
				if e = os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			o, e := openRemotePoolCredentials(context.Background(), c, s)
			if o != nil {
				if ce := o.Close(); ce != nil {
					t.Fatal("unexpected cleanup", ce)
				}
			}
			if e == nil || strings.Contains(e.Error(), path) || strings.Contains(e.Error(), strings.Repeat("97", 32)) {
				t.Fatal("refusal or private error boundary", e)
			}
		})
	}
}
func TestRemotePoolCredentialsStickyDescriptorClose(t *testing.T) {
	c, s := remotePoolTestFiles(t)
	o, e := openRemotePoolCredentials(context.Background(), c, s)
	if e != nil {
		t.Fatal(e)
	}
	f := o.slots[0].file
	if e = f.file.Close(); e != nil {
		t.Fatal(e)
	}
	if e = o.Close(); !errors.Is(e, errRemotePoolCredentialCleanup) {
		t.Fatal("failed close not retained", e)
	}
	if o.slots[0].file != f || !errors.Is(o.Close(), errRemotePoolCredentialCleanup) {
		t.Fatal("original sticky cleanup owner lost")
	}
}
