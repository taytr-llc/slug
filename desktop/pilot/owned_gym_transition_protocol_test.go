// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fightercarrier"
)

type ownedTransitionProtocolController struct {
	OwnedGymControllerAPI
	scope      OwnedGymScope
	state      string
	pending    string
	scopeCalls int
	transition string
	mutate     func(*ownedTransitionProtocolController)
}

func (c *ownedTransitionProtocolController) Snapshot() OwnedGymSnapshot {
	if c.scopeCalls >= 2 && c.mutate != nil {
		fn := c.mutate
		c.mutate = nil
		fn(c)
	}
	s := OwnedGymSnapshot{Version: 1, Mode: OwnedGymMode, State: c.state, Account: AccountSnapshot{State: c.state, Fixture: c.scope.Fixture}, Source: c.scope.Source, PendingResetID: c.pending}
	if c.pending != "" {
		s.Account.LastError = "hosted_reset_pending"
		s.Error = "hosted_reset_pending"
	}
	if c.state == "failed" && c.pending == "" {
		s.Account.LastError = "account_unavailable"
		s.Error = "owned_unavailable"
	}
	return s
}
func (c *ownedTransitionProtocolController) Scope(_ context.Context, fixture string) (OwnedGymScope, error) {
	if fixture != c.scope.Fixture {
		return OwnedGymScope{}, ErrPoolRuntime
	}
	c.scopeCalls++
	if c.transition != "" && c.scopeCalls > 2 {
		return OwnedGymScope{}, ErrPoolRuntime
	}
	value := c.scope
	if c.scopeCalls == 2 && c.transition != "" {
		c.state = c.transition
	}
	return value, nil
}
func (c *ownedTransitionProtocolController) ResumePendingReset(_ context.Context, id string) error {
	if id != c.pending || id == "" {
		return ErrPoolRuntime
	}
	c.pending = ""
	c.state = "resetting"
	return nil
}
func (*ownedTransitionProtocolController) Close() error { return nil }

func runOwnedTransitionProtocol(t *testing.T, c OwnedGymControllerAPI, requests []OwnedGymRequest) []OwnedGymResponse {
	t.Helper()
	var in bytes.Buffer
	for _, r := range requests {
		b, e := MarshalOwnedGymRequest(r)
		if e != nil {
			t.Fatal(e)
		}
		in.Write(b)
		in.WriteByte('\n')
	}
	var out ownedScopeProtocolOutput
	if e := ServeOwnedGym(context.Background(), io.NopCloser(&in), &out, c); e != nil {
		t.Fatal(e)
	}
	lines := bytes.Split(bytes.TrimSuffix(out.Bytes(), []byte{'\n'}), []byte{'\n'})
	if len(lines) != len(requests) {
		t.Fatal("reply count", len(lines))
	}
	results := make([]OwnedGymResponse, len(lines))
	for i, line := range lines {
		if e := json.Unmarshal(line, &results[i]); e != nil {
			t.Fatal(e)
		}
		if results[i].ID != requests[i].ID || results[i].Command != requests[i].Command {
			t.Fatal("correlation changed")
		}
	}
	return results
}
func ownedTransitionRequests() []OwnedGymRequest {
	return []OwnedGymRequest{{Version: 1, ID: 1, Command: "startup"}, {Version: 1, ID: 2, Command: "scope", Fixture: "zero256"}, {Version: 1, ID: 3, Command: "snapshot"}}
}

