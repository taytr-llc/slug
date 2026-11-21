//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectservice"
)

func remoteDailyFiles(t *testing.T) (RemotePoolConfig, remotePoolCredentialSelection) {
	t.Helper()
	dir := remoteAttachmentTestDir(t)
	r := remoteDailyOracle(t)
	v := r.Transport.Positives[0]
	s := remotePoolCredentialSelection{filepath.Join(dir, "pool.json"), filepath.Join(dir, "admission.json")}
	for _, x := range r.Transport.Credentials {
		if !x.Valid || x.Value.ConfigID != v.ConfigID {
			continue
		}
		path := s.PoolPath
		if x.Value.Audience == "admission" {
			path = s.AdmissionPath
		}
		if e := os.WriteFile(path, remotePoolTestHex(t, x.FileHex), 0600); e != nil {
			t.Fatal(e)
		}
	}
	return v.Config, s
}
func TestDailyRemoteProtectedOriginalAudienceAndCancellation(t *testing.T) {
	c, s := remoteDailyFiles(t)
	if old, e := openRemotePoolCredentials(context.Background(), c, s); e == nil || old != nil {
		if old != nil {
			_ = old.Close()
		}
		t.Fatal("legacy credential owner adopted daily")
	}
	for _, audience := range []int{0, 1} {
		t.Run([]string{"pool", "admission"}[audience], func(t *testing.T) {
			life, cancel := context.WithCancel(context.Background())
			defer cancel()
			o, e := openSelectedRemotePoolCredentials(life, c, s, true)
			if e != nil {
				if o != nil {
					_ = o.Close()
				}
				t.Fatal(e)
			}
			defer func() {
				if e := o.Close(); e != nil {
					t.Error(e)
				}
			}()
			l, e := o.begin(context.Background(), audience)
			if e != nil {
				t.Fatal(e)
			}
			if audience == 0 && (l.pool.ProtocolVersion() != 2 || l.pool.SourceConfig() != c.Source) {
				t.Fatal("daily concrete client not selected")
			}
			if _, e = o.begin(context.Background(), audience); !errors.Is(e, errRemotePoolCredentialBusy) {
				t.Fatal("overlapping original audience", e)
			}
			entered := make(chan struct{})
			released := make(chan struct{})
			var release sync.Once
			defer release.Do(func() { close(released) })
			result := make(chan error, 1)
			go func() {
				_, e := remotePoolLeaseCall(context.Background(), l, true, func(ctx context.Context) (int, error) { close(entered); <-ctx.Done(); <-released; return 1, nil })
				result <- e
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("call did not start")
			}
			closed := make(chan error, 1)
			go func() { closed <- o.Close() }()
			select {
			case <-closed:
				t.Fatal("owner close passed admitted operation")
			case <-time.After(10 * time.Millisecond):
			}
			release.Do(func() { close(released) })
			select {
			case e := <-result:
				if audience == 0 {
					f, ok := e.(*poolservice.ClientError)
					if !ok || !f.OutcomeUnknown || f.Code != "outcome_unknown" {
						t.Fatal("pool uncertainty", e)
					}
				} else {
					f, ok := e.(*prospectservice.ClientError)
					if !ok || !f.OutcomeUnknown || f.Code != "outcome_unknown" {
						t.Fatal("admission uncertainty", e)
					}
				}
			case <-time.After(time.Second):
				t.Fatal("call did not join")
			}
			select {
			case e := <-closed:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(time.Second):
				t.Fatal("close did not join")
			}
			called := false
			_, e = remotePoolLeaseCall(context.Background(), l, false, func(context.Context) (int, error) { called = true; return 1, nil })
			if e == nil || called {
				t.Fatal("closed original lease revived")
			}
		})
	}
}

func TestDailyRemoteMarkersAndExactOutboxRemainSeparate(t *testing.T) {
	r := remoteDailyOracle(t)
	c := r.Transport.Positives[0].Config
	parent := remoteAttachmentTestDir(t)
	path := filepath.Join(parent, "attachment")
	f := remoteAttachmentTestOpenFiles(t, path, c, true)
	remoteAttachmentTestCompleteFilesystem(t, f)
	if e := f.markReady(); e != nil {
		t.Fatal(e)
	}
	intent, e := os.ReadFile(filepath.Join(path, remotePoolIntentName))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(intent, remotePoolTestHex(t, r.Transport.Positives[0].IntentHex)) {
		t.Fatal("wrong daily intent")
	}
	if e = f.close(); e != nil {
		t.Fatal(e)
	}
	f = remoteAttachmentTestOpenFiles(t, path, c, false)
	if e = f.close(); e != nil {
		t.Fatal(e)
	}
	changed := c
	changed.MaxRecords++
	remoteAttachmentTestRefused(t, path, changed, false)
	legacy := remotePoolTestReference(t).Positives[0].Config
	remoteAttachmentTestRefused(t, path, legacy, false)
	// The original source/recipient public bytes are kept verbatim even though
	// the remote credential and attachment marker versions are daily.
	before, _ := json.Marshal(c.ExpectedLocal)
	after, _ := json.Marshal(r.Transport.Positives[1].Config.ExpectedLocal)
	if !bytes.Equal(before, after) {
		t.Fatal("limits changed original local owner")
	}
}
