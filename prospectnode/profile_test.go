//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectnode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/prospect"
)

func TestCheapProfileRefusesBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	if _, err := NewClient(RPCConfig{Version: 1, URL: server.URL, Username: "test", Password: "secret"}); err == nil || !strings.Contains(err.Error(), "slugtestpow") {
		t.Fatal("cheap client constructor was not refused")
	}
	client := &Client{endpoint: server.URL, http: server.Client()}
	_, resolveErr := client.Resolve(context.Background(), testPolicy())
	_, _, verifyErr := client.Verify(context.Background(), testPolicy(), prospect.Proof{})
	recheckErr := client.Recheck(context.Background(), prospect.Context{}, prospect.Proof{})
	for _, err := range []error{resolveErr, verifyErr, recheckErr} {
		if err == nil || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatal("cheap verification was not refused")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("cheap profile contacted node")
	}
}
