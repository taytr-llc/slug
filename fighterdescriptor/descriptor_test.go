// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterdescriptor

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
)

func TestDescriptorBoundariesAndSupport(t *testing.T) {
	d := CandidateDescriptor()
	if d.DNAVersion != 1 || d.ManifestVersion != fightermanifest.Version1 || d.GeneratorID != fightermanifest.GeneratorID || d.ManifestTableHash != fightermanifest.TableHash || d.RendererVersion != fighterportrait.Version1 || d.RendererID != fighterportrait.RendererID {
		t.Fatal("compiled implementation mapping changed")
	}
	id, err := ID(d)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveCandidate(id); err != nil || got != d {
		t.Fatal("candidate did not resolve", err)
	}
	if got, err := ResolveProduction(id); !errors.Is(err, ErrProductionUnavailable) || got != (Descriptor{}) {
		t.Fatal("candidate activated production")
	}
	unknown := d
	unknown.RendererVersion = math.MaxUint32
	unknown.RendererID = "Unknown_Bundle-2"
	unknown.ManifestTableHash = strings.Repeat("0", 64)
	raw, err := Marshal(unknown)
	if err != nil {
		t.Fatal("unsupported selectors not structurally encodable", err)
	}
	if got, err := Parse(raw); err != nil || got != unknown {
		t.Fatal("unknown tuple shape failed", err)
	}
	uid, err := ID(unknown)
	if err != nil || uid == id {
		t.Fatal("whole descriptor identity failed")
	}
	if got, err := ResolveCandidate(uid); !errors.Is(err, ErrUnsupportedDescriptor) || got != (Descriptor{}) {
		t.Fatal("unsupported tuple resolved")
	}
	if got, err := ResolveProduction(uid); !errors.Is(err, ErrProductionUnavailable) || got != (Descriptor{}) {
		t.Fatal("unknown tuple activated")
	}
	for _, bad := range []string{"", "latest", "0x" + id, id + "00", strings.ToUpper(id), strings.Repeat("z", 64)} {
		if got, err := ResolveCandidate(bad); !errors.Is(err, ErrDescriptor) || got != (Descriptor{}) {
			t.Fatal("malformed candidate ID accepted", bad)
		}
		if got, err := ResolveProduction(bad); !errors.Is(err, ErrDescriptor) || got != (Descriptor{}) {
			t.Fatal("malformed production ID accepted", bad)
		}
	}
	if got, err := ResolveCandidate(strings.Repeat("0", 64)); !errors.Is(err, ErrUnsupportedDescriptor) || got != (Descriptor{}) {
		t.Fatal("zero digest treated as malformed")
	}
	upper := d
	upper.ManifestTableHash = strings.ToUpper(upper.ManifestTableHash)
	upper.ManifestContractHash = strings.ToUpper(upper.ManifestContractHash)
	upper.RendererContractHash = strings.ToUpper(upper.RendererContractHash)
	if got, err := Normalize(upper); err != nil || got != d {
		t.Fatal("explicit normalization failed", err)
	}
	if Validate(upper) == nil {
		t.Fatal("validation silently normalized")
	}
	if raw, err := Marshal(upper); err == nil || raw != nil {
		t.Fatal("serialization silently normalized")
	}
	if id, err := ID(upper); err == nil || id != "" {
		t.Fatal("ID silently normalized")
	}
	changed := CandidateDescriptor()
	changed.RendererID = "caller-mutated"
	if CandidateDescriptor() != d {
		t.Fatal("candidate state mutated")
	}
}

