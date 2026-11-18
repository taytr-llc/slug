// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"os"
	"testing"
)

func TestBoutTransportNoRegenerationOrPublicRecordAudit(t *testing.T) {
	for _, name := range []string{"bout_codec.go", "bout_validation.go", "bout_protocol.go", "bout_lane.go", "bout_controller.go"} {
		raw, e := os.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		for _, s := range []string{"fightermanifest.Generate", "fightercondition.Project(", "fighterportrait.", "fighterbout.Simulate", "fighterbout.ValidateResult", "fighterbout.MarshalResult", "gymbout.ValidateRecord(", "gymbout.ValidateRunResult(", "gymbout.MarshalRunResult(", "gymbout.RecordDigest(", "gymbout.DeriveInput(", "gymcard.ValidateBoutSource(", "gymcard.BoutSourceDigest(", ".BlockHash(", "ed25519.Sign("} {
			if bytes.Contains(raw, []byte(s)) {
				t.Fatalf("%s unexpectedly reaches %s", name, s)
			}
		}
	}
}
