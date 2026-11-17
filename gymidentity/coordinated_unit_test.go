// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package gymidentity

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
)

func coordinatedUnitFixture(t *testing.T, kind string) (*Verifier, coordinatedReferenceCase) {
	t.Helper()
	file, config := coordinatedReferences(t)
	v := identityReferenceVerifier(t, config)
	for _, c := range file.Cases {
		if c.Accepted && c.Kind == kind {
			v.now = func() time.Time { return time.Unix(c.NowUnixSeconds, 0) }
			return v, c
		}
	}
	t.Fatal("missing independently signed valid fixture")
	return nil, coordinatedReferenceCase{}
}

func TestCoordinatedCancellationAndZeroOutput(t *testing.T) {
	for _, kind := range []string{"login", "reset"} {
		t.Run(kind, func(t *testing.T) {
			v, fixture := coordinatedUnitFixture(t, kind)
			verify := func(ctx context.Context, verifier *Verifier, token string) error {
				t.Helper()
				if kind == "login" {
					got, err := verifier.VerifyCoordinatedLogin(ctx, token, fixture.Binding.binding(), fixture.ExpectedFlowID, fixture.ExpectedNonce)
					if got != (CoordinatedLogin{}) {
						t.Fatal("failed operation returned trusted login output")
					}
					return err
				}
				got, err := verifier.VerifyCoordinatedReset(ctx, token, fixture.Binding.binding(), fixture.ExpectedEventID, fixture.ExpectedNonce)
				if got != (CoordinatedReset{}) {
					t.Fatal("failed operation returned trusted reset output")
				}
				return err
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := verify(ctx, v, fixture.Token); !errors.Is(err, context.Canceled) {
				t.Fatal("pre-verification cancellation lost", err)
			}
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			v.now = func() time.Time {
				cancel() // RSA and binding validation completed before the clock.
				return time.Unix(fixture.NowUnixSeconds, 0)
			}
			if err := verify(ctx, v, fixture.Token); !errors.Is(err, context.Canceled) {
				t.Fatal("late cancellation published trusted output", err)
			}
			v.now = func() time.Time { return time.Unix(fixture.NowUnixSeconds, 0) }
			for _, invalid := range []*Verifier{nil, {}} {
				if err := verify(context.Background(), invalid, fixture.Token); !errors.Is(err, ErrConfig) {
					t.Fatal("uninitialized verifier accepted", err)
				}
			}
			if err := verify(nil, v, fixture.Token); !errors.Is(err, ErrConfig) {
				t.Fatal("nil context accepted", err)
			}
			bad := fixture.Token + "private-assertion-sentinel"
			err := verify(context.Background(), v, bad)
			if !errors.Is(err, ErrAssertion) || strings.Contains(err.Error(), "sentinel") || strings.Contains(err.Error(), fixture.Token) {
				t.Fatal("error exposed assertion contents")
			}
		})
	}
}

func TestCoordinatedImmutableTrustConcurrentUse(t *testing.T) {
	v, login := coordinatedUnitFixture(t, "login")
	_, reset := coordinatedUnitFixture(t, "reset")
	if login.NowUnixSeconds != reset.NowUnixSeconds {
		t.Fatal("fixture clocks differ")
	}
	ctx := context.Background()
	wantLogin, err := v.VerifyCoordinatedLogin(ctx, login.Token, login.Binding.binding(), login.ExpectedFlowID, login.ExpectedNonce)
	if err != nil || wantLogin.Login.VerifierConfigHash != v.ConfigDigest() {
		t.Fatal("login trust binding", err)
	}
	wantReset, err := v.VerifyCoordinatedReset(ctx, reset.Token, reset.Binding.binding(), reset.ExpectedEventID, reset.ExpectedNonce)
	if err != nil || wantReset.Reset.VerifierConfigHash != v.ConfigDigest() {
		t.Fatal("reset trust binding", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				gotLogin, err := v.VerifyCoordinatedLogin(ctx, login.Token, login.Binding.binding(), login.ExpectedFlowID, login.ExpectedNonce)
				if err != nil || gotLogin != wantLogin {
					t.Error("concurrent login changed", err)
					return
				}
				gotReset, err := v.VerifyCoordinatedReset(ctx, reset.Token, reset.Binding.binding(), reset.ExpectedEventID, reset.ExpectedNonce)
				if err != nil || gotReset != wantReset {
					t.Error("concurrent reset changed", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestCoordinatedDoesNotChangeLegacyAcceptance(t *testing.T) {
	file := identityReferences(t)
	v := identityReferenceVerifier(t, file)
	_, bindingFixture := coordinatedUnitFixture(t, "login")
	count := 0
	for _, fixture := range file.Cases {
		if !fixture.Accepted {
			continue
		}
		count++
		v.now = func() time.Time { return time.Unix(fixture.NowUnixSeconds, 0) }
		if fixture.Kind == "login" {
			old, err := v.VerifyLogin(context.Background(), fixture.Token, fixture.ExpectedNonce)
			if err != nil || old == (gymaccount.VerifiedLogin{}) {
				t.Fatalf("old accepted format %s changed: %v", fixture.Name, err)
			}
			got, err := v.VerifyCoordinatedLogin(context.Background(), fixture.Token, bindingFixture.Binding.binding(), bindingFixture.ExpectedFlowID, fixture.ExpectedNonce)
			if err == nil || got != (CoordinatedLogin{}) {
				t.Fatal("ordinary assertion gained coordinated authority", fixture.Name)
			}
		} else {
			old, err := v.VerifyReset(context.Background(), fixture.Token)
			if err != nil || old == (gymaccount.VerifiedReset{}) {
				t.Fatalf("old accepted reset %s changed: %v", fixture.Name, err)
			}
			got, err := v.VerifyCoordinatedReset(context.Background(), fixture.Token, bindingFixture.Binding.binding(), fixture.Expected.EventID, bindingFixture.ExpectedNonce)
			if err == nil || got != (CoordinatedReset{}) {
				t.Fatal("ordinary reset gained coordinated authority", fixture.Name)
			}
		}
	}
	if count != 23 {
		t.Fatal("old accepted corpus inventory changed")
	}
}

func TestCoordinatedFixedEpochClaimDoesNotChangeLegacyConfig(t *testing.T) {
	c := unitConfig(t)
	c.EpochClaim = "provider_auth_epoch"
	v, err := NewVerifier(c)
	if err != nil {
		t.Fatal(err)
	}
	nonce := strings.Repeat("a", 64)
	token := unitSign(t, unitClaims(c, nonce))
	if _, err := v.VerifyLogin(context.Background(), token, nonce); err != nil {
		t.Fatal("legacy configured epoch claim stopped working", err)
	}
	_, fixture := coordinatedUnitFixture(t, "login")
	if got, err := v.VerifyCoordinatedLogin(context.Background(), token, fixture.Binding.binding(), fixture.ExpectedFlowID, nonce); !errors.Is(err, ErrConfig) || got != (CoordinatedLogin{}) {
		t.Fatal("coordinated protocol accepted a different epoch claim", err)
	}
}