func TestDescriptorAllFieldsAndLimits(t *testing.T) {
	base := CandidateDescriptor()
	raw, err := Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		t.Fatal("bad canonical fixture")
	}
	for key := range object {
		t.Run(key, func(t *testing.T) {
			for _, replacement := range []json.RawMessage{json.RawMessage("null"), json.RawMessage("true"), json.RawMessage("[]"), json.RawMessage("{}")} {
				prefix := []byte(`"` + key + `":`)
				needle := append(bytes.Clone(prefix), object[key]...)
				bad := bytes.Replace(raw, needle, append(bytes.Clone(prefix), replacement...), 1)
				if got, err := Parse(bad); err == nil || got != (Descriptor{}) {
					t.Fatal("invalid scalar accepted")
				}
			}
			name := []byte(`"` + key + `":`)
			dup := bytes.Replace(raw, name, append(append(bytes.Clone(name), object[key]...), append([]byte(","), name...)...), 1)
			if got, err := Parse(dup); err == nil || got != (Descriptor{}) {
				t.Fatal("duplicate accepted")
			}
		})
	}
	for i := 0; i < reflect.TypeOf(base).NumField(); i++ {
		d := base
		value := reflect.ValueOf(&d).Elem().Field(i)
		switch value.Kind() {
		case reflect.Uint32:
			value.SetUint(0)
		case reflect.String:
			value.SetString("")
		}
		if got, err := Normalize(d); err == nil || got != (Descriptor{}) {
			t.Fatal("missing typed field normalized", i)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 97), " leading", "trailing ", "a/b", "a.b", "é", "<svg>", "a\x00b"} {
		d := base
		d.GeneratorID = bad
		if Validate(d) == nil {
			t.Fatal("bad identifier accepted", bad)
		}
	}
	d := base
	d.GeneratorID = strings.Repeat("A", 96)
	d.RendererID = strings.Repeat("_", 96)
	d.DNAVersion = math.MaxUint32
	if Validate(d) != nil {
		t.Fatal("valid maximum values rejected")
	}
	for _, bad := range [][]byte{nil, []byte("null"), []byte("[]"), []byte("{}"), append(bytes.Clone(raw), '\n'), append([]byte(" "), raw...), append(bytes.Clone(raw), raw...), []byte(strings.Repeat(" ", MaxDescriptorSize+1)), bytes.Replace(raw, []byte(`"version"`), []byte(`"ver\u0073ion"`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1.0`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":-0`), 1), bytes.Replace(raw, []byte(`"version"`), []byte(`"VERSION"`), 1)} {
		if got, err := Parse(bad); err == nil || got != (Descriptor{}) {
			t.Fatal("noncanonical descriptor accepted")
		}
	}
	if got, err := Parse([]byte(strings.Repeat(`{"x":`, 100) + "1" + strings.Repeat("}", 100))); err == nil || got != (Descriptor{}) {
		t.Fatal("deep object accepted")
	}
}

func TestDescriptorConcurrentOwnership(t *testing.T) {
	d := CandidateDescriptor()
	want, err := Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := ID(d)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				raw, err := Marshal(d)
				if err != nil || !bytes.Equal(raw, want) {
					t.Error("marshal changed", err)
					return
				}
				parsed, err := Parse(raw)
				if err != nil || parsed != d {
					t.Error("parse changed", err)
					return
				}
				raw[0] = 'X'
				got, err := ResolveCandidate(id)
				if err != nil || got != d {
					t.Error("resolve changed", err)
					return
				}
				got.GeneratorID = "changed"
			}
		}()
	}
	wg.Wait()
}

func FuzzDescriptorParse(f *testing.F) {
	raw, _ := Marshal(CandidateDescriptor())
	f.Add(raw)
	f.Add([]byte("{}"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		d, err := Parse(raw)
		if err != nil {
			if d != (Descriptor{}) {
				t.Fatal("partial descriptor")
			}
			return
		}
		canonical, err := Marshal(d)
		if err != nil || !bytes.Equal(canonical, raw) {
			t.Fatal("noncanonical success")
		}
		if production, err := ResolveProduction(strings.Repeat("0", 64)); !errors.Is(err, ErrProductionUnavailable) || production != (Descriptor{}) {
			t.Fatal("production opened")
		}
	})
}
