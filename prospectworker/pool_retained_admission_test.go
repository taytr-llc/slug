//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
)

type retainedAdmissionResponse struct {
	*pwAdmission
	failure *prospectservice.ClientError
}

func (a *retainedAdmissionResponse) SubmitManifest(_ context.Context, j prospectjob.ManifestJob, p prospect.Proof, auth gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	a.calls++
	if gymauth.VerifyAuthorization(j.Context, p, auth) != nil {
		return prospectservice.ManifestAdmissionResult{}, &prospectservice.ClientError{Code: "invalid_request"}
	}
	return prospectservice.ManifestAdmissionResult{}, a.failure
}

// Conditional delivery test using the inherited independent prospect-only
// proof and original durable outbox. These controlled authority responses test
// the shared delivery path, not a DAILY server's authentication or issuer clock;
// the selected service and actual real-expiry scenarios prove those separately.
func TestRetainedPoolAdmissionDefiniteExpiryAndUnknownReopen(t *testing.T) {
	for _, tc := range []struct {
		name, code, want string
		unknown          bool
	}{
		{"definite-expired", "expired", "expired", false},
		{"unknown-expired", "expired", "pending", true},
		{"capacity-after-expiry", "capacity", "pending", false},
		{"unavailable-after-expiry", "unavailable", "pending", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, work, proof := pwFixture(t, "coinbase-only", 0, true)
			fixture := pwAdmissionFixture(t)
			box, path := pwBox(t, c)
			entry := pdFound(t, c, work, proof, box)
			entry.Fighter.Authorization, entry.Fighter.AdmissionState = &fixture.Auth, "pending"
			if err := box.update(entry); err != nil {
				t.Fatal(err)
			}
			original := poolImmutable(entry)
			authority := &pwAuthority{c: c, w: work}
			admission := &retainedAdmissionResponse{pwAdmission: &pwAdmission{c: c}, failure: &prospectservice.ClientError{Code: tc.code, OutcomeUnknown: tc.unknown}}
			node := &pwNode{}
			worker, err := NewPool(c, authority, admission, node, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
				t.Fatal("retained original signature replaced")
				return gymauth.Authorization{}, nil
			}, box)
			if err != nil {
				t.Fatal(err)
			}
			worker.now = func() time.Time { return time.UnixMilli(work.Job.ExpiresAtMS + 100) }
			if _, err := worker.deliver(context.Background(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Fighter.AdmissionState != tc.want || entry.Fighter.LastError != tc.code || entry.Fighter.Receipt != nil || !reflect.DeepEqual(*entry.Fighter.Authorization, fixture.Auth) || !reflect.DeepEqual(poolImmutable(entry), original) {
				t.Fatal("definite/unknown authority changed original retained work")
			}
			if admission.calls != 1 || authority.issues != 0 || authority.lookups != 0 || authority.submits != 0 || len(node.calls) != 0 {
				t.Fatal("retained fighter reconciliation performed unrelated work")
			}
			if err := box.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenPoolOutbox(path, c)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := reopened.Close(); err != nil {
					t.Error(err)
				}
			}()
			rows, err := reopened.List(0, 1)
			if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], entry) {
				t.Fatal("original pending/expired state changed on physical reopen", err)
			}
		})
	}
}