func TestOwnedGymWireNormalTransitionsUseOriginalSealedScope(t *testing.T) {
	for _, state := range []string{"signing_in", "signing_out", "resetting", "failed", "expired"} {
		t.Run(state, func(t *testing.T) {
			c := &ownedTransitionProtocolController{scope: ownedScopeProtocolFixtures(t)["zero256"], state: "signed_out", transition: state}
			// Scope is unavailable while the async account transition owns its gate.
			// The scope was disclosed before that state; ordinary snapshots use it.
			rows := runOwnedTransitionProtocol(t, c, ownedTransitionRequests())
			for _, row := range rows {
				if !row.OK || row.Error != "" {
					t.Fatal("normal transition became uncertain", state, row.Error)
				}
			}
			var s OwnedGymSnapshot
			if e := json.Unmarshal(rows[2].Result, &s); e != nil || s.State != state {
				t.Fatal("transition response changed", e)
			}
			if c.scopeCalls != 2 {
				t.Fatal("transition reacquired controller Scope", c.scopeCalls)
			}
		})
	}
}
func TestOwnedGymWireSealedScopeOwnsNestedValues(t *testing.T) {
	c := &ownedTransitionProtocolController{scope: ownedScopeProtocolFixtures(t)["zero256"], state: "signed_out"}
	c.mutate = func(c *ownedTransitionProtocolController) {
		c.scope.EventPin.Participants[0].StableStoreID = strings.Repeat("f", 64)
	}
	rows := runOwnedTransitionProtocol(t, c, ownedTransitionRequests())
	for _, r := range rows {
		if !r.OK {
			t.Fatal("controller alias mutated sealed scope", r.Error)
		}
	}
}
func TestOwnedGymWireRejectsChangedScopeRedisclosure(t *testing.T) {
	c := &ownedTransitionProtocolController{scope: ownedScopeProtocolFixtures(t)["zero256"], state: "signed_out"}
	c.mutate = func(c *ownedTransitionProtocolController) {
		c.scope.Carrier.JournalID = strings.Repeat("ab", 32)
		var e error
		c.scope.CarrierBindingID, e = fightercarrier.BindingID(c.scope.Carrier)
		if e != nil || ValidateOwnedGymScope(c.scope) != nil {
			t.Fatal("replacement must be structurally valid", e)
		}
	}
	req := ownedTransitionRequests()
	req = append(req, OwnedGymRequest{Version: 1, ID: 4, Command: "scope", Fixture: "zero256"})
	rows := runOwnedTransitionProtocol(t, c, req)
	for _, r := range rows[:3] {
		if !r.OK {
			t.Fatal("original context lost", r.Error)
		}
	}
	if rows[3].OK || rows[3].Error != "outcome_unknown" || string(rows[3].Result) != "null" {
		t.Fatal("replacement disclosure adopted")
	}
}
func TestOwnedGymWirePendingResetRemainsBootstrapOnly(t *testing.T) {
	id := strings.Repeat("cd", 32)
	c := &ownedTransitionProtocolController{scope: ownedScopeProtocolFixtures(t)["zero256"], state: "signed_out", pending: id}
	req := []OwnedGymRequest{{Version: 1, ID: 1, Command: "startup"}, {Version: 1, ID: 2, Command: "snapshot"}, {Version: 1, ID: 3, Command: "resume_reset", OperationID: id}, {Version: 1, ID: 4, Command: "snapshot"}}
	rows := runOwnedTransitionProtocol(t, c, req)
	for i, row := range rows {
		raw, e := json.Marshal(row)
		if e != nil {
			t.Fatal(e)
		}
		parsed, value, e := ParseOwnedGymBootstrapResponse(raw, req[i], &c.scope.Source)
		if e != nil || !parsed.OK || value == nil {
			t.Fatal("pending reset bootstrap lost", i, e, row.Error)
		}
	}
	if c.scopeCalls != 0 {
		t.Fatal("bootstrap opened wallet scope")
	}
	// Merely observing signing_in without any prior scope still cannot promote
	// bootstrap into a normal view; it remains refused by the same parser.
	c = &ownedTransitionProtocolController{scope: ownedScopeProtocolFixtures(t)["zero256"], state: "signing_in"}
	rows = runOwnedTransitionProtocol(t, c, []OwnedGymRequest{{Version: 1, ID: 1, Command: "startup"}})
	if rows[0].OK || rows[0].Error != "outcome_unknown" {
		t.Fatal("bootstrap widened to unsealed transition")
	}
}
