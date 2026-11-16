//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package accountpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

type continuationNode struct {
	endpoint, username, password string
	client                       *http.Client
}

type continuationChainState struct {
	Blocks  int32  `json:"blocks"`
	Headers int32  `json:"headers"`
	Hash    string `json:"bestblockhash"`
}

// Only the private nodes created by this test are read. Credentials remain
// inside the test process, never in logs or the public evidence report.
func continuationRPC(t *testing.T, root, id, name string) continuationNode {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "gyms", id, name+".conf"))
	if err != nil || len(raw) > 8192 {
		t.Fatal("owned fixture node configuration unavailable")
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	if !strings.HasPrefix(values["rpclisten"], "127.0.0.1:") || values["rpcuser"] == "" || values["rpcpass"] == "" {
		t.Fatal("fixture RPC is not explicit loopback")
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return continuationNode{endpoint: "http://" + values["rpclisten"], username: values["rpcuser"], password: values["rpcpass"], client: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (n continuationNode) call(ctx context.Context, method string, params []any, result any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.SetBasicAuth(n.username, n.password)
	request.Header.Set("Content-Type", "application/json")
	response, err := n.client.Do(request)
	if err != nil {
		return errors.New("fixture node RPC unavailable")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 || response.StatusCode != http.StatusOK {
		return errors.New("fixture node RPC response unavailable")
	}
	var envelope struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.ID != 1 || string(envelope.Error) != "null" {
		return errors.New("fixture node RPC failed")
	}
	return json.Unmarshal(envelope.Result, result)
}

func continuationTipsAgree(primary, peer continuationNode) (continuationChainState, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var first, other continuationChainState
	if primary.call(ctx, "getblockchaininfo", []any{}, &first) != nil || first.Blocks < 1 || first.Headers != first.Blocks {
		return first, false
	}
	var canonical string
	if primary.call(ctx, "getblockheader", []any{first.Hash, false}, &canonical) != nil {
		return first, false
	}
	var headers []string
	if primary.call(ctx, "getheaders", []any{[]string{gymaccount.FixtureGenesis}, strings.Repeat("0", 64)}, &headers) != nil || len(headers) != int(first.Blocks) || headers[len(headers)-1] != canonical {
		return first, false
	}
	if peer.call(ctx, "getblockchaininfo", []any{}, &other) != nil || other != first {
		return first, false
	}
	var last struct {
		Hash   string `json:"hash"`
		Height int32  `json:"height"`
	}
	if primary.call(ctx, "getbestblock", []any{}, &last) != nil || last.Hash != first.Hash || last.Height != first.Blocks {
		return first, false
	}
	return first, true
}

// Start from the GUI's exact ordering: enroll at genesis, reset, then Start
// during the hold. Require a new parent/job and multiple subsequent receipts,
// not merely one successful receipt followed by a preparing retry loop.
func TestAccountPilotContinuesAfterCanonicalWinNormalPoW(t *testing.T) {
	binary := os.Getenv("SLUG_DESKTOP_NODE")
	if binary == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an immutable normal node")
	}
	root := t.TempDir()
	if artifacts := os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"); artifacts != "" {
		var err error
		root, err = os.MkdirTemp(artifacts, "account-continuation-")
		if err != nil {
			t.Fatal(err)
		}
		t.Log("public-fixture continuation artifacts", root)
	}
	config := pilot.Config{DataDir: filepath.Join(root, "LocalAccountPilot"), NodeBinary: binary}
	c, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
		checkAccountListenersClosed(t, config.DataDir)
	})
	if err := c.LoginFixture("zero256"); err != nil {
		t.Fatal(err)
	}
	initial := waitAccount(t, c, 40*time.Second, accountSignedIn)
	if initial.Attempts != 0 || initial.Height != 0 {
		t.Fatal("enrollment mined before Start")
	}
	if err := c.ResetFixtureAccount(); err != nil {
		t.Fatal(err)
	}
	held := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool {
		return accountSignedIn(s) && s.Account.Generation > initial.Account.Generation
	})
	if !held.Account.Held || held.Attempts != 0 || held.Height != 0 {
		t.Fatal("reset did not preserve idle genesis wallet with hold")
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "running" || s.ProspectCount > 0 })
	primary := continuationRPC(t, config.DataDir, held.Account.ID, "primary")
	peer := continuationRPC(t, config.DataDir, held.Account.ID, "peer")
	deadline := time.Now().Add(90 * time.Second)
	seen := make(map[uint64]registry.Record)
	var firstWin registry.Record
	var lastCount uint64
	var lastError string
	for {
		state := c.Snapshot()
		if state.State == "failed" || state.Account == nil || state.Account.State == "failed" {
			t.Fatalf("continuation failed: %+v", state)
		}
		if state.ProspectCount != lastCount || state.LastError != lastError {
			t.Logf("continuation attempts=%d height=%d receipts=%d error=%q recovery=%q", state.Attempts, state.Height, state.ProspectCount, state.LastError, state.RecoveryError)
			lastCount, lastError = state.ProspectCount, state.LastError
		}
		for _, summary := range state.Prospects {
			if _, exists := seen[summary.Sequence]; exists {
				continue
			}
			record, err := c.Receipt(summary.Sequence)
			if err != nil {
				t.Fatal(err)
			}
			seen[summary.Sequence] = record
			if record.Eligibility != nil && record.Eligibility.Source == "canonical-winning-block" &&
				(firstWin.Sequence == 0 || record.Sequence < firstWin.Sequence) {
				firstWin = record
			}
		}
		after := 0
		for sequence, record := range seen {
			if firstWin.Sequence > 0 && sequence > firstWin.Sequence && record.Job != nil && firstWin.Job != nil &&
				record.Job.ID != firstWin.Job.ID && record.Context.ParentHeight >= firstWin.Eligibility.Height {
				after++
			}
		}
		var consistent continuationChainState
		caughtUp := false
		if after >= 3 {
			consistent, caughtUp = continuationTipsAgree(primary, peer)
		}
		if after >= 3 && caughtUp {
			c.Stop()
			stopped := waitAccount(t, c, 30*time.Second, func(s pilot.Snapshot) bool { return s.State == "stopped" })
			if stopped.Account.HoldUntilMS != held.Account.HoldUntilMS || !stopped.Account.Held {
				t.Fatal("continuation changed reset hold")
			}
			result := map[string]any{"first_win_sequence": firstWin.Sequence, "first_win_hash": firstWin.Verified.WorkHash,
				"first_win_height": firstWin.Eligibility.Height, "later_new_job_receipts": after,
				"final_height": stopped.Height, "final_receipts": stopped.ProspectCount, "final_attempts": stopped.Attempts,
				"hold_until_ms": held.Account.HoldUntilMS, "last_error": stopped.LastError,
				"coherent_primary_peer_height": consistent.Blocks, "coherent_primary_peer_hash": consistent.Hash,
				"actual_headers_match_full_tip": true, "getheaders_matches_canonical_tip": true}
			raw, _ := json.MarshalIndent(result, "", "  ")
			if err := os.WriteFile(filepath.Join(root, "result.json"), append(raw, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			t.Logf("continued across canonical block %d into %d later receipts on new jobs; peer and actual header/full tips agree at height%d", firstWin.Eligibility.Height, after, consistent.Blocks)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("did not continue after canonical win: %+v; first win=%d later receipts=%d", state, firstWin.Sequence, after)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
