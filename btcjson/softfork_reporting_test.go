package btcjson

import (
	"encoding/json"
	"testing"
)

func TestChainInfoSoftForkReporting(t *testing.T) {
	buried := &SoftForkDescription{ID: "bip34", Version: 2}
	buried.Reject.Status = true
	legacy := &SoftForks{
		SoftForks: []*SoftForkDescription{buried},
		Bip9SoftForks: map[string]*Bip9SoftForkDescription{
			"segwit": {Status: "active", AlwaysActiveHeight: 62},
			"unset":  {Status: "defined"},
		},
	}
	unified := &UnifiedSoftForks{SoftForks: map[string]*UnifiedSoftFork{
		"segwit": {Type: "buried", Height: 62, Active: true},
	}}
	for _, test := range []struct {
		name string
		old  *SoftForks
		new  *UnifiedSoftForks
		want string
	}{
		{"legacy", legacy, nil, `[{"id":"bip34","version":2,"reject":{"status":true}}]`},
		{"unified", nil, unified, `{"segwit":{"type":"buried","bip9":null,"height":62,"active":true}}`},
		{"absent", nil, nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := GetBlockChainInfoResult{Chain: "mainnet", Blocks: 62,
				SoftForks: test.old, UnifiedSoftForks: test.new}
			encoded, err := json.Marshal(info)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields["softforks"]) != test.want || string(fields["blocks"]) != "62" || string(fields["chain"]) != `"mainnet"` {
				t.Fatalf("missing or ambiguous chain status: %s", encoded)
			}
			if test.old != nil {
				var deployments map[string]map[string]json.RawMessage
				if err := json.Unmarshal(fields["bip9_softforks"], &deployments); err != nil {
					t.Fatal(err)
				}
				if string(deployments["segwit"]["always_active_height"]) != "62" ||
					string(deployments["segwit"]["min_activation_height"]) != "0" ||
					len(deployments["unset"]["always_active_height"]) != 0 {
					t.Fatalf("fixed override was confused with voting minimum: %s", encoded)
				}
			}
		})
	}
	if _, err := json.Marshal(GetBlockChainInfoResult{SoftForks: legacy, UnifiedSoftForks: unified}); err == nil {
		t.Fatal("ambiguous soft-fork representations accepted")
	}
}
