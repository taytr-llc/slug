//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/registry"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type publicReadForbiddenBody struct{ t *testing.T }

func (b publicReadForbiddenBody) Read([]byte) (int, error) {
	b.t.Error("unsupported response writer allowed a request body read")
	return 0, io.EOF
}
func (b publicReadForbiddenBody) Close() error { return nil }

func TestPublicReadServerRequiresBoundedHTTP(t *testing.T) {
	store, source, _, _ := cutoffGenesisFixture(t)
	owner, e := NewPublicReadServer(store.pin, source.publisher, store, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	r := httptest.NewRequest(http.MethodPost, publicReadPath, nil)
	r.Header.Set("Content-Type", "application/json")
	r.Body = publicReadForbiddenBody{t}
	w := httptest.NewRecorder()
	owner.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("unbounded response writer accepted")
	}
}

func TestPublicReadServerBorrowedOwners(t *testing.T) {
	store, source, _, _ := cutoffGenesisFixture(t)
	ctx := context.Background()
	owner, e := NewPublicReadServer(store.pin, source.publisher, store, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	server := httptest.NewServer(owner)
	defer server.Close()
	r, e := NewPublicReader(ctx, store.pin, PublicReadConfig{URL: server.URL + publicReadPath}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	got, e := r.Status(ctx)
	if e != nil {
		t.Fatal(e)
	}
	want := publicReadEmptyStatus(t, store.pin)
	if got != want {
		t.Fatal("unexpected initial frontier", got)
	}
	if _, e = r.Admission(ctx, 1); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("invented missing admission", e)
	}
	if _, e = r.FinalityFrame(ctx, 1); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("invented missing frame", e)
	}
	store.mu.Lock()
	_, e = r.Status(ctx)
	store.mu.Unlock()
	if !errors.Is(e, ErrBusy) {
		t.Fatal("borrowed mutex blocks", e)
	}
	request := publicReadRequest{Version: 1, PinID: store.pinID, ID: 9, Method: "status"}
	raw, _ := json.Marshal(request)
	for _, v := range []struct {
		path, method, body string
		code               int
	}{
		{publicReadPath, "GET", string(raw), 405},
		{publicReadPath + "?private=true", "POST", string(raw), 404},
		{publicReadPath, "POST", string(raw) + "\n", 400},
		{publicReadPath, "POST", strings.Replace(string(raw), `"id":9`, `"id":9,"id":9`, 1), 400},
		{publicReadPath, "POST", strings.Repeat(" ", 1025), 400},
	} {
		code, _ := publicReadHTTP(t, server.URL+v.path, v.method, v.body)
		if code != v.code {
			t.Fatal("HTTP grammar", code, v.code)
		}
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Status(ctx); !errors.Is(e, ErrClosed) {
		t.Fatal("closed original core still serves status", e)
	}
}
