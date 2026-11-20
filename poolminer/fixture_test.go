// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func pinnedJSON(t *testing.T, path, hash string, out any) {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != hash {
		t.Fatal("independent fixture changed")
	}
	if e = json.Unmarshal(raw, out); e != nil {
		t.Fatal(e)
	}
}
func minerBinding(t *testing.T) (Config, Binding) {
	t.Helper()
	var ac struct {
		Positives []struct {
			Family, Name string
			Value        json.RawMessage
		}
	}
	pinnedJSON(t, "../registry/testdata/operator-reference/authority-v2.json", "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4", &ac)
	var pin registry.PublicManifestPin
	found := false
	for _, x := range ac.Positives {
		if x.Family == "pin" && x.Name == "v1/operator_v2_distinct" {
			if json.Unmarshal(x.Value, &pin) != nil {
				t.Fatal("pin")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("pin absent")
	}
	profile, e := registry.NewOperatorProfile(pin)
	if e != nil {
		t.Fatal(e)
	}
	var wc struct {
		Guards []struct {
			Source     poolshare.Config
			Settlement poolsettle.Config
		}
	}
	pinnedJSON(t, "../poolsettle/testdata/operator-wallet/reference-v1.json", "c1f2d725fcdbc3af7ba1b0a30f4dd4dfc6983352ffb0b4f72d83f7c09ac7b087", &wc)
	if len(wc.Guards) != 3 {
		t.Fatal("wallet fixture")
	}
	b := Binding{Profile: profile, Source: wc.Guards[0].Source, Settlement: wc.Guards[0].Settlement}
	b.Source.AuthorityPin = pin.Source
	c := Config{Version: 1, CredentialID: strings.Repeat("11", 32), RuntimeID: strings.Repeat("22", 32), OperatorProfileID: profile.ID(), Enrollments: []Enrollment{}}
	for i, key := range []string{"0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5"} {
		p := b.Source.Policy
		gym, e := gymauth.GymID(prospect.Context{Version: p.Version, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.DNAVersion}, key)
		if e != nil {
			t.Fatal(e)
		}
		c.Enrollments = append(c.Enrollments, Enrollment{PublicKey: key, GymID: gym, PayeeScript: b.Settlement.Destinations[i].Script})
	}
	sort.Slice(c.Enrollments, func(i, j int) bool { return c.Enrollments[i].GymID < c.Enrollments[j].GymID })
	b.Settlement.Destinations = nil
	for _, x := range c.Enrollments {
		b.Settlement.Destinations = append(b.Settlement.Destinations, poolsettle.Destination{GymID: x.GymID, Script: x.PayeeScript})
	}
	c.SourceID, e = poolservice.BindingID(b.Source)
	if e != nil {
		t.Fatal(e)
	}
	c.SettlementConfigID, e = poolsettle.ConfigBindingID(b.Settlement)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidateConfig(c, b); e != nil {
		t.Fatal(e)
	}
	return c, b
}
func newMiner(t *testing.T) (*Store, string, Config, Binding, int64) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal profile only")
	}
	c, b := minerBinding(t)
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(parent, "miners")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, e := Create(ctx, p, c, b)
	if e != nil {
		t.Fatal(e)
	}
	now := s.state.ClockMS
	s.now = func() time.Time { return time.UnixMilli(now) }
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := s.Close(ctx); e != nil {
			t.Error(e)
		}
	})
	return s, p, c, b, now
}
func minerContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func reopenMiner(t *testing.T, s *Store, path string, c Config, b Binding, now int64) *Store {
	t.Helper()
	ctx := minerContext(t)
	if e := s.Close(ctx); e != nil {
		t.Fatal(e)
	}
	next, e := Open(ctx, path, c, b)
	if e != nil {
		t.Fatal(e)
	}
	next.now = func() time.Time { return time.UnixMilli(now) }
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := next.Close(ctx); e != nil {
			t.Error(e)
		}
	})
	return next
}
