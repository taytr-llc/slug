// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

// Full scopes use the actual two public fixture identities and all production
// pin derivations. This tests disclosure framing, not possession of authority.
func ownedScopeProtocolFixtures(t *testing.T) map[string]OwnedGymScope {
	t.Helper()
	r := ownedSourceRuntimeTest(t)
	source := r.owned.config
	cfg, e := OwnedLeagueConfigFor(source, r.source, strings.Repeat("11", 32), strings.Repeat("12", 32), strings.Repeat("13", 32))
	if e != nil {
		t.Fatal(e)
	}
	oid, e := fighterownership.ID(source.Ownership)
	if e != nil {
		t.Fatal(e)
	}
	result := map[string]OwnedGymScope{}
	for i, p := range cfg.Participants {
		f := p.StablePin.Stable.Identity
		identity := fightercarrier.Identity{JournalID: strings.Repeat([]string{"21", "22"}[i], 32), Pin: fightercarrier.Pin{Version: 1, OwnerMode: "hosted", ManagerSchema: 14, Network: f.Network, Genesis: f.Genesis, WalletProfile: f.Profile, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, WalletOwnerID: strings.Repeat([]string{"31", "32"}[i], 32), OwnershipPinID: oid, AccountAuthorityID: p.StablePin.Stable.AccountAuthorityID, AccountID: p.StablePin.Stable.AccountID, AllocationBindingID: strings.Repeat([]string{"41", "42"}[i], 32)}}
		bid, e := fightercarrier.BindingID(identity)
		if e != nil {
			t.Fatal(e)
		}
		s := OwnedGymScope{Version: 1, Fixture: p.Fixture, Source: source, PoolSource: cfg.PoolSource, SourceBindingID: cfg.SourceBindingID, StablePin: p.StablePin, StableStoreID: p.StableStoreID, EntryPin: p.EntryPin, EntryStoreID: p.EntryStoreID, EventPin: cfg.EventPin, EventStoreID: cfg.EventStoreID, Carrier: identity, CarrierBindingID: bid}
		if e := ValidateOwnedGymScope(s); e != nil {
			t.Fatal("complete fixture scope invalid", e)
		}
		result[p.Fixture] = s
	}
	return result
}

func TestOwnedGymWireCompleteScopeRoundTrip(t *testing.T) {
	for fixture, s := range ownedScopeProtocolFixtures(t) {
		req := OwnedGymRequest{Version: 1, ID: 2, Command: "scope", Fixture: fixture}
		raw, e := json.Marshal(s)
		if e != nil {
			t.Fatal(e)
		}
		envelope := OwnedGymResponse{Version: 1, ID: 2, Command: "scope", OK: true, Result: raw}
		encoded, e := json.Marshal(envelope)
		if e != nil {
			t.Fatal(e)
		}
		got, value, e := ParseOwnedGymResponse(encoded, req, &s, nil)
		if e != nil || !got.OK || !ownedcodec.Equal(value, s) {
			t.Fatal("full scope did not roundtrip", fixture, e)
		}
		for label, bad := range map[string][]byte{
			"lowercase journal": bytes.Replace(encoded, []byte(`"JournalID":`), []byte(`"journal_id":`), 1),
			"lowercase pin":     bytes.Replace(encoded, []byte(`"Pin":`), []byte(`"pin":`), 1),
			"duplicate journal": bytes.Replace(encoded, []byte(`"JournalID":`), []byte(`"JournalID":"`+s.Carrier.JournalID+`","JournalID":`), 1),
		} {
			if _, _, e := ParseOwnedGymResponse(bad, req, &s, nil); e == nil {
				t.Fatal("noncanonical identity accepted", label)
			}
		}
	}
}

type ownedScopeProtocolController struct {
	OwnedGymControllerAPI
	scopes map[string]OwnedGymScope
}

func (c *ownedScopeProtocolController) Snapshot() OwnedGymSnapshot {
	return OwnedGymSnapshot{Version: 1, Mode: OwnedGymMode, State: "signed_out", Account: AccountSnapshot{State: "signed_out"}, Source: c.scopes["zero256"].Source}
}
func (c *ownedScopeProtocolController) Scope(_ context.Context, fixture string) (OwnedGymScope, error) {
	return c.scopes[fixture], nil
}
func (c *ownedScopeProtocolController) Close() error { return nil }

