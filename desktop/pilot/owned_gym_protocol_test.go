// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

func TestOwnedGymWireRetainsMutationUncertainty(t *testing.T) {
	for _, e := range []error{gymstable.ErrOutcomeUnknown, gymentry.ErrOutcomeUnknown, gymevent.ErrOutcomeUnknown, recoveryvault.ErrOutcomeUnknown} {
		if got := OwnedGymError(errors.Join(e, context.Canceled, gymaccount.ErrUnauthorized)); got != "outcome_unknown" {
			t.Fatal("committed action uncertainty lost", got)
		}
	}
	if got := OwnedGymError(errors.Join(recoveryvault.ErrExportUncertain, context.Canceled)); got != "backup_export_uncertain" {
		t.Fatal("possible file publication hidden", got)
	}
}

// This literal follows the written wire field order, independent of the Go
// serializer. It is a framing expectation, not a cryptographic test vector.
const ownedSnapshotRequestLiteral = `{"version":1,"id":1,"command":"snapshot","view_id":"","fixture":"","capture_id":"","prospect_id":"","request_id":"","operation_id":"","review_id":"","recipient_gym":"","leg":"","saturday":"","after":0,"limit":0,"slot":null,"entry":null,"selection":null}`

func TestOwnedGymWireClosedRequest(t *testing.T) {
	r, e := ParseOwnedGymRequest([]byte(ownedSnapshotRequestLiteral))
	if e != nil || r.Command != "snapshot" || r.ID != 1 {
		t.Fatal("written request refused", e)
	}
	raw, e := MarshalOwnedGymRequest(OwnedGymRequest{Version: 1, ID: 1, Command: "snapshot"})
	if e != nil || string(raw) != ownedSnapshotRequestLiteral {
		t.Fatal("written order differs", e)
	}
	for name, raw := range map[string]string{
		"duplicate":      strings.Replace(ownedSnapshotRequestLiteral, `"id":1`, `"id":1,"id":1`, 1),
		"case":           strings.Replace(ownedSnapshotRequestLiteral, `"command"`, `"Command"`, 1),
		"reorder":        strings.Replace(ownedSnapshotRequestLiteral, `"version":1,"id":1`, `"id":1,"version":1`, 1),
		"null string":    strings.Replace(ownedSnapshotRequestLiteral, `"view_id":""`, `"view_id":null`, 1),
		"missing arm":    strings.Replace(ownedSnapshotRequestLiteral, `,"slot":null`, "", 1),
		"unknown":        strings.Replace(ownedSnapshotRequestLiteral, `"selection":null}`, `"selection":null,"raw_tx":"00"}`, 1),
		"trailing":       ownedSnapshotRequestLiteral + " ",
		"exponent":       strings.Replace(ownedSnapshotRequestLiteral, `"id":1`, `"id":1e0`, 1),
		"unsafe integer": strings.Replace(ownedSnapshotRequestLiteral, `"id":1`, `"id":9007199254740992`, 1),
		"negative zero":  strings.Replace(ownedSnapshotRequestLiteral, `"after":0`, `"after":-0`, 1),
		"inactive field": strings.Replace(ownedSnapshotRequestLiteral, `"leg":""`, `"leg":"commit"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := ParseOwnedGymRequest([]byte(raw)); e == nil {
				t.Fatal("accepted ambiguous request")
			}
		})
	}
}

func TestOwnedGymWireTransferRequiresExactCorrelation(t *testing.T) {
	f, e := gymaccount.FixtureIdentity("incrementing256")
	if e != nil {
		t.Fatal(e)
	}
	r := OwnedGymRequest{Version: 1, ID: 3, Command: "transfer_prepare", ViewID: strings.Repeat("1", 64), RequestID: strings.Repeat("2", 64), ProspectID: strings.Repeat("3", 64), RecipientGym: f.GymPublicKey}
	if e := ValidateOwnedGymRequest(r); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*OwnedGymRequest){
		"missing original view": func(x *OwnedGymRequest) { x.ViewID = "" },
		"missing known request": func(x *OwnedGymRequest) { x.RequestID = "" },
		"zero request":          func(x *OwnedGymRequest) { x.RequestID = strings.Repeat("0", 64) },
		"invalid curve point":   func(x *OwnedGymRequest) { x.RecipientGym = "02" + strings.Repeat("f", 64) },
		"uncompressed":          func(x *OwnedGymRequest) { x.RecipientGym = "04" + x.RecipientGym[2:] },
		"preselected review":    func(x *OwnedGymRequest) { x.ReviewID = strings.Repeat("4", 64) },
		"unused paging":         func(x *OwnedGymRequest) { x.Limit = 8 },
		"extra fixture":         func(x *OwnedGymRequest) { x.Fixture = "zero256" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := r
			change(&bad)
			if ValidateOwnedGymRequest(bad) == nil {
				t.Fatal("accepted changed authority/correlation")
			}
		})
	}
	for _, cmd := range []string{"transfer_operation", "transfer_abandon"} {
		v := OwnedGymRequest{Version: 1, ID: 4, Command: cmd, ViewID: r.ViewID, RequestID: r.RequestID}
		if ValidateOwnedGymRequest(v) != nil {
			t.Fatal(cmd)
		}
		v.ReviewID = strings.Repeat("4", 64)
		if ValidateOwnedGymRequest(v) == nil {
			t.Fatal("lookup accepted review arm")
		}
	}
}

func TestOwnedGymWireErrorsCannotCarrySuccess(t *testing.T) {
	req := OwnedGymRequest{Version: 1, ID: 1, Command: "snapshot"}
	r := OwnedGymResponse{Version: 1, ID: 1, Command: "snapshot", Result: json.RawMessage("null"), Error: "outcome_unknown"}
	raw, _ := json.Marshal(r)
	got, value, e := ParseOwnedGymResponse(raw, req, nil, nil)
	if e != nil || got.Error != "outcome_unknown" || value != nil {
		t.Fatal("lost acknowledgment collapsed", e)
	}
	for name, change := range map[string]func(*OwnedGymResponse){
		"result on failure":       func(x *OwnedGymResponse) { x.Result = json.RawMessage(`{}`) },
		"wrong command":           func(x *OwnedGymResponse) { x.Command = "transfer_confirm" },
		"wrong id":                func(x *OwnedGymResponse) { x.ID++ },
		"wrong view":              func(x *OwnedGymResponse) { x.ViewID = strings.Repeat("1", 64) },
		"success with error":      func(x *OwnedGymResponse) { x.OK = true },
		"unbounded backend error": func(x *OwnedGymResponse) { x.Error = "private backend details" },
		"self-authorized result":  func(x *OwnedGymResponse) { x.OK = true; x.Error = ""; x.Result = json.RawMessage(`{}`) },
	} {
		t.Run(name, func(t *testing.T) {
			v := r
			change(&v)
			raw, _ := json.Marshal(v)
			if _, _, e := ParseOwnedGymResponse(raw, req, nil, nil); e == nil {
				t.Fatal("accepted mixed/crossed response")
			}
		})
	}
}

// Bootstrap discloses only the original childless setup. A pending reset must
// be recoverable before opening wallet scopes, without admitting child actions.
func TestOwnedGymWireBootstrapResetBoundary(t *testing.T) {
	source := ownedSourceRuntimeTest(t).owned.config
	encode := func(req OwnedGymRequest, s OwnedGymSnapshot) []byte {
		t.Helper()
		raw, e := json.Marshal(s)
		if e != nil {
			t.Fatal(e)
		}
		out, e := json.Marshal(OwnedGymResponse{Version: 1, ID: req.ID, Command: req.Command, OK: true, Result: raw})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	plain := OwnedGymSnapshot{Version: 1, Mode: OwnedGymMode, State: "signed_out", Account: AccountSnapshot{State: "signed_out"}, Source: source}
	for _, state := range []string{"signed_out", "failed", "resetting"} {
		s := plain
		s.State = state
		s.Account.State = state
		if state != "resetting" {
			s.Account.Fixture = "zero256"
			s.Account.LastError = "hosted_reset_pending"
			s.PendingResetID = strings.Repeat("1", 64)
			s.Error = "hosted_reset_pending"
		}
		for _, command := range []string{"startup", "snapshot", "resume_reset"} {
			req := OwnedGymRequest{Version: 1, ID: 1, Command: command}
			if command == "resume_reset" {
				req.OperationID = strings.Repeat("1", 64)
			}
			_, value, e := ParseOwnedGymBootstrapResponse(encode(req, s), req, &source)
			if e != nil || value.(OwnedGymSnapshot).State != state {
				t.Fatal("childless reset state refused", state, command, e)
			}
		}
	}
	req := OwnedGymRequest{Version: 1, ID: 1, Command: "snapshot"}
	if _, _, e := ParseOwnedGymBootstrapResponse(encode(req, plain), req, &source); e != nil {
		t.Fatal("completed reset cannot load scopes", e)
	}
	for name, mutate := range map[string]func(*OwnedGymSnapshot){
		"source retarget":       func(s *OwnedGymSnapshot) { s.Source.Store.StoreID = strings.Repeat("2", 64) },
		"view injection":        func(s *OwnedGymSnapshot) { s.View = &OwnedGymView{} },
		"child injection":       func(s *OwnedGymSnapshot) { s.Child = &OwnedGymChildSnapshot{} },
		"signed in":             func(s *OwnedGymSnapshot) { s.State = "signed_in"; s.Account.State = "signed_in" },
		"failure without reset": func(s *OwnedGymSnapshot) { s.State = "failed"; s.Account.State = "failed" },
		"unbound reset":         func(s *OwnedGymSnapshot) { s.PendingResetID = strings.Repeat("1", 64) },
		"export authority":      func(s *OwnedGymSnapshot) { s.Account.CanExport = true },
	} {
		s := plain
		mutate(&s)
		if _, _, e := ParseOwnedGymBootstrapResponse(encode(req, s), req, &source); e == nil {
			t.Fatal("unsafe bootstrap accepted", name)
		}
	}
	for _, command := range []string{"login_fixture", "scope", "transfer_prepare", "start", "export_recovery"} {
		r := req
		r.Command = command
		if _, _, e := ParseOwnedGymBootstrapResponse(encode(r, plain), r, &source); e == nil {
			t.Fatal("bootstrap admitted child command", command)
		}
	}
}
