// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gymauth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/prospect"
)

func TestJobAuthorizationVersionRouting(t *testing.T) {
	context, legacy, oldAuth := codecFixture(t)
	proof := legacy
	proof.Version = prospect.Version2
	proof.JobID = strings.Repeat("0", 64)
	// This tests action routing and signature binding only. Work/commitment
	// validity uses separate independent v2 proof fixtures in prospectjob.
	private, _ := btcec.PrivKeyFromBytes([]byte{1}) // Published fixture key only.
	auth, err := SignAdmission(context, proof, private)
	if err != nil || auth.Version != Version2 {
		t.Fatalf("v2 signing failed: %v", err)
	}
	if err := VerifyAuthorization(context, proof, auth); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(auth)
	parsed, err := ParseAuthorization(encoded)
	if err != nil || parsed != auth {
		t.Fatalf("v2 authorization JSON failed: %v", err)
	}
	if err := VerifyAuthorization(context, proof, oldAuth); err == nil {
		t.Fatal("v1 authorization accepted for v2 proof")
	}
	downgraded := auth
	downgraded.Version = Version1
	if err := VerifyAuthorization(context, legacy, downgraded); err == nil {
		t.Fatal("v2 signature replayed in v1 domain")
	}
	upgraded := oldAuth
	upgraded.Version = Version2
	if err := VerifyAuthorization(context, proof, upgraded); err == nil {
		t.Fatal("v1 signature replayed in v2 domain")
	}
	proof.JobID = strings.Repeat("1", 64)
	if err := VerifyAuthorization(context, proof, auth); err == nil {
		t.Fatal("job substitution kept authorization valid")
	}
	for _, bad := range []prospect.Proof{
		{Version: prospect.Version1, JobID: strings.Repeat("0", 64)},
		{Version: prospect.Version2},
		{Version: prospect.Version2, JobID: "0"},
	} {
		candidate := legacy
		candidate.Version = bad.Version
		candidate.JobID = bad.JobID
		if _, err := AdmissionDigest(context, candidate, oldAuth.PublicKey); err == nil {
			t.Fatal("accepted invalid job/version shape")
		}
	}
}
