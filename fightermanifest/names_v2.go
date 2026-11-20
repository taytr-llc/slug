// Copyright (c) 2026 The SLUG developers. ISC license.
package fightermanifest

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
)

const (
	Version2       uint32 = 2
	GeneratorIDV2         = "slug-fighter-manifest-candidate-v2"
	TableHashV2           = "b537490e1e602e27de01975aeec36f5fb53ba9a3a35f98143e8ecb7f9e2687c2"
	streamDomainV2        = "SLUG fighter manifest\x00v2\x00"
	tableDomainV2         = "SLUG fighter manifest tables\x00v2\x00"
	digestDomainV2        = "SLUG fighter manifest digest\x00v2\x00"
)

//go:embed tables_v2.json
var encodedNamesV2 string

var namesV2 = func() struct {
	First []string `json:"first_names"`
	Last  []string `json:"last_names"`
} {
	var out struct {
		First []string `json:"first_names"`
		Last  []string `json:"last_names"`
	}
	digest := sha256.Sum256([]byte(tableDomainV2 + encodedNamesV2))
	if hex.EncodeToString(digest[:]) != TableHashV2 || json.Unmarshal([]byte(encodedNamesV2), &out) != nil || len(out.First) != 512 || len(out.Last) != 512 {
		panic("fighter manifest frozen v2 name tables mismatch")
	}
	return out
}()

// Name streams are separate from every inherited v1 trait stream.
type nameStreamV2 struct {
	seed      [32]byte
	label     string
	counter   uint64
	exhausted bool
}

func (s *nameStreamV2) next() (uint64, error) {
	if s.exhausted {
		return 0, ErrStream
	}
	h := sha256.New()
	h.Write([]byte(streamDomainV2))
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
func generateNameV2(seed [32]byte) (Manifest, error) {
	m, err := generateV1(seed)
	if err != nil {
		return Manifest{}, err
	}
	first := nameStreamV2{seed: seed, label: "name.first"}
	last := nameStreamV2{seed: seed, label: "name.last"}
	a, err := uniform(512, first.next)
	if err != nil {
		return Manifest{}, err
	}
	b, err := uniform(512, last.next)
	if err != nil {
		return Manifest{}, err
	}
	m.Version, m.Generator, m.Tables = Version2, GeneratorIDV2, TableHashV2
	m.Name = Name{First: namesV2.First[a], Last: namesV2.Last[b]}
	m.Name.Display = m.Name.First + " " + m.Name.Last
	return m, nil
}
