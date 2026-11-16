// Package accountpilot owns an explicit private two-public-fixture login demo.
// It has no production identity-provider, real signup, or custody fallback.
package accountpilot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

const fixtureIssuer = "slug:public-fixture-provider:v1"

type fixtureProvider struct{}

// The assertions are deliberately public test credentials, selected inside the
// private helper. The webview selects an allowlisted demo label, never a provider
// issuer, subject, account ID, verified flag, timestamp, or recovery event.
func (fixtureProvider) Verify(ctx context.Context, assertion string) (gymaccount.Principal, error) {
	if ctx.Err() != nil {
		return gymaccount.Principal{}, ctx.Err()
	}
	var subject string
	switch assertion {
	case "SLUG-PUBLIC-FIXTURE-LOGIN-A":
		subject = "zero256"
	case "SLUG-PUBLIC-FIXTURE-LOGIN-B":
		subject = "incrementing256"
	default:
		return gymaccount.Principal{}, errors.New("fixture authentication unavailable")
	}
	return gymaccount.Principal{Issuer: fixtureIssuer, Subject: subject, AuthenticatedAtMS: time.Now().UnixMilli()}, nil
}

func verifyFixture(ctx context.Context, fixture string, reset bool) (gymaccount.Principal, error) {
	var assertion string
	switch fixture {
	case "zero256":
		assertion = "SLUG-PUBLIC-FIXTURE-LOGIN-A"
	case "incrementing256":
		assertion = "SLUG-PUBLIC-FIXTURE-LOGIN-B"
	default:
		return gymaccount.Principal{}, errors.New("unknown fixture account")
	}
	p, err := (fixtureProvider{}).Verify(ctx, assertion)
	if err != nil {
		return p, err
	}
	if reset {
		var nonce [32]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return gymaccount.Principal{}, errors.New("fixture recovery unavailable")
		}
		p.RecoveryVerified, p.RecoveryEventID = true, hex.EncodeToString(nonce[:])
	}
	return p, nil
}

func storeConfig() gymaccount.Config {
	return gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{
		{Issuer: fixtureIssuer, Subject: "zero256", Fixture: "zero256"},
		{Issuer: fixtureIssuer, Subject: "incrementing256", Fixture: "incrementing256"},
	}, MaxSessions: 4096, MaxChallenges: 4096, MaxEvents: 16384}
}
