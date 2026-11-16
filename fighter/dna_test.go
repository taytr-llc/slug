// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fighter

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestParseSeed(t *testing.T) {
	text := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	seed, err := ParseSeed(strings.ToUpper(text))
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range seed {
		if value != byte(i) {
			t.Fatal("displayed hash byte order was reversed or changed")
		}
	}
	dna, err := Generate(Version1, seed)
	if err != nil || dna.Seed != text {
		t.Fatalf("uppercase normalization failed: %q, %v", dna.Seed, err)
	}
	for _, invalid := range []string{"", "01", text[:63], text + "0", " " + text, text + "\n", "0x" + text, strings.Repeat("z", 64)} {
		if _, err := ParseSeed(invalid); err == nil {
			t.Errorf("accepted invalid seed %q", invalid)
		}
	}
	for _, version := range []uint32{0, 2, ^uint32(0)} {
		if _, err := Generate(version, seed); err == nil {
			t.Errorf("accepted unsupported version %d", version)
		}
	}
}

func TestRarityTickets(t *testing.T) {
	tests := []struct {
		first, last      uint64
		name             string
		minimum, maximum int
	}{
		{0, 54, "club", 250, 299},
		{55, 81, "professional", 300, 349},
		{82, 93, "contender", 350, 399},
		{94, 98, "elite", 400, 449},
		{99, 99, "generational", 450, 489},
	}
	for _, test := range tests {
		for ticket := test.first; ticket <= test.last; ticket++ {
			tier, err := rarityForTicket(ticket)
			want := rarityTier{test.name, test.minimum, test.maximum}
			if err != nil || tier != want {
				t.Fatalf("ticket %d: got %+v, %v; want %+v", ticket, tier, err, want)
			}
		}
	}
	if _, err := rarityForTicket(100); err == nil {
		t.Fatal("accepted an out-of-range rarity ticket")
	}
}

func TestUniformBelowRejection(t *testing.T) {
	tests := []struct {
		bound uint64
		draws []uint64
		want  uint64
	}{
		{10, []uint64{0, 5, 6}, 6}, // 2^64 mod 10 = 6.
		{10, []uint64{25}, 5},
		{1, []uint64{^uint64(0)}, 0},
		{^uint64(0), []uint64{0, 1}, 1},
		{1 << 63, []uint64{^uint64(0)}, (1 << 63) - 1},
	}
	for _, test := range tests {
		calls := 0
		got := uniformBelow(test.bound, func() uint64 {
			if calls == len(test.draws) {
				t.Fatal("sampler rejected a permitted draw")
			}
			value := test.draws[calls]
			calls++
			return value
		})
		if got != test.want || calls != len(test.draws) {
			t.Fatalf("bound %d: got %d after %d draws; want %d after %d", test.bound, got, calls, test.want, len(test.draws))
		}
	}
}

func TestStreamReferenceVectors(t *testing.T) {
	// Independently derived with Python hashlib from literal domain bytes,
	// label, seed 00..1f, and little-endian counters. The nonzero counters
	// check encoding that ordinary small-bound generation rarely exercises.
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i)
	}
	for _, test := range []struct {
		counter uint64
		want    uint64
	}{
		{0, 0x16a54217febde4ae},
		{1, 0x71d05c11797f45c3},
		{256, 0x64a3c59a38dc4af6},
	} {
		s := stream{seed: seed, label: "attributes", counter: test.counter}
		if got := s.next(); got != test.want {
			t.Errorf("counter %d: got %016x, want %016x", test.counter, got, test.want)
		}
		if s.counter != test.counter+1 {
			t.Error("each draw attempt must advance the stream counter")
		}
	}
}

func attributeValues(a Attributes) [5]int {
	return [5]int{a.Power, a.Speed, a.Chin, a.Stamina, a.Skill}
}

