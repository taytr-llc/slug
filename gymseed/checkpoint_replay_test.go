// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
)

// The independent Python fixture uses a distinct valid Ed25519 nonce for the
// frozen checkpoint message. It is separate from the unchanged reference tree.
func TestSeedAttestRetainsAlternateCertificate(t *testing.T) {
	requireNormal(t)
	raw, err := os.ReadFile("testdata/alternate-checkpoint/alternate-checkpoint.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "1b613e398d712ae46a164bdef94ec0aba2dadd82cc91f96110d8591ddb91bddb" {
		t.Fatal("independent alternate signature fixture changed")
	}
	var fixture struct {
		Original  chaincfg.SignedCheckpoint `json:"original_checkpoint"`
		Alternate chaincfg.SignedCheckpoint `json:"alternate_checkpoint"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	c := readSeedReference(t)
	p := c.Pin.Value
	if !validCheckpoint(p, fixture.Original) || !validCheckpoint(p, fixture.Alternate) || equal(fixture.Original, fixture.Alternate) {
		t.Fatal("alternate signature is not a distinct valid same-message checkpoint")
	}
	var finalized Record
	for _, item := range c.Records {
		r, err := ParseRecord(p, []byte(item.Canonical))
		if err == nil && r.Finality != nil && len(r.Conflicts) == 0 && equal(r.Checkpoint, &fixture.Original) {
			finalized = r
			break
		}
	}
	if finalized.Finality == nil {
		t.Fatal("missing independently modeled finalized record")
	}
	for _, existingFinality := range []bool{false, true} {
		name := "certificate_only"
		if existingFinality {
			name = "historically_finalized"
		}
		t.Run(name, func(t *testing.T) {
			// Copy through the closed codec so neither branch aliases the fixture.
			b, err := MarshalRecord(p, finalized)
			if err != nil {
				t.Fatal(err)
			}
			r, err := ParseRecord(p, b)
			if err != nil {
				t.Fatal(err)
			}
			r.Checkpoint = &fixture.Alternate
			r.Finality.Checkpoint = fixture.Alternate
			prior := *r.Finality
			if !existingFinality {
				r.Finality = nil
				r.Observation.CurrentFinality = false
			}
			if _, err := ValidateRecord(p, r); err != nil {
				t.Fatal("alternate positive control", err)
			}
			tip := prior.ObservedTip
			n := r.Observation.ObservedAtMS + 1
			created := retainAttestedCheckpoint(&r, fixture.Original, tip, n)
			if created == existingFinality || !equal(r.Checkpoint, &fixture.Alternate) || !equal(r.Finality.Checkpoint, fixture.Alternate) {
				t.Fatal("attestation replaced retained certificate evidence")
			}
			if existingFinality && !equal(*r.Finality, prior) {
				t.Fatal("attestation rewrote historical finality")
			}
			r.Observation = Observation{ObservedAtMS: n, ChainObservedAtMS: n, Tip: &tip, NodeState: "online", TargetDescendants: tip.Height - r.Target.Height, CurrentFinality: true}
			if _, err := ValidateRecord(p, r); err != nil {
				t.Fatal("replayed attestation invalidated the record", err)
			}
		})
	}
}
