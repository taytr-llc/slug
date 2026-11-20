// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
)

func TestOperatorCapacityIndependentEdges(t *testing.T) {
	raw, err := os.ReadFile("testdata/capacity/edges-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "afa13b15904568a370678e9d6ef4ef9ba92940202aefd3b07330e3820edcfe42" {
		t.Fatal("independent edge input changed")
	}
	var corpus struct {
		Cases []struct {
			Name     string
			Inputs   capacityInputs
			Expected CapacityReservation
		}
		Refusals []map[string]uint64 `json:"refused_inputs"`
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := capacityEnvelope(c.Inputs)
			if err != nil || got != c.Expected {
				t.Fatalf("bound mismatch: got%+v want%+v %v", got, c.Expected, err)
			}
		})
	}
	for _, mutation := range corpus.Refusals {
		base := map[string]uint64{"source_max": 4096, "rounds": 256, "round_bytes": 256 << 20, "credit_rounds": 256, "batches": 366, "gyms": 64}
		for key, value := range mutation {
			base[key] = value
		}
		raw, _ := json.Marshal(base)
		var input capacityInputs
		if json.Unmarshal(raw, &input) != nil {
			t.Fatal("invalid independent refusal")
		}
		if got, err := capacityEnvelope(input); err == nil || got != (CapacityReservation{}) {
			t.Fatal("invalid limit accepted", mutation)
		}
	}
	if _, err = CapacityEnvelope(Setup{}); err == nil {
		t.Fatal("unvalidated setup granted reservation")
	}
}

func TestOperatorCapacityMixedErrorsRemainFatal(t *testing.T) {
	for _, err := range []error{poolshare.ErrCapacity, poolround.ErrCapacity, fighterownership.ErrCapacity, registry.ErrPublicCapacity, errors.Join(poolround.ErrCapacity, registry.ErrPublicCapacity)} {
		if !capacityOnly(err) {
			t.Fatal("known capacity not recognized", err)
		}
	}
	for _, err := range []error{nil, registry.ErrPublicOwnership, context.Canceled, errors.Join(poolround.ErrCapacity, ErrCleanup), errors.Join(registry.ErrPublicCapacity, registry.ErrCorrupt), errors.Join(fighterownership.ErrCapacity, context.DeadlineExceeded)} {
		if capacityOnly(err) {
			t.Fatal("mixed/integrity error became drain", err)
		}
	}
	if !errors.Is(registry.ErrPublicCapacity, registry.ErrPublicOwnership) {
		t.Fatal("legacy error compatibility lost")
	}
	if !retainedRoundRetry(poolround.ErrBusy) || retainedRoundRetry(errors.Join(poolround.ErrBusy, ErrCleanup)) || retainedRoundRetry(errors.Join(poolround.ErrBusy, poolround.ErrCapacity)) {
		t.Fatal("mixed retained refresh failure became retry")
	}
	// Status is scalar copied data; no scheduler/ledger authority is embedded.
	status := Status{Capacity: CapacityStatus{Reservation: CapacityReservation{Submissions: 183}, Mode: "draining", Reason: "pool_capacity", Submissions: 183}}
	copy, err := cloneStatus(status)
	if err != nil || !reflect.DeepEqual(copy, status) {
		t.Fatal(err)
	}
	copy.Capacity.Submissions = 0
	if status.Capacity.Submissions != 183 {
		t.Fatal("status aliased retained observation")
	}
}
