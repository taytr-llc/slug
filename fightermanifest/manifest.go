// Copyright (c) 2026 The SLUG developers. ISC license.

// Package fightermanifest generates versioned public previews. A manifest is
// neither proof of qualifying work nor fighter identity, ownership or birth.
package fightermanifest

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"

	"github.com/taytr-llc/slug/fighter"
)

const (
	Version1        uint32 = 1
	GeneratorID            = "slug-fighter-manifest-candidate-v1"
	TableHash              = "72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44"
	MaxEncodedBytes        = 8192
	MaxDepth               = 8
	streamDomain           = "SLUG fighter manifest\x00v1\x00"
	tableDomain            = "SLUG fighter manifest tables\x00v1\x00"
	digestDomain           = "SLUG fighter manifest digest\x00v1\x00"
	maxDraws               = 256
)

var ErrManifest = errors.New("invalid fighter manifest")
var ErrStream = errors.New("fighter manifest stream exhausted")

//go:embed tables_v1.json
var encodedTables string

var names = func() struct {
	First []string `json:"first_names"`
	Last  []string `json:"last_names"`
} {
	var result struct {
		First []string `json:"first_names"`
		Last  []string `json:"last_names"`
	}
	digest := sha256.Sum256([]byte(tableDomain + encodedTables))
	if hex.EncodeToString(digest[:]) != TableHash || json.Unmarshal([]byte(encodedTables), &result) != nil || len(result.First) != 32 || len(result.Last) != 32 {
		panic("fighter manifest frozen tables mismatch")
	}
	return result
}()

type Name struct {
	First   string `json:"first"`
	Last    string `json:"last"`
	Display string `json:"display"`
}
type Physical struct {
	Height int    `json:"height_in"`
	Reach  int    `json:"reach_in"`
	Stance string `json:"stance"`
	Weight int    `json:"weight_lb"`
	Class  string `json:"weight_class"`
}
type Portrait struct {
	Version        uint32 `json:"portrait_version"`
	Skin           int    `json:"skin"`
	HairStyle      int    `json:"hair_style"`
	HairColor      int    `json:"hair_color"`
	FacialHair     int    `json:"facial_hair"`
	HeadShape      int    `json:"head_shape"`
	Mouth          int    `json:"mouth"`
	Brow           int    `json:"brow"`
	Background     int    `json:"background"`
	Scar           string `json:"scar"`
	ShoulderJitter int    `json:"shoulder_jitter"`
}
type Manifest struct {
	Version   uint32      `json:"manifest_version"`
	Generator string      `json:"generator_id"`
	Tables    string      `json:"table_hash"`
	DNA       fighter.DNA `json:"dna"`
	Name      Name        `json:"name"`
	Physical  Physical    `json:"physical"`
	Style     string      `json:"style"`
	Portrait  Portrait    `json:"portrait"`
}

type stream struct {
	seed      [32]byte
	label     string
	counter   uint64
	exhausted bool
}

func (s *stream) next() (uint64, error) {
	if s.exhausted {
		return 0, ErrStream
	}
	h := sha256.New()
	h.Write([]byte(streamDomain))
	h.Write([]byte(s.label))
	h.Write([]byte{0})
	h.Write(s.seed[:])
	var counter [8]byte
	binary.LittleEndian.PutUint64(counter[:], s.counter)
	h.Write(counter[:])
	if s.counter == math.MaxUint64 {
		s.exhausted = true
	} else {
		s.counter++
	}
	return binary.BigEndian.Uint64(h.Sum(nil)[:8]), nil
}
func uniform(n uint64, next func() (uint64, error)) (uint64, error) {
	if n == 0 {
		return 0, ErrStream
	}
	threshold := -n % n
	for i := 0; i < maxDraws; i++ {
		r, err := next()
		if err != nil {
			return 0, err
		}
		if r >= threshold {
			return r % n, nil
		}
	}
	return 0, ErrStream
}
func weightForTicket(ticket uint64) (int, error) {
	if ticket >= 329511 {
		return 0, ErrManifest
	}
	for weight := 128; weight <= 268; weight++ {
		mass := 91 * (weight - 127)
		if weight > 178 {
			mass = 51 * (269 - weight)
		}
		if ticket < uint64(mass) {
			return weight, nil
		}
		ticket -= uint64(mass)
	}
	return 0, ErrManifest
}
func heightFor(weight, jitter int) (int, error) {
	if weight < 128 || weight > 268 || jitter < -5 || jitter > 5 {
		return 0, ErrManifest
	}
	tenths := 640 + (weight - 128) + 5*jitter
	height, remainder := tenths/10, tenths%10
	if remainder > 5 || (remainder == 5 && height%2 == 1) {
		height++
	}
	if height < 62 {
		height = 62
	}
	if height > 80 {
		height = 80
	}
	return height, nil
}
func classFor(weight int) (string, error) {
	if weight < 128 || weight > 268 {
		return "", ErrManifest
	}
	switch {
	case weight <= 145:
		return "light", nil
	case weight <= 170:
		return "welter", nil
	case weight <= 195:
		return "middle", nil
	case weight <= 225:
		return "light_heavy", nil
	default:
		return "heavy", nil
	}
}
func styleFor(a fighter.Attributes, p Physical) string {
	offset := p.Reach - p.Height
	switch {
	case a.Power >= 70 && a.Chin >= 65 && a.Speed < 60:
		return "slugger"
	case a.Speed >= 70 && a.Skill >= 65 && offset >= 2:
		return "outfighter"
	case a.Speed >= 65 && a.Power >= 65:
		return "boxer_puncher"
	case a.Speed >= 65 && a.Stamina >= 70 && a.Skill >= 55 && offset <= 1:
		return "pressure"
	default:
		return "boxer"
	}
}

