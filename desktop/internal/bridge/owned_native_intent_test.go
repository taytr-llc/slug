// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
)

// This models only native correlation storage. It cannot authorize a wallet
// operation, and none of these setup IDs is a source of cryptographic vectors.
func ownedIntentTestContext(t *testing.T) (*OwnedBridge, *OwnedContext, string) {
	t.Helper()
	f, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		t.Fatal(e)
	}
	to, e := gymaccount.FixtureIdentity("incrementing256")
	if e != nil {
		t.Fatal(e)
	}
	b := ownedWireTestBridge(&ownedWireTestWriter{closed: make(chan struct{})})
	v := &OwnedContext{helper: b, view: pilot.OwnedGymView{ViewID: strings.Repeat("1", 64), AccountID: strings.Repeat("2", 64), Generation: 1, GymPublicKey: f.GymPublicKey}}
	v.scope.SourceBindingID = strings.Repeat("3", 64)
	v.scope.CarrierBindingID = strings.Repeat("4", 64)
	v.scope.Carrier.Pin.OwnershipPinID = strings.Repeat("5", 64)
	v.scope.Carrier.Pin.AccountID = v.view.AccountID
	b.view = v
	return b, v, to.GymPublicKey
}
func TestOwnedNativeIntentDurableBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(parent, "native")
	l, e := OpenOwnedIntentLog(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	b, original, to := ownedIntentTestContext(t)
	r, e := l.Prepare(ctx, original, strings.Repeat("6", 64), to)
	if e != nil {
		t.Fatal(e)
	}
	st, e := os.Stat(filepath.Join(dir, r.RequestID+".json"))
	if e != nil || st.Mode().Perm() != 0600 {
		t.Fatal("private persisted intent", e)
	}
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	l, e = OpenOwnedIntentLog(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	got, e := l.Lookup(original, r.RequestID)
	if e != nil || !reflect.DeepEqual(got, r) {
		t.Fatal("lost original correlation", e)
	}
	next := *original
	next.view.ViewID = strings.Repeat("7", 64)
	next.view.Generation = 2
	b.view = &next
	if _, e = l.List(original); e == nil {
		t.Fatal("revoked original view disclosed")
	}
	if got, e = l.Lookup(&next, r.RequestID); e != nil || got != r {
		t.Fatal("new original login could not inspect same request", e)
	}
	foreign := next
	foreign.scope.CarrierBindingID = strings.Repeat("8", 64)
	b.view = &foreign
	if _, e = l.Lookup(&foreign, r.RequestID); e == nil {
		t.Fatal("foreign carrier adopted request")
	}
}
func TestOwnedNativeIntentRefusesStorageReplacement(t *testing.T) {
	ctx := context.Background()
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(parent, "native")
	l, e := OpenOwnedIntentLog(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	_, v, to := ownedIntentTestContext(t)
	if e = os.Rename(dir, dir+"-old"); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if _, e = l.Prepare(ctx, v, strings.Repeat("6", 64), to); e == nil {
		t.Fatal("replacement directory adopted")
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 0 {
		t.Fatal("replacement was written", e)
	}
}
func TestOwnedNativeIntentCorruptionRefusesReopen(t *testing.T) {
	for _, mode := range []string{"extra", "mismatched-id", "symlink", "unknown-file"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			parent, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			dir := filepath.Join(parent, "native")
			l, e := OpenOwnedIntentLog(ctx, dir)
			if e != nil {
				t.Fatal(e)
			}
			_, v, to := ownedIntentTestContext(t)
			r, e := l.Prepare(ctx, v, strings.Repeat("6", 64), to)
			if e != nil {
				t.Fatal(e)
			}
			if e = l.Close(); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(dir, r.RequestID+".json")
			switch mode {
			case "extra":
				raw, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				raw = append(raw[:len(raw)-1], []byte(`,"approval":true}`)...)
				e = os.WriteFile(path, raw, 0600)
				if e != nil {
					t.Fatal(e)
				}
			case "mismatched-id":
				r.RequestID = strings.Repeat("9", 64)
				raw, _ := json.Marshal(r)
				if e = os.WriteFile(path, raw, 0600); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				if e = os.Rename(path, path+".saved"); e != nil {
					t.Fatal(e)
				}
				if e = os.Symlink(path+".saved", path); e != nil {
					t.Fatal(e)
				}
			case "unknown-file":
				if e = os.WriteFile(filepath.Join(dir, "unrecognized"), []byte("x"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = OpenOwnedIntentLog(ctx, dir); e == nil {
				t.Fatal("corrupt correlation directory accepted")
			}
		})
	}
}

// A failed close can become a no-op on retry; the original error and handles
// must survive native shutdown rather than being replaced by that nil.
type ownedIntentCloseFailure struct {
	calls int
	err   error
}

func (c *ownedIntentCloseFailure) Close() error {
	c.calls++
	if c.calls == 1 {
		return c.err
	}
	return nil
}
func TestOwnedNativeIntentRetainsFailedCleanup(t *testing.T) {
	failure := errors.New("descriptor close failed")
	owner := &ownedIntentCloseFailure{err: failure}
	log := &OwnedIntentLog{}
	if e := log.finishClose(owner); !errors.Is(e, failure) {
		t.Fatal("lost initial failure", e)
	}
	if !log.uncertain || len(log.cleanup) != 1 || log.cleanup[0] != owner {
		t.Fatal("lost exact cleanup owner")
	}
	for n := 0; n < 2; n++ {
		if e := log.Close(); !errors.Is(e, failure) {
			t.Fatal("close laundered previous failure", e)
		}
	}
	if owner.calls != 1 {
		t.Fatal("retried unknown descriptor close")
	}
	_, original, to := ownedIntentTestContext(t)
	if _, e := log.List(original); e == nil {
		t.Fatal("poisoned owner disclosed")
	}
	if _, e := log.Prepare(context.Background(), original, strings.Repeat("6", 64), to); e == nil {
		t.Fatal("poisoned owner wrote replacement")
	}
}
