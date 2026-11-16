// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fighter

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Version1 is the supported version of the DNA component.
const Version1 uint32 = 1

const streamDomain = "SLUG fighter DNA\x00v1\x00"

// Attributes are generated base values, before aging or experience.
type Attributes struct {
	Power   int `json:"power"`
	Speed   int `json:"speed"`
	Chin    int `json:"chin"`
	Stamina int `json:"stamina"`
	Skill   int `json:"skill"`
}

// DNA contains the reproducible outputs of this component. It makes no claim
// about a seed's proof of work, discovery time, registration, or ownership.
type DNA struct {
	DNAVersion   uint32     `json:"dna_version"`
	Seed         string     `json:"seed"`
	Rarity       string     `json:"rarity"`
	TalentBudget int        `json:"talent_budget"`
	Attributes   Attributes `json:"attributes"`
	PrimeWeeks   int        `json:"prime_weeks"`
	DeclineWeeks int        `json:"decline_weeks"`
}

// ParseSeed decodes exactly 64 hexadecimal characters in displayed/RPC order.
// Uppercase is accepted; Generate emits the normalized lowercase spelling.
// Prefixes, surrounding whitespace, and shortened hashes are not accepted.
func ParseSeed(value string) ([32]byte, error) {
	var seed [32]byte
	if len(value) != hex.EncodedLen(len(seed)) {
		return seed, fmt.Errorf("fighter seed must contain exactly 64 hexadecimal characters")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return seed, fmt.Errorf("invalid fighter seed: %w", err)
	}
	copy(seed[:], decoded)
	return seed, nil
}

type rarityTier struct {
	name             string
	minimum, maximum int
}

func rarityForTicket(ticket uint64) (rarityTier, error) {
	switch {
	case ticket < 55:
		return rarityTier{"club", 250, 299}, nil
	case ticket < 82:
		return rarityTier{"professional", 300, 349}, nil
	case ticket < 94:
		return rarityTier{"contender", 350, 399}, nil
	case ticket < 99:
		return rarityTier{"elite", 400, 449}, nil
	case ticket == 99:
		return rarityTier{"generational", 450, 489}, nil
	default:
		return rarityTier{}, fmt.Errorf("rarity ticket %d is outside [0,100)", ticket)
	}
}

type stream struct {
	seed    [32]byte
	label   string
	counter uint64
}

func (s *stream) next() uint64 {
	hasher := sha256.New()
	hasher.Write([]byte(streamDomain))
	hasher.Write([]byte(s.label))
	hasher.Write([]byte{0})
	hasher.Write(s.seed[:])
	var counter [8]byte
	binary.LittleEndian.PutUint64(counter[:], s.counter)
	hasher.Write(counter[:])
	s.counter++
	digest := hasher.Sum(nil)
	return binary.BigEndian.Uint64(digest[:8])
}

// uniformBelow uses unsigned wraparound to compute 2^64 mod n without an
// unrepresentable 2^64 constant. All callers supply a positive bound.
func uniformBelow(n uint64, next func() uint64) uint64 {
	threshold := -n % n
	for {
		r := next()
		if r >= threshold {
			return r % n
		}
	}
}

// compositionCounts is initialized once and only read thereafter. Entry [p][s]
// counts ordered p-tuples in [1,100] summing to s. Even the sum of all five-part
// counts is only 100^5, comfortably below uint64's limit.
var compositionCounts = buildCompositionCounts()

func buildCompositionCounts() [6][501]uint64 {
	var counts [6][501]uint64
	counts[0][0] = 1
	for parts := 1; parts <= 5; parts++ {
		for sum := parts; sum <= parts*100; sum++ {
			for value := 1; value <= 100 && value <= sum; value++ {
				counts[parts][sum] += counts[parts-1][sum-value]
			}
		}
	}
	return counts
}

// unrankAttributes selects a bounded tuple in lexicographic Power, Speed,
// Chin, Stamina, Skill order. Uniform ranks give every feasible ordered tuple
// equal probability, without clamping or favoring an attribute position.
func unrankAttributes(total int, rank uint64) (Attributes, error) {
	if total < 5 || total > 500 || rank >= compositionCounts[5][total] {
		return Attributes{}, fmt.Errorf("invalid attribute composition: total=%d rank=%d", total, rank)
	}
	var values [5]int
	remaining := total
	for position := range values {
		selected := false
		for value := 1; value <= 100 && value <= remaining; value++ {
			ways := compositionCounts[4-position][remaining-value]
			if rank >= ways {
				rank -= ways
				continue
			}
			values[position] = value
			remaining -= value
			selected = true
			break
		}
		if !selected {
			return Attributes{}, fmt.Errorf("attribute composition could not be unranked")
		}
	}
	return Attributes{values[0], values[1], values[2], values[3], values[4]}, nil
}

// Generate derives DNA using the requested version and a displayed-order seed.
// Each field uses an independent labeled SHA256 stream. It accepts any seed;
// proof validation and the choice of version for a discovery belong to callers.
func Generate(version uint32, seed [32]byte) (DNA, error) {
	if version != Version1 {
		return DNA{}, fmt.Errorf("unsupported fighter DNA version %d", version)
	}
	draw := func(label string, n uint64) uint64 {
		fieldStream := stream{seed: seed, label: label}
		return uniformBelow(n, fieldStream.next)
	}
	tier, err := rarityForTicket(draw("rarity", 100))
	if err != nil {
		return DNA{}, err
	}
	budget := tier.minimum + int(draw("budget", uint64(tier.maximum-tier.minimum+1)))
	rank := draw("attributes", compositionCounts[5][budget])
	attributes, err := unrankAttributes(budget, rank)
	if err != nil {
		return DNA{}, err
	}
	return DNA{
		DNAVersion:   version,
		Seed:         hex.EncodeToString(seed[:]),
		Rarity:       tier.name,
		TalentBudget: budget,
		Attributes:   attributes,
		PrimeWeeks:   26 + int(draw("prime", 27)),
		DeclineWeeks: 13 + int(draw("decline", 27)),
	}, nil
}
