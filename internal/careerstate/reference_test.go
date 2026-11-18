// Copyright (c) 2026 The SLUG developers. ISC license.
package careerstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fightercareer"
)

const careerCorpusSHA = "89fbf2b1d16793b544d84922311017303832cdba89203f8a73fe4b79b3d66d42"

type referenceTotals struct {
	ProspectID string               `json:"prospect_id"`
	Found      bool                 `json:"found"`
	Totals     fightercareer.Totals `json:"totals"`
}

func TestFrozenCareerReference(t *testing.T) {
	b, e := os.ReadFile("testdata/reference/corpus.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != careerCorpusSHA {
		t.Fatal("independent corpus changed")
	}
	var corpus struct {
		Version int
		Policy  struct {
			PolicyID string `json:"policy_id"`
		}
		Activations []struct {
			ID        string
			Value     Activation
			Canonical string `json:"canonical_json"`
		}
		Events []struct {
			ID        string
			Value     Event
			Canonical string `json:"canonical_json"`
		}
		Finalities []struct {
			Canonical string `json:"canonical_json"`
			Digest    string
		}
		Refusals []struct {
			ID     string
			Events []Event
			Valid  bool `json:"expected_valid"`
		} `json:"index_refusals"`
		Transitions []struct {
			ID                 string
			Previous, Proposed []Event
			Valid              bool `json:"expected_valid"`
		} `json:"index_transitions"`
		Snapshots []struct {
			ID    string
			Input struct {
				Activation string
				AsOf       int64 `json:"as_of_ms"`
				Clock      int64 `json:"clock_ms"`
				Events     []string
				IDs        []string `json:"ids"`
				Mode       Mode
				Owner      string `json:"owner_state"`
			}
			Expected struct {
				Error, Digest string
				AsOf          int64 `json:"as_of_ms"`
				Mode          Mode
				Totals        []referenceTotals
				Unrequested   referenceTotals `json:"unrequested_lookup"`
			}
		} `json:"snapshot_cases"`
	}
	if e = json.Unmarshal(b, &corpus); e != nil || corpus.Version != 1 {
		t.Fatal(e)
	}
	if len(corpus.Activations) != 3 || len(corpus.Events) != 33 || len(corpus.Finalities) != 4 || len(corpus.Refusals) != 7 || len(corpus.Transitions) != 9 || len(corpus.Snapshots) != 52 {
		t.Fatal("finite corpus inventory changed")
	}
	if PolicyID() != corpus.Policy.PolicyID {
		t.Fatal("policy identity")
	}
	activations := map[string]Activation{}
	events := map[string]Event{}
	for _, v := range corpus.Activations {
		if ValidateActivation(v.Value) != nil || ActivationID(v.Value) != v.Value.ActivationID {
			t.Fatal("activation", v.ID)
		}
		raw, _ := json.Marshal(v.Value)
		if string(raw) != v.Canonical {
			t.Fatal("activation canonical", v.ID)
		}
		activations[v.ID] = v.Value
	}
	primary := activations["primary"]
	for _, v := range corpus.Events {
		if _, e := ValidateIndex(primary, []Event{v.Value}); e != nil {
			t.Fatal("event", v.ID, e)
		}
		raw, _ := json.Marshal(v.Value)
		if string(raw) != v.Canonical {
			t.Fatal("event canonical", v.ID)
		}
		events[v.ID] = v.Value
	}
	for i, v := range corpus.Finalities {
		if FinalityDigest([]byte(v.Canonical)) != v.Digest {
			t.Fatal("original finality digest", i)
		}
	}
	for _, v := range corpus.Refusals {
		t.Run("index/"+v.ID, func(t *testing.T) {
			_, e := ValidateIndex(primary, v.Events)
			if (e == nil) != v.Valid {
				t.Fatal("index acceptance", e)
			}
		})
	}
	for _, v := range corpus.Transitions {
		t.Run("transition/"+v.ID, func(t *testing.T) {
			_, e := ValidateTransition(primary, v.Previous, v.Proposed)
			if (e == nil) != v.Valid {
				t.Fatal("transition acceptance", e)
			}
		})
	}
	scope := Scope{StableStoreIDs: [2]string{strings.Repeat("1", 64), strings.Repeat("2", 64)}, CardStoreID: strings.Repeat("3", 64)}
	for _, v := range corpus.Snapshots {
		t.Run("snapshot/"+v.ID, func(t *testing.T) {
			a, ok := activations[v.Input.Activation]
			if !ok {
				t.Fatal("unknown activation")
			}
			state := State{Activation: a, Scope: scope, ClockMS: v.Input.Clock, Events: []Event{}}
			for _, name := range v.Input.Events {
				event, ok := events[name]
				if !ok {
					t.Fatal("unknown event", name)
				}
				state.Events = append(state.Events, event)
			}
			ownerErrors := map[string]error{"live": nil, "inactive": ErrInactive, "closed": ErrClosed, "poisoned": ErrPoisoned, "cancelled": ErrCancelled, "corrupt": ErrCorrupt}
			owner, ok := ownerErrors[v.Input.Owner]
			if !ok {
				t.Fatal("unknown owner state")
			}
			r, e := NewReader(a, scope, func(context.Context) (State, error) { return state, owner })
			if e != nil {
				t.Fatal(e)
			}
			got, e := r.Snapshot(context.Background(), v.Input.IDs, v.Input.AsOf, v.Input.Mode)
			if v.Expected.Error != "" {
				if e == nil || e.Error() != v.Expected.Error || got.Digest() != "" {
					t.Fatalf("refusal got=%v want=%s", e, v.Expected.Error)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if got.Digest() != v.Expected.Digest || got.Activation() != a || got.AsOfMS() != v.Expected.AsOf || got.Mode() != v.Expected.Mode || !reflect.DeepEqual(got.ProspectIDs(), v.Input.IDs) {
				t.Fatalf("bound prefix got=%s want=%s", got.Digest(), v.Expected.Digest)
			}
			for _, row := range append(v.Expected.Totals, v.Expected.Unrequested) {
				totals, ok := got.Totals(row.ProspectID)
				if ok != row.Found || totals != row.Totals {
					t.Fatalf("totals %s got=%+v/%v want=%+v/%v", row.ProspectID, totals, ok, row.Totals, row.Found)
				}
			}
		})
	}
}
