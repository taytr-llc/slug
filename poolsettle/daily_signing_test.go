// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailySigningCase struct {
	Name      string             `json:"name"`
	Body      signingRequestBody `json:"body"`
	Canonical string             `json:"canonical"`
	Digest    string             `json:"digest"`
}

func dailySigningCases(t *testing.T) []dailySigningCase {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-profile independent identities")
	}
	raw, err := os.ReadFile("testdata/daily-signing/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "a60c1444b447020576241d24674f10bfbed6370f9b1ec26d7e49531ba01e3598" {
		t.Fatal("independent signing codec corpus changed")
	}
	var corpus struct {
		Cases []dailySigningCase `json:"cases"`
		Bound struct {
			Size  int64 `json:"conservative_body_bytes"`
			Limit int64 `json:"limit"`
		} `json:"bound"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) != 6 || corpus.Bound.Size >= dailySigningBytes || corpus.Bound.Limit != dailySigningBytes {
		t.Fatal("independent bound/case contract")
	}
	return corpus.Cases
}
func TestDailySigningIndependentBytes(t *testing.T) {
	for _, c := range dailySigningCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			ordinary, err := json.Marshal(c.Body)
			if err != nil || string(ordinary) != c.Canonical {
				t.Fatal("closed body grammar changed", err)
			}
			digest, err := dailySigningDigest(context.Background(), c.Body)
			if err != nil || hex.EncodeToString(digest[:]) != c.Digest {
				t.Fatal("stream differs from independent canonical body", err)
			}
		})
	}
}

type conditionalDailyBackend struct {
	Backend
	sign func(context.Context, []Coin, []byte) ([]byte, error)
}

func (b *conditionalDailyBackend) Sign(ctx context.Context, c []Coin, raw []byte) ([]byte, error) {
	return b.sign(ctx, c, raw)
}

// This directly exercises the private synchronous capability's lifecycle using
// a conditional owner. It does not import credits, authorize real payment, or
// replace the separate concrete source/ledger/wallet acceptance proof.
func conditionalDailyOwner(t *testing.T, body signingRequestBody, b Backend) *Store {
	t.Helper()
	db, err := leveldb.OpenFile(filepath.Join(t.TempDir(), "conditional"), &opt.Options{ErrorIfExist: true})
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(state{Config: body.Config, Source: body.Source}, nil, b)
	s.daily = &dailyState{}
	s.db = db
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestDailySigningOpaqueOriginalLifetime(t *testing.T) {
	body := dailySigningCases(t)[0].Body
	unsigned, _ := hex.DecodeString(body.Unsigned)
	b := &conditionalDailyBackend{}
	s := conditionalDailyOwner(t, body, b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var captured *DailySigningCheck
	var saved context.Context
	b.sign = func(signCtx context.Context, coins []Coin, raw []byte) ([]byte, error) {
		saved = signCtx
		var err error
		captured, err = CaptureDailySigningCheck(signCtx, s, b, body.Config, body.Source, coins, raw)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 721; i++ {
			if err = captured.Check(signCtx); err != nil {
				t.Fatal(err)
			}
		}
		for _, run := range []func() (*DailySigningCheck, error){
			func() (*DailySigningCheck, error) {
				return CaptureDailySigningCheck(context.Background(), s, b, body.Config, body.Source, coins, raw)
			},
			func() (*DailySigningCheck, error) {
				return CaptureDailySigningCheck(signCtx, &Store{daily: &dailyState{}}, b, body.Config, body.Source, coins, raw)
			},
			func() (*DailySigningCheck, error) {
				return CaptureDailySigningCheck(signCtx, s, &conditionalDailyBackend{}, body.Config, body.Source, coins, raw)
			},
			func() (*DailySigningCheck, error) {
				c := body.Config
				c.MaxBatches--
				return CaptureDailySigningCheck(signCtx, s, b, c, body.Source, coins, raw)
			},
			func() (*DailySigningCheck, error) {
				source := body.Source
				source.MaxWorks--
				return CaptureDailySigningCheck(signCtx, s, b, body.Config, source, coins, raw)
			},
		} {
			if got, e := run(); got != nil || e == nil {
				t.Fatal("foreign DTO/owner admitted", e)
			}
		}
		if err = CheckSigningRequest(signCtx, s, b, body.Config, body.Source, coins, raw); !errors.Is(err, ErrInvalid) {
			t.Fatal("legacy checker adopted daily capability", err)
		}
		coins[0].Claims[0].Amount.Numerator = "1"
		if err = captured.Recheck(signCtx, coins, raw); !errors.Is(err, ErrInvalid) {
			t.Fatal("changed backend claims accepted", err)
		}
		if err = captured.Recheck(signCtx, body.Inputs, unsigned); err != nil {
			t.Fatal("backend mutation aliased original body", err)
		}
		cancel()
		if err = captured.Check(context.WithoutCancel(signCtx)); !errors.Is(err, context.Canceled) {
			t.Fatal("new context revived canceled original", err)
		}
		return nil, context.Canceled
	}
	if _, err := s.signSettlementRequest(ctx, body.Inputs, body.Payments, body.Change, body.FeeAtoms, unsigned); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if captured == nil || captured.Check(context.Background()) == nil {
		t.Fatal("opaque check survived Sign return")
	}
	if _, err := CaptureDailySigningCheck(context.WithoutCancel(saved), s, b, body.Config, body.Source, body.Inputs, unsigned); err == nil {
		t.Fatal("retained context reminted capability")
	}
	b.sign = func(signCtx context.Context, coins []Coin, raw []byte) ([]byte, error) {
		var err error
		captured, err = CaptureDailySigningCheck(signCtx, s, b, body.Config, body.Source, coins, raw)
		if err != nil {
			t.Fatal(err)
		}
		if err = captured.Recheck(signCtx, coins, raw); err != nil {
			t.Fatal(err)
		}
		return []byte("conditional-backend-return"), nil
	}
	if result, err := s.signSettlementRequest(context.Background(), body.Inputs, body.Payments, body.Change, body.FeeAtoms, unsigned); err != nil || string(result) != "conditional-backend-return" {
		t.Fatal("successful synchronous scope", err)
	}
	if captured.Check(context.Background()) == nil {
		t.Fatal("successful Sign retained its authority")
	}
	var zero DailySigningCheck
	if zero.Check(context.Background()) == nil {
		t.Fatal("zero check is authority")
	}
}

func TestDailySigningBoundsAndOwnerCopies(t *testing.T) {
	body := dailySigningCases(t)[0].Body
	for _, mutate := range []func(*signingRequestBody){
		func(b *signingRequestBody) { b.Inputs[0].Claims = make([]Claim, dailySigningClaims+1) },
		func(b *signingRequestBody) { b.Change.Claims = make([]Claim, dailySigningClaims+1) },
		func(b *signingRequestBody) {
			b.Inputs[0].Claims[0].Amount.Numerator = strings.Repeat("9", maxFractionDigits+1)
		},
		func(b *signingRequestBody) { b.Version = 1 },
		func(b *signingRequestBody) { b.Config.Version = 1 },
		func(b *signingRequestBody) { b.Source.Version = 1 },
	} {
		copy := clone(body)
		mutate(&copy)
		if _, err := dailySigningDigest(context.Background(), copy); err == nil {
			t.Fatal("unbounded/cross-profile body encoded")
		}
	}
	owned := dailyOwnedCoins(body.Inputs)
	owned[0].Claims[0].Amount.Numerator = "2"
	if body.Inputs[0].Claims[0].Amount.Numerator == "2" {
		t.Fatal("owned claims alias caller slice")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dailySigningDigest(ctx, body); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled encoding", err)
	}
}

func TestDailySigningCheapRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	if c, e := CaptureDailySigningCheck(context.Background(), nil, nil, Config{}, poolshare.Config{}, nil, nil); c != nil || !errors.Is(e, ErrProfile) {
		t.Fatal(c, e)
	}
	var check DailySigningCheck
	if e := check.Check(context.Background()); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	var store *Store
	if e := store.CheckDailyBackendBinding(context.Background(), nil, Config{}, poolshare.Config{}); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}
