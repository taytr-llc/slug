package rpcclient

import (
	"encoding/json"
	"testing"
)

func TestChainInfoReceivesActualSLUGFormatWithoutVersionProbe(t *testing.T) {
	// This is the slugd handler's public response shape, including its restored
	// legacy buried-rule array. Version capability guesses previously tried
	// to decode this array as a unified map and rejected the entire response.
	for _, buried := range []string{`"softforks":[{"id":"bip34","version":2,"reject":{"status":false}}],`, ""} {
		response := make(chan *Response, 1)
		response <- &Response{result: []byte(`{"chain":"mainnet","blocks":61,"bestblockhash":"fixture",` + buried +
			`"bip9_softforks":{"segwit":{"status":"active","always_active_height":62,"min_activation_height":0}}}`)}
		// A nil client is intentional: receiving a complete chain-info response
		// must not send an unrelated RPC to infer its already visible format.
		info, err := (FutureGetBlockChainInfoResult{Response: response}).Receive()
		if err != nil {
			t.Fatal(err)
		}
		if info.Chain != "mainnet" || info.Blocks != 61 || info.BestBlockHash != "fixture" ||
			info.SoftForks == nil || info.UnifiedSoftForks != nil {
			t.Fatalf("lost chain or legacy status: %+v", info)
		}
		segwit := info.Bip9SoftForks["segwit"]
		if segwit == nil || segwit.Status != "active" || segwit.AlwaysActiveHeight != 62 || segwit.MinActivationHeight != 0 {
			t.Fatal("lost next-block override status")
		}
		if buried != "" && (len(info.SoftForks.SoftForks) != 1 || info.SoftForks.SoftForks[0].Reject.Status) {
			t.Fatal("lost current-tip buried-rule status")
		}
	}
}

func TestChainInfoUnifiedAndEmptyFormats(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		fork bool
	}{
		{"unified", `{"blocks":62,"softforks":{"segwit":{"type":"buried","active":true,"height":62}}}`, true},
		{"absent", `{"blocks":62}`, false},
		{"null", `{"blocks":62,"softforks":null}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := make(chan *Response, 1)
			response <- &Response{result: []byte(test.data)}
			info, err := (FutureGetBlockChainInfoResult{Response: response}).Receive()
			if err != nil {
				t.Fatal(err)
			}
			if info.Blocks != 62 || info.SoftForks != nil || (info.UnifiedSoftForks != nil) != test.fork {
				t.Fatal("incorrect response shape")
			}
			if test.fork {
				fork := info.UnifiedSoftForks.SoftForks["segwit"]
				if fork == nil || !fork.Active || fork.Height != 62 {
					t.Fatal("lost unified activation status")
				}
			}
			if _, err := json.Marshal(info); err != nil {
				t.Fatal("decoded response retained ambiguous representations", err)
			}
		})
	}
}