func TestCompositionCountsAndUnranking(t *testing.T) {
	var total uint64
	for sum, count := range compositionCounts[5] {
		total += count
		if sum >= 5 && count != compositionCounts[5][505-sum] {
			t.Fatalf("complementary totals disagree at %d", sum)
		}
	}
	if total != 10000000000 {
		t.Fatalf("all five-attribute tuples total %d, want 100^5", total)
	}
	for sum := 2; sum <= 200; sum++ {
		want := sum - 1
		if sum > 101 {
			want = 201 - sum
		}
		if compositionCounts[2][sum] != uint64(want) {
			t.Fatalf("two-attribute count at %d does not match triangular distribution", sum)
		}
	}
	if compositionCounts[5][489] != 1365 {
		t.Fatal("total 489 must have C(15,4)=1365 tuples")
	}

	// All five possibilities at total six have a known lexical order.
	for rank := uint64(0); rank < 5; rank++ {
		got, err := unrankAttributes(6, rank)
		want := [5]int{1, 1, 1, 1, 1}
		want[4-int(rank)] = 2
		if err != nil || attributeValues(got) != want {
			t.Fatalf("rank %d: got %v, %v; want %v", rank, got, err, want)
		}
	}
	for _, sum := range []int{5, 250, 299, 300, 349, 350, 399, 400, 449, 450, 489, 499, 500} {
		count := compositionCounts[5][sum]
		for _, rank := range []uint64{0, count / 2, count - 1} {
			got, err := unrankAttributes(sum, rank)
			if err != nil {
				t.Fatal(err)
			}
			complement, err := unrankAttributes(505-sum, count-1-rank)
			if err != nil {
				t.Fatal(err)
			}
			values, complements := attributeValues(got), attributeValues(complement)
			actualSum := 0
			for i, value := range values {
				if value < 1 || value > 100 || value+complements[i] != 101 {
					t.Fatalf("invalid or asymmetric tuple at total %d rank %d: %v", sum, rank, values)
				}
				actualSum += value
			}
			if actualSum != sum {
				t.Fatalf("tuple total %d != %d", actualSum, sum)
			}
		}
	}
	for _, invalid := range []struct {
		total int
		rank  uint64
	}{{4, 0}, {501, 0}, {250, compositionCounts[5][250]}} {
		if _, err := unrankAttributes(invalid.total, invalid.rank); err == nil {
			t.Fatal("accepted an impossible composition rank or total")
		}
	}
}

func TestGenerateGoldenVectors(t *testing.T) {
	encoded, err := os.ReadFile("testdata/dna_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []DNA
	if err := json.Unmarshal(encoded, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("independent golden vectors are empty")
	}
	for _, want := range vectors {
		seed, err := ParseSeed(want.Seed)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Generate(want.DNAVersion, seed)
		if err != nil || got != want {
			t.Errorf("seed %s: got %+v, err %v; want %+v", want.Seed, got, err, want)
		}
	}
}

func TestGenerateBoundsAndConcurrency(t *testing.T) {
	check := func(seed [32]byte) {
		dna, err := Generate(Version1, seed)
		if err != nil {
			t.Error(err)
			return
		}
		var tier rarityTier
		for _, ticket := range []uint64{0, 55, 82, 94, 99} {
			candidate, _ := rarityForTicket(ticket)
			if candidate.name == dna.Rarity {
				tier = candidate
			}
		}
		if tier.name == "" || dna.TalentBudget < tier.minimum || dna.TalentBudget > tier.maximum {
			t.Errorf("budget outside rarity range: %+v", dna)
		}
		sum := 0
		for _, value := range attributeValues(dna.Attributes) {
			if value < 1 || value > 100 {
				t.Errorf("attribute outside [1,100]: %+v", dna)
			}
			sum += value
		}
		if sum != dna.TalentBudget || dna.PrimeWeeks < 26 || dna.PrimeWeeks > 52 || dna.DeclineWeeks < 13 || dna.DeclineWeeks > 39 {
			t.Errorf("invalid budget or career lengths: %+v", dna)
		}
		again, err := Generate(Version1, seed)
		if err != nil || !reflect.DeepEqual(dna, again) {
			t.Error("generation changed for the same version and seed")
		}
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := 0; i < 128; i++ {
				var seed [32]byte
				binary.LittleEndian.PutUint64(seed[:8], uint64(worker*128+i))
				check(seed)
			}
		}(worker)
	}
	workers.Wait()
}
