// Copyright (c) 2026 The SLUG developers. ISC license.
package btcjson_test

import (
	"encoding/json"
	"testing"

	"github.com/taytr-llc/slug/btcjson"
)

func TestChainObservationWireGrammar(t *testing.T) {
	raw, err := btcjson.MarshalCmd(btcjson.RpcVersion1, 1, btcjson.NewGetChainObservationCmd())
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"jsonrpc":"1.0","method":"getchainobservation","params":[],"id":1}` {
		t.Fatal("command grammar", string(raw))
	}
	if _, err = btcjson.NewCmd("getchainobservation", 1); err == nil {
		t.Fatal("unexpected argument accepted")
	}
	value := btcjson.GetChainObservationResult{
		Version: 1, InstanceID: "0000000000000000000000000000000000000000000000000000000000000001",
		Revision: "18446744073709551615", Hash: "0000000000000000000000000000000000000000000000000000000000000002", Height: 3,
	}
	raw, err = json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"instance_id":"0000000000000000000000000000000000000000000000000000000000000001","revision":"18446744073709551615","hash":"0000000000000000000000000000000000000000000000000000000000000002","height":3}`
	if string(raw) != want {
		t.Fatal("ordered result or exact decimal text changed")
	}
}
