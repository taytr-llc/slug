// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

type workFixture struct {
	Job           prospectjob.Job       `json:"job"`
	Template      prospectwork.Template `json:"template"`
	PayoutAddress string                `json:"payout_address"`
}

func workCommandFiles(t *testing.T, reject bool) ([]string, prospectjob.Job, *atomic.Int64) {
	t.Helper()
	data, err := os.ReadFile("../../prospectwork/testdata/templates_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []workFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	f := fixtures[0]
	// A current synthetic job tests command behavior. Independent fixed-time
	// vectors separately pin the job ID and complete block bytes.
	job, err := prospectjob.New(f.Job.Context, f.Job.PublicKey, f.Job.Challenge, time.Now().UnixMilli()-1000)
	if err != nil {
		t.Fatal(err)
	}
	proposals := new(atomic.Int64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     uint64            `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var result any
		c := f.Template.Context
		switch req.Method {
		case "getcurrentnet":
			result = c.Network
		case "getblockhash":
			result = c.Genesis
		case "getblockchaininfo":
			result = map[string]any{"chain": "mainnet", "blocks": c.ParentHeight, "bestblockhash": c.ParentHash, "mediantime": c.MinTimestamp - 1}
		case "getbestblock":
			result = map[string]any{"hash": c.ParentHash, "height": c.ParentHeight}
		case "getblocktemplate":
			var param struct {
				Mode string `json:"mode"`
				Data string `json:"data"`
			}
			if len(req.Params) != 1 || json.Unmarshal(req.Params[0], &param) != nil {
				t.Error("invalid GBT request")
				return
			}
			if param.Mode == "proposal" {
				proposals.Add(1)
				if len(param.Data) < 160 {
					t.Error("missing full candidate")
				}
				if reject {
					result = "secret-password <html>reject</html>"
				}
			} else if param.Mode == "template" {
				result = f.Template.Block
			} else {
				t.Error("unexpected GBT mode")
			}
		default:
			t.Errorf("unexpected RPC (must never submit a block): %q", req.Method)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": result, "error": nil})
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	files := map[string]any{
		"job":    job,
		"policy": prospectnode.Policy{Version: 1, Network: job.Context.Network, Genesis: job.Context.Genesis, DNAVersion: job.Context.DNAVersion, ProspectTarget: job.Context.ProspectTarget},
		"rpc":    prospectnode.RPCConfig{Version: 1, URL: server.URL, Username: "fixture-user", Password: "secret-password"},
	}
	for name, value := range files {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return []string{"prepare-work-node", "--job", filepath.Join(dir, "job.json"), "--policy", filepath.Join(dir, "policy.json"), "--rpc-config", filepath.Join(dir, "rpc.json"), "--payout-address", f.PayoutAddress, "--extra-nonce", "7"}, job, proposals
}

func TestPrepareWorkNode(t *testing.T) {
	args, job, proposals := workCommandFiles(t, false)
	var stdout, stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	if wire.TestPoW {
		if err == nil || stdout.Len() != 0 || proposals.Load() != 0 {
			t.Fatal("cheap profile prepared production work")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ProposalAccepted bool                   `json:"proposal_accepted"`
		WorkVerified     bool                   `json:"work_verified"`
		Admitted         bool                   `json:"admitted"`
		Candidate        prospectwork.Candidate `json:"candidate"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.ProposalAccepted || result.WorkVerified || result.Admitted || result.Candidate.JobID != job.ID || result.Candidate.Proof.JobID != job.ID || result.Candidate.PayoutAddress != args[8] || result.Candidate.CoinbaseValue != 1_000_000_000 || proposals.Load() != 1 {
		t.Fatalf("incorrect candidate labels or payout: %+v", result)
	}
	if !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatal("missing JSON newline")
	}
}

func TestPrepareWorkRejectsWithoutSuccessOutput(t *testing.T) {
	for _, name := range []string{"node rejection", "wrong policy", "expired job", "payout missing", "bad extra nonce", "unexpected context"} {
		t.Run(name, func(t *testing.T) {
			args, job, _ := workCommandFiles(t, name == "node rejection")
			switch name {
			case "wrong policy":
				p := prospectnode.Policy{Version: 1, Network: job.Context.Network, Genesis: job.Context.Genesis, DNAVersion: job.Context.DNAVersion, ProspectTarget: strings.Repeat("0", 63) + "1"}
				data, _ := json.Marshal(p)
				os.WriteFile(args[4], data, 0600)
			case "expired job":
				old, err := prospectjob.New(job.Context, job.PublicKey, job.Challenge, time.Now().Add(-10*time.Minute).UnixMilli())
				if err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(old)
				os.WriteFile(args[2], data, 0600)
			case "payout missing":
				args = args[:7]
			case "bad extra nonce":
				args[len(args)-1] = "18446744073709551616"
			case "unexpected context":
				args = append(args, "--context", "untrusted.json")
			}
			var stdout, stderr bytes.Buffer
			err := run(args, &stdout, &stderr)
			if err == nil || stdout.Len() != 0 || strings.Contains(err.Error(), "secret-password") || strings.Contains(err.Error(), "<html>") {
				t.Fatalf("unsafe failure: %v, stdout=%s", err, stdout.String())
			}
		})
	}
}

func TestPrepareWorkHelpAndLocalTimeBoundaries(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"prepare-work-node", "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "payout-address") || strings.Contains(stderr.String(), "private-key") {
		t.Fatal("incorrect command help")
	}
	job := prospectjob.Job{IssuedAtMS: 1000, ExpiresAtMS: 301000}
	for _, tc := range []struct {
		ms    int64
		valid bool
	}{{999, false}, {1000, true}, {300999, true}, {301000, false}} {
		if (checkJobTime(job, time.UnixMilli(tc.ms)) == nil) != tc.valid {
			t.Fatalf("incorrect local hint at %d", tc.ms)
		}
	}
}
