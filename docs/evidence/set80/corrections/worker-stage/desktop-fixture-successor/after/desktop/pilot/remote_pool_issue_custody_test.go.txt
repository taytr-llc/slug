//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecoverableRemoteProtectedOriginalLeaseAndMarkers(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal selected owner")
	}
	r := remoteIssueReference(t)
	v := r.Fixtures.Remote[0]
	c := v.Config
	dir := remoteAttachmentTestDir(t)
	s := remotePoolCredentialSelection{filepath.Join(dir, "pool.json"), filepath.Join(dir, "admission.json")}
	for _, record := range v.Credentials {
		// This live ownership test needs current credentials; the separate pure
		// reference test preserves the frozen credential bytes and their expiry.
		record.Base.ExpiresAtMS = time.Now().Add(time.Hour).UnixMilli()
		path := s.PoolPath
		if record.Base.Audience == "admission" {
			path = s.AdmissionPath
		}
		raw, _ := json.Marshal(record)
		if e := os.WriteFile(path, append(raw, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
	old, e := openSelectedRemotePoolCredentials(context.Background(), c.Base, s, true)
	if old != nil {
		if ce := old.Close(); ce != nil {
			t.Fatal(ce)
		}
	}
	if e == nil {
		t.Fatal("old credential owner adopted wrapper")
	}
	life, cancel := context.WithCancel(context.Background())
	defer cancel()
	o, e := openRemotePoolCredentialsSelection(life, c.Base, s, true, &c)
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
	p, e := o.beginPool(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	wc, e := remotePoolRecoverableWorkerConfig(c)
	if e != nil || p.ProtocolVersion() != 3 || p.TransportTrustID() != wc.TransportTrustID || p.lease.pool.CheckRecoverableDailySelection(poolservice.RecoverableDailySelection{Version: 1, BaseVersion: 2, URL: c.Base.Origin, PublicKey: wc.Base.PublicKey, Source: c.Base.Source, IssueProfile: c.IssueProfile, SettlementConfigID: wc.SettlementConfigID, RecipientScript: wc.RecipientScript, BaseTransportTrustID: wc.Base.TransportTrustID, TransportTrustID: wc.TransportTrustID}) != nil {
		t.Fatal("original concrete client selection", e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	result := make(chan error, 1)
	go func() {
		_, e := remotePoolLeaseCall(context.Background(), p.lease, true, func(ctx context.Context) (int, error) { close(entered); <-ctx.Done(); <-release; return 1, nil })
		result <- e
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("call not entered")
	}
	closed := make(chan error, 1)
	go func() { closed <- o.Close() }()
	select {
	case <-closed:
		t.Fatal("close passed live original call")
	case <-time.After(10 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case e := <-result:
		var ce *poolservice.ClientError
		if !errors.As(e, &ce) || !ce.OutcomeUnknown {
			t.Fatal("mutation cancellation provenance", e)
		}
	case <-time.After(time.Second):
		t.Fatal("call not joined")
	}
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("owner not joined")
	}
	if _, e := p.ResumeIssue(context.Background(), "invalid"); e == nil {
		t.Fatal("closed lease revived")
	}
	path := filepath.Join(dir, "attachment")
	f, e := openRemotePoolFilesSelection(path, c.Base, true, &c)
	if e != nil {
		if f != nil {
			_ = f.close()
		}
		t.Fatal(e)
	}
	remoteAttachmentTestCompleteFilesystem(t, f)
	if e = f.markReady(); e != nil {
		t.Fatal(e)
	}
	if e = f.close(); e != nil {
		t.Fatal(e)
	}
	legacy, e := openRemotePoolFiles(path, c.Base, false)
	if legacy != nil {
		if ce := legacy.close(); ce != nil {
			t.Fatal(ce)
		}
	}
	if e == nil {
		t.Fatal("old marker owner adopted selected root")
	}
	f, e = openRemotePoolFilesSelection(path, c.Base, false, &c)
	if e != nil {
		if f != nil {
			_ = f.close()
		}
		t.Fatal(e)
	}
	if e = f.close(); e != nil {
		t.Fatal(e)
	}
}
