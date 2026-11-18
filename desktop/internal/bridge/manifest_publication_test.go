// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"encoding/json"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func TestManifestBlankPublicationBoundary(t *testing.T) {
	for _, state := range []string{"signed_out", "signing_in", "resetting", "signing_out", "failed", "expired", "signed_in"} {
		t.Run(state, func(t *testing.T) {
			s := pilot.BlankManifestSnapshot()
			s.Account = pilot.AccountSnapshot{State: state, Fixture: "incrementing256"}
			raw, err := json.Marshal(pilot.ManifestResponse{ID: 1, OK: true, Snapshot: &s})
			if err != nil {
				t.Fatal(err)
			}
			_, err = decodeManifest(raw)
			if state == "signed_in" {
				if err != ErrUnavailable {
					t.Fatal("blank signed-in snapshot was accepted", err)
				}
			} else if err != nil {
				t.Fatal("valid blank lifecycle snapshot refused", err)
			}
		})
	}
}
