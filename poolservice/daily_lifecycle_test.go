// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDailyServerBusyAndRunJoins(t *testing.T) {
	h := newDailyServerHarness(t)
	h.node.resolveEntered = make(chan struct{}, 1)
	h.node.resolveRelease = make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(h.node.resolveRelease) }) }
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan string, 1)
	go func() { done <- h.server.Run(ctx, func(origin string) { ready <- origin }) }()
	var origin string
	select {
	case origin = <-ready:
	case e := <-done:
		t.Fatal(e)
	case <-time.After(3 * time.Second):
		t.Fatal("service readiness deadline")
	}
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("service did not drain")
		}
	}()
	request := func(route string) (int, error) {
		r, _ := http.NewRequest(http.MethodPost, origin+dailyPathPrefix+route, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+serverTokenA)
		res, e := (&http.Client{Timeout: 4 * time.Second}).Do(r)
		if e != nil {
			return 0, e
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode, nil
	}
	requestDone := make(chan error, 1)
	go func() { _, e := request("issue"); requestDone <- e }()
	select {
	case <-h.node.resolveEntered:
	case e := <-requestDone:
		t.Fatal(e)
	case <-time.After(3 * time.Second):
		t.Fatal("fixture operation did not enter")
	}
	status, e := request("balance")
	if e != nil || status != 503 {
		t.Fatalf("second request queued behind active account lease: status=%d err=%v", status, e)
	}
	if h.sessions.nested.Load() != 0 {
		t.Fatal("busy request reached a nested lease")
	}
	cancel()
	select {
	case e := <-done:
		t.Fatalf("Run returned before actual callback drained: %v", e)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case <-requestDone:
	case <-time.After(3 * time.Second):
		t.Fatal("request failed to drain")
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
		done <- nil
	case <-time.After(3 * time.Second):
		t.Fatal("Run failed to drain")
	}
	if e := h.server.Run(context.Background(), nil); e == nil {
		t.Fatal("stopped server restarted")
	}
}