type ownedScopeProtocolOutput struct{ bytes.Buffer }

func (*ownedScopeProtocolOutput) Close() error { return nil }

func TestOwnedGymWireServeStartupAndBothCompleteScopes(t *testing.T) {
	c := &ownedScopeProtocolController{scopes: ownedScopeProtocolFixtures(t)}
	requests := []OwnedGymRequest{{Version: 1, ID: 1, Command: "startup"}, {Version: 1, ID: 2, Command: "scope", Fixture: "zero256"}, {Version: 1, ID: 3, Command: "scope", Fixture: "incrementing256"}}
	var input bytes.Buffer
	for _, r := range requests {
		raw, e := MarshalOwnedGymRequest(r)
		if e != nil {
			t.Fatal(e)
		}
		input.Write(raw)
		input.WriteByte('\n')
	}
	var output ownedScopeProtocolOutput
	if e := ServeOwnedGym(context.Background(), io.NopCloser(&input), &output, c); e != nil {
		t.Fatal(e)
	}
	lines := bytes.Split(bytes.TrimSuffix(output.Bytes(), []byte{'\n'}), []byte{'\n'})
	if len(lines) != len(requests) {
		t.Fatal("missing exact replies", len(lines))
	}
	for i, req := range requests {
		var scope *OwnedGymScope
		if i > 0 {
			v := c.scopes[req.Fixture]
			scope = &v
		}
		envelope, value, e := ParseOwnedGymResponse(lines[i], req, scope, nil)
		if e != nil || !envelope.OK || envelope.Error != "" || value == nil {
			t.Fatal("successful disclosure became uncertain", req.Command, req.Fixture, envelope.Error, e)
		}
	}
}

// The exact command result family must remain compatible with the common
// closed prewalk. Check every nested exported field, not just currently
// nonempty arms in one fixture. The inherited checkpoint is the sole custom
// marshaler explicitly handled by ownedcodec.
func TestOwnedGymWireResultShapesHaveExplicitTags(t *testing.T) {
	roots := []any{OwnedGymScope{}, OwnedGymSnapshot{}, OwnedStableCapture{},
		gymstable.OwnedPage{}, gymstable.OwnedIntent{}, gymstable.OwnedOperation{}, gymstable.OwnedMutationResult{},
		gymstable.OwnedCareerHistoryCapture{}, gymstable.OwnedCareerHistoryPage{},
		gymentry.OwnedEventStatus{}, gymentry.OwnedIntent{}, gymentry.OwnedOperation{}, gymentry.OwnedMutationResult{},
		gymevent.OwnedEventStatus{}, gymevent.OwnedSnapshotReceipt{},
		OwnedTransferReview{}, OwnedTransferDeliveryReview{}, OwnedTransferFinality{}, fightercarrier.Operation{}}
	seen := map[reflect.Type]bool{}
	var visit func(reflect.Type, string)
	visit = func(typ reflect.Type, path string) {
		if typ == reflect.TypeOf(json.RawMessage{}) || typ == reflect.TypeOf(chaincfg.SignedCheckpoint{}) {
			return
		}
		if seen[typ] {
			return
		}
		seen[typ] = true
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			visit(typ.Elem(), path+"[]")
		case reflect.Struct:
			for n := 0; n < typ.NumField(); n++ {
				f := typ.Field(n)
				if f.PkgPath != "" {
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag == "-" {
					continue
				}
				if tag == "" {
					t.Errorf("untagged response field %s.%s (%s)", path, f.Name, typ)
				}
				visit(f.Type, path+"."+f.Name)
			}
		case reflect.String, reflect.Bool, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		default:
			t.Errorf("unsupported closed response shape %s (%s)", path, typ)
		}
	}
	for _, v := range roots {
		typ := reflect.TypeOf(v)
		visit(typ, typ.String())
	}
}
