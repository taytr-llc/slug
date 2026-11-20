// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"reflect"
	"strings"
	"testing"
)

func TestRemotePoolWireAdapterPreservesOriginalDTO(t *testing.T) {
	s := RemotePoolControllerSnapshot{Version: 1, ConfigID: strings.Repeat("a", 64), State: "signed_out", Account: pilot.AccountSnapshot{State: "signed_out", Fixture: "zero256"}}
	before, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	w := remotePoolWireSnapshot(s)
	if w.Version != s.Version || w.ConfigID != s.ConfigID || w.State != s.State || w.Account != s.Account || w.View != s.View || w.Remote != s.Remote || w.Error != s.Error {
		t.Fatal("mechanical field conversion")
	}
	after, e := json.Marshal(s)
	if e != nil || string(before) != string(after) {
		t.Fatal("old DTO changed")
	}
	if strings.Contains(string(before), `"view"`) || strings.Contains(string(before), `"remote"`) || strings.Contains(string(before), `"error"`) {
		t.Fatal("old omitempty behavior changed")
	}
	wire, e := json.Marshal(w)
	if e != nil || !strings.Contains(string(wire), `"view":null,"remote":null,"error":""`) {
		t.Fatal("new explicit wire fields", e)
	}
	s.View = &pilot.RemotePoolView{Version: 1, ViewID: strings.Repeat("b", 64)}
	s.Remote = &pilot.RemotePoolSnapshot{Version: 1, View: *s.View}
	w = remotePoolWireSnapshot(s)
	if !reflect.DeepEqual(w.View, s.View) || !reflect.DeepEqual(w.Remote, s.Remote) {
		t.Fatal("nested fields changed")
	}
	var absent *RemotePoolController
	if _, e = absent.WireSnapshot(context.Background()); e == nil {
		t.Fatal("nil original owner accepted")
	}
}