// Generate preserves DNA v1 and isolates every additional field's stream.
func Generate(version uint32, seed [32]byte) (Manifest, error) {
	switch version {
	case Version1:
		return generateV1(seed)
	case Version2:
		return generateNameV2(seed)
	default:
		return Manifest{}, ErrManifest
	}
}

// The original generator's streams and values remain frozen.
func generateV1(seed [32]byte) (Manifest, error) {
	dna, err := fighter.Generate(fighter.Version1, seed)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{Version: Version1, Generator: GeneratorID, Tables: TableHash, DNA: dna}
	var firstErr error
	draw := func(label string, n uint64) int {
		if firstErr != nil {
			return 0
		}
		s := stream{seed: seed, label: label}
		r, e := uniform(n, s.next)
		if e != nil {
			firstErr = e
		}
		return int(r)
	}
	m.Name.First = names.First[draw("name.first", 32)]
	m.Name.Last = names.Last[draw("name.last", 32)]
	m.Name.Display = m.Name.First + " " + m.Name.Last
	m.Physical.Weight, err = weightForTicket(uint64(draw("physical.weight", 329511)))
	if err != nil {
		return Manifest{}, err
	}
	m.Physical.Height, err = heightFor(m.Physical.Weight, draw("physical.height_jitter", 11)-5)
	if err != nil {
		return Manifest{}, err
	}
	m.Physical.Reach = m.Physical.Height + draw("physical.reach", 7) - 2
	m.Physical.Stance = "orthodox"
	if draw("physical.stance", 4) == 0 {
		m.Physical.Stance = "southpaw"
	}
	m.Physical.Class, err = classFor(m.Physical.Weight)
	if err != nil {
		return Manifest{}, err
	}
	m.Style = styleFor(m.DNA.Attributes, m.Physical)
	m.Portrait = Portrait{Version: 1, Skin: draw("portrait.skin", 6), HairStyle: draw("portrait.hair_style", 8), HairColor: draw("portrait.hair_color", 6), FacialHair: draw("portrait.facial_hair", 5), HeadShape: draw("portrait.head_shape", 4), Mouth: draw("portrait.mouth", 4), Brow: draw("portrait.brow", 3), Background: draw("portrait.background", 8), Scar: "none"}
	if draw("portrait.scar_chance", 100) < 35 {
		m.Portrait.Scar = []string{"left_brow", "right_cheek"}[draw("portrait.scar_site", 2)]
	}
	m.Portrait.ShoulderJitter = draw("portrait.shoulder_jitter", 13) - 6
	if firstErr != nil {
		return Manifest{}, firstErr
	}
	return m, nil
}

// GenerateText accepts exactly one display-order hex seed, preserving the DNA
// parser's uppercase convenience; canonical manifests always use lowercase.
func GenerateText(version uint32, text string) (Manifest, error) {
	seed, err := fighter.ParseSeed(text)
	if err != nil {
		return Manifest{}, ErrManifest
	}
	return Generate(version, seed)
}
func validate(m Manifest) error {
	seed, err := fighter.ParseSeed(m.DNA.Seed)
	if err != nil {
		return ErrManifest
	}
	want, err := Generate(m.Version, seed)
	if err != nil || m != want {
		return ErrManifest
	}
	return nil
}
func Marshal(m Manifest) ([]byte, error) {
	if err := validate(m); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > MaxEncodedBytes {
		return nil, ErrManifest
	}
	return raw, nil
}
func Digest(m Manifest) (string, error) {
	raw, err := Marshal(m)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	domain := digestDomain
	if m.Version == Version2 {
		domain = digestDomainV2
	}
	h.Write([]byte(domain))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}
