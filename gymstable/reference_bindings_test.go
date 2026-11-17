// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestIndependentFullSourceBindings(t *testing.T) {
	if wire.TestPoW {
		t.Skip("independently verified normal-profile receipts")
	}
	raw, err := os.ReadFile("testdata/reference/stable_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "29f6a1302c2e04ade612fe2775c085c65b8bf33587c010060b727833182d2057" {
		t.Fatal("frozen input changed")
	}
	var f struct {
		Pins []struct {
			Name string
			Pin  Pin
		}
		Cases []struct {
			Fixture    string
			Record     registry.ManifestRecord
			Assignment Assignment
		}
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	pins := map[string]Pin{}
	for _, p := range f.Pins {
		pins[p.Name] = p.Pin
	}
	for _, tc := range f.Cases {
		digest, err := sourceRecordDigest(tc.Record)
		if err != nil || digest != tc.Assignment.SourceRecordDigest {
			t.Fatal("independent full receipt digest differs", err)
		}
		assignment, err := assignmentForRecord(tc.Record, pins[tc.Fixture])
		if err != nil || assignment != tc.Assignment {
			t.Fatal("independent assignment binding differs", err)
		}
		// Acceptance time can be locally self-consistent after a rewrite; binding
		// the entire original receipt must still distinguish that changed evidence.
		changed := tc.Record
		changed.Birth.AcceptedAtMS++
		other, err := sourceRecordDigest(changed)
		if err != nil || other == digest {
			t.Fatal("source digest omitted immutable acceptance evidence", err)
		}
	}
}
