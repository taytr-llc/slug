// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyMinerReference struct {
	Composition struct {
		Positives []struct {
			Name  string
			Setup struct {
				Authority  registry.PublicManifestPin
				Pool       poolshare.Config
				Settlement poolsettle.Config
				Miners     Config
			}
			MinerConfigID string `json:"miner_config_id"`
		}
	}
	Credentials struct {
		Config   Config
		ConfigID string `json:"config_id"`
		Hashes   []struct {
			Audience, Token string
			TokenHash       string `json:"token_hash"`
			ReuseHash       string `json:"reuse_hash"`
		}
		Cases []struct {
			Name      string
			Valid     bool
			Canonical string `json:"canonical_hex"`
		}
	}
}

func loadDailyMinerReference(t *testing.T) dailyMinerReference {
	t.Helper()
	var v dailyMinerReference
	pinnedJSON(t, "../testdata/public-daily/reference-v1.json", "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3", &v)
	if len(v.Composition.Positives) != 5 || len(v.Credentials.Cases) != 14 || len(v.Credentials.Hashes) != 3 {
		t.Fatal("daily reference inventory")
	}
	return v
}
func dailyMinerBinding(t *testing.T) (Config, Binding) {
	t.Helper()
	x := loadDailyMinerReference(t).Composition.Positives[0]
	p, e := registry.NewOperatorProfile(x.Setup.Authority)
	if e != nil {
		t.Fatal(e)
	}
	return x.Setup.Miners, Binding{Profile: p, Source: x.Setup.Pool, Settlement: x.Setup.Settlement}
}
func TestDailyMinerIndependentBytesAndBindings(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal source identities only")
	}
	v := loadDailyMinerReference(t)
	for _, x := range v.Composition.Positives {
		t.Run(x.Name, func(t *testing.T) {
			p, e := registry.NewOperatorProfile(x.Setup.Authority)
			if e != nil {
				t.Fatal(e)
			}
			b := Binding{Profile: p, Source: x.Setup.Pool, Settlement: x.Setup.Settlement}
			id, e := DailyConfigID(x.Setup.Miners, b)
			if e != nil || id != x.MinerConfigID {
				t.Fatal("daily enrollment identity", e)
			}
			if _, e = ConfigID(x.Setup.Miners, b); e == nil {
				t.Fatal("legacy accepted daily")
			}
			raw, e := encodeMarkerVersion(x.Setup.Miners, Version2)
			if e != nil {
				t.Fatal(e)
			}
			got, e := decodeMarkerVersion(raw, Version2)
			if e != nil || !reflect.DeepEqual(got, x.Setup.Miners) {
				t.Fatal("daily marker", e)
			}
			if _, e = decodeMarker(raw); e == nil {
				t.Fatal("legacy marker accepted daily")
			}
			changed := cloneConfig(x.Setup.Miners)
			changed.Enrollments[0].PayeeScript = "0014" + strings.Repeat("11", 20)
			if ValidateDailyConfig(changed, b) == nil {
				t.Fatal("unbound payee")
			}
		})
	}
	for _, x := range v.Credentials.Cases {
		t.Run(x.Name, func(t *testing.T) {
			raw, e := hex.DecodeString(x.Canonical)
			if e != nil {
				t.Fatal(e)
			}
			s, e := decodeStateVersion(raw, Version2)
			if (e == nil) != x.Valid {
				t.Fatal("state classification", e)
			}
			if e == nil {
				got, e := encodeStateVersion(s, Version2)
				if e != nil || !bytes.Equal(got, raw) {
					t.Fatal("state bytes", e)
				}
			}
			if _, e = decodeState(raw); e == nil {
				t.Fatal("legacy state accepted daily")
			}
		})
	}
	for _, x := range v.Credentials.Hashes {
		got, e := tokenHashesVersion(v.Credentials.ConfigID, x.Audience, x.Token, Version2)
		if e != nil || got != (TokenUse{x.TokenHash, x.ReuseHash}) {
			t.Fatal("daily token domains", e)
		}
		old, e := tokenHashes(v.Credentials.ConfigID, x.Audience, x.Token)
		if e != nil || old == got {
			t.Fatal("cross-version token alias")
		}
	}
}
func TestDailyMinerRotationAudienceAndReopen(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile only")
	}
	c, b := dailyMinerBinding(t)
	ctx := minerContext(t)
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(parent, "daily")
	s, e := CreateDaily(ctx, path, c, b)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	now := s.state.ClockMS
	s.now = func() time.Time { return time.UnixMilli(now) }
	gym := c.Enrollments[0].GymID
	poolToken := strings.Repeat("71", 32)
	admissionToken := strings.Repeat("72", 32)
	nextToken := strings.Repeat("73", 32)
	first, e := s.Rotate(ctx, gym, PoolAudience, 0, poolToken, now+60000)
	if e != nil {
		t.Fatal(e)
	}
	before, e := s.db.Get(stateKey, nil)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.Rotate(ctx, gym, PoolAudience, 0, poolToken, now+60000)
	if e != nil || retry != first {
		t.Fatal("exact replay", e)
	}
	after, _ := s.db.Get(stateKey, nil)
	if !bytes.Equal(before, after) {
		t.Fatal("replay mutated journal")
	}
	if _, e = s.Rotate(ctx, gym, AdmissionAudience, 0, poolToken, now+60000); !errors.Is(e, ErrConflict) {
		t.Fatal("token reused", e)
	}
	if _, e = s.Rotate(ctx, gym, AdmissionAudience, 0, admissionToken, now+60000); e != nil {
		t.Fatal(e)
	}
	var escaped func() error
	fn := func(key string, check func() error) error {
		if key != c.Enrollments[0].PublicKey {
			t.Fatal("identity changed")
		}
		escaped = check
		return check()
	}
	if e = s.PoolSessions().WithSession(ctx, admissionToken, fn); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("audience crossed", e)
	}
	if e = s.PoolSessions().WithSession(ctx, poolToken, fn); e != nil {
		t.Fatal(e)
	}
	if escaped() == nil {
		t.Fatal("escaped capability")
	}
	if _, e = s.Rotate(ctx, gym, PoolAudience, 1, nextToken, now+60000); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Revoke(ctx, gym, PoolAudience, 2); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(ctx); e != nil {
		t.Fatal(e)
	}
	if other, e := Open(ctx, path, c, b); e == nil || other != nil {
		t.Fatal("legacy reopened daily")
	}
	s, e = OpenDaily(ctx, path, c, b)
	if e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return time.UnixMilli(now) }
	for _, token := range []string{poolToken, nextToken} {
		if e = s.PoolSessions().WithSession(ctx, token, fn); e == nil {
			t.Fatal("retired token revived")
		}
	}
	if e = s.AdmissionSessions().WithSession(ctx, admissionToken, fn); e != nil {
		t.Fatal(e)
	}
	raw, e := s.db.Get(stateKey, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{poolToken, admissionToken, nextToken} {
		if bytes.Contains(raw, []byte(token)) {
			t.Fatal("plaintext stored")
		}
	}
	now += 60000
	if e = s.AdmissionSessions().WithSession(ctx, admissionToken, fn); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("expired credential", e)
	}
	if e = s.Close(ctx); e != nil {
		t.Fatal(e)
	}
	s, e = OpenDaily(ctx, path, c, b)
	if e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return time.UnixMilli(now - 1) }
	if e = s.AdmissionSessions().WithSession(ctx, admissionToken, fn); !errors.Is(e, ErrClock) {
		t.Fatal("clock high-water lost", e)
	}
}
func TestDailyMinerRefusalBeforeEffects(t *testing.T) {
	c, b := dailyMinerBinding(t)
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(parent, "absent")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, e := CreateDaily(ctx, path, c, b)
	if s != nil || e == nil {
		t.Fatal("cancel/profile accepted")
	}
	if wire.TestPoW && !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("effects before refusal")
	}
	legacy := cloneConfig(c)
	legacy.Version = Version1
	if ValidateDailyConfig(legacy, b) == nil {
		t.Fatal("daily accepted legacy")
	}
	raw, _ := json.Marshal(c)
	if boundedJSON(raw, maxMarkerBytes, &Config{}) != nil {
		t.Fatal("fixture grammar")
	}
}
