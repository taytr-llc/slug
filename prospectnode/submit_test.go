//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func submissionNode(t *testing.T, f winningFixture, mode string) (*Client, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	submissions, requests := new(atomic.Int64), new(atomic.Int64)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var req struct {
			ID     uint64            `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			t.Error("invalid submission RPC")
			return
		}
		var result any
		switch req.Method {
		case "getcurrentnet":
			result = f.Context.Network
			if mode == "wrong chain" || (mode == "switched after submit" && submissions.Load() > 0) {
				result = uint32(1)
			}
		case "getblockhash":
			if len(req.Params) != 1 || string(req.Params[0]) != "0" {
				t.Error("submission tried to infer canonical membership")
				return
			}
			result = f.Context.Genesis
		case "submitblock":
			submissions.Add(1)
			var block string
			if len(req.Params) != 1 || json.Unmarshal(req.Params[0], &block) != nil || block != f.BlockHex {
				t.Error("submission did not preserve exact block")
				return
			}
			switch mode {
			case "rejected":
				result = "rejected: secret-password <html>duplicate</html>"
			case "empty result":
				result = ""
			case "boolean result":
				result = true
			case "invalid envelope":
				w.Write([]byte(`<html>secret-password</html>`))
				return
			case "http error":
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("secret-password"))
				return
			case "lost response":
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				connection.Close()
				return
			case "duplicate result":
				w.Write([]byte(`{"id":3,"error":null,"result":null,"result":"rejected"}`))
				return
			}
		default:
			t.Errorf("submission unexpectedly called %s", req.Method)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "error": nil, "result": result})
	})
	return newTestClient(t, handler), submissions, requests
}

func TestSubmitBlockProcessedDoesNotClaimCanonical(t *testing.T) {
	f := independentWinner(t)
	client, submissions, requests := submissionNode(t, f, "")
	if err := client.SubmitBlock(context.Background(), f.Context, f.BlockHex); err != nil {
		t.Fatal(err)
	}
	if submissions.Load() != 1 || requests.Load() != 5 {
		t.Fatalf("submissions=%d requests=%d", submissions.Load(), requests.Load())
	}
}

func TestSubmitBlockDistinguishesRejectionAndUnknownOutcome(t *testing.T) {
	f := independentWinner(t)
	for _, mode := range []string{"rejected", "empty result", "boolean result", "invalid envelope", "http error", "lost response", "duplicate result", "switched after submit"} {
		t.Run(mode, func(t *testing.T) {
			client, submissions, _ := submissionNode(t, f, mode)
			err := client.SubmitBlock(context.Background(), f.Context, f.BlockHex)
			var failure *SubmissionError
			if !errors.As(err, &failure) {
				t.Fatalf("untyped error: %v", err)
			}
			unknown := mode != "rejected"
			code := "rejected"
			if unknown {
				code = "outcome_unknown"
			}
			if failure.Code != code || failure.OutcomeUnknown != unknown || strings.Contains(err.Error(), "secret-password") || strings.Contains(err.Error(), "<html>") {
				t.Fatalf("unsafe outcome: %+v %v", failure, err)
			}
			if submissions.Load() != 1 {
				t.Fatal("submission transport retried automatically")
			}
		})
	}
}

func TestSubmitBlockPreflightRejectsWithoutBroadcast(t *testing.T) {
	f := independentWinner(t)
	for _, mode := range []string{"wrong chain", "invalid block", "wrong context"} {
		t.Run(mode, func(t *testing.T) {
			client, submissions, requests := submissionNode(t, f, mode)
			block, initial := f.BlockHex, f.Context
			if mode == "invalid block" {
				block += "00"
			}
			if mode == "wrong context" {
				initial.ExpectedBits = 0x01010000
			}
			err := client.SubmitBlock(context.Background(), initial, block)
			var failure *SubmissionError
			if !errors.As(err, &failure) || failure.OutcomeUnknown || submissions.Load() != 0 {
				t.Fatalf("unsafe preflight result: %v", err)
			}
			if mode != "wrong chain" && requests.Load() != 0 {
				t.Fatal("invalid block reached RPC")
			}
		})
	}
}
