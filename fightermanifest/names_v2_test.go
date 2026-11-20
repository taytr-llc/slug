// Copyright (c) 2026 The SLUG developers. ISC license.
package fightermanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strconv"
	"testing"
)

func readNameV2Reference(t *testing.T, out any) {
	t.Helper()
	raw, err := os.ReadFile("../fighterdescriptor/testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent names-v2 corpus changed")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}

func TestNameV2IndependentManifests(t *testing.T) {
	var ref struct {
		Cases []struct {
			Seed       string
			FirstIndex int `json:"first_index"`
			LastIndex  int `json:"last_index"`
			Manifest   Manifest
			Canonical  string
			Digest     string `json:"manifest_digest"`
		}
		Invalid []struct {
			Name     string
			Input    string `json:"input_hex"`
			Accepted bool
		} `json:"invalid_manifest_inputs"`
	}
	readNameV2Reference(t, &ref)
	if len(ref.Cases) != 764 || len(ref.Invalid) != 18 {
		t.Fatal("incomplete independent corpus")
	}
	first, last := map[int]bool{}, map[int]bool{}
	for _, c := range ref.Cases {
		m, err := GenerateText(Version2, c.Seed)
		if err != nil || m != c.Manifest {
			t.Fatalf("v2 manifest %s: %v", c.Seed, err)
		}
		raw, err := Marshal(m)
		if err != nil || !bytes.Equal(raw, []byte(c.Canonical)) {
			t.Fatalf("canonical %s: %v", c.Seed, err)
		}
		parsed, err := Parse(raw)
		if err != nil || parsed != m {
			t.Fatalf("parse %s: %v", c.Seed, err)
		}
		digest, err := Digest(m)
		if err != nil || digest != c.Digest {
			t.Fatalf("digest %s: %v", c.Seed, err)
		}
		old, err := GenerateText(Version1, c.Seed)
		if err != nil || m.DNA != old.DNA || m.Physical != old.Physical || m.Style != old.Style || m.Portrait != old.Portrait {
			t.Fatalf("non-name trait changed for %s", c.Seed)
		}
		first[c.FirstIndex], last[c.LastIndex] = true, true
	}
	if len(first) != 512 || len(last) != 512 {
		t.Fatal("table index coverage incomplete")
	}
	for _, c := range ref.Invalid {
		raw, err := hex.DecodeString(c.Input)
		if err != nil {
			t.Fatal(err)
		}
		_, err = Parse(raw)
		if c.Accepted || err == nil {
			t.Fatal("accepted independently invalid manifest", c.Name)
		}
	}
}

func TestNameV2IndependentStreams(t *testing.T) {
	var ref struct {
		Streams []struct {
			Seed, Label string
			Counter     string `json:"counter_decimal"`
			Value       string `json:"prefix_uint64_decimal"`
			Index       uint64 `json:"index512"`
		}
	}
	readNameV2Reference(t, &ref)
	if len(ref.Streams) != 24 {
		t.Fatal("incomplete streams")
	}
	for _, c := range ref.Streams {
		raw, err := hex.DecodeString(c.Seed)
		if err != nil || len(raw) != 32 {
			t.Fatal("seed")
		}
		var seed [32]byte
		copy(seed[:], raw)
		counter, err := strconv.ParseUint(c.Counter, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		want, err := strconv.ParseUint(c.Value, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		s := nameStreamV2{seed: seed, label: c.Label, counter: counter}
		got, err := s.next()
		if err != nil || got != want || got%512 != c.Index {
			t.Fatalf("stream %s %s %s", c.Seed, c.Label, c.Counter)
		}
		if counter == math.MaxUint64 {
			if _, err = s.next(); !errors.Is(err, ErrStream) {
				t.Fatal("counter wrapped")
			}
		}
	}
}
