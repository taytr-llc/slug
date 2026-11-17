// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
)

func canonicalHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// NewInput selects version1 and canonicalizes only the complete corner order.
func NewInput(asOfMS int64, seed string, a, b FighterInput) (Input, error) {
	if a.ProspectID > b.ProspectID {
		a, b = b, a
	}
	return ValidateInput(Input{Version: Version1, EngineID: EngineID,
		Mode: "standard", SeedKind: "explicit", AsOfMS: asOfMS, Seed: seed,
		A: a, B: b})
}

// ValidateInput regenerates both entire manifests and condition projections.
// Input is value-only, so the returned value has no shared mutable storage.
func ValidateInput(in Input) (Input, error) {
	_, err := validateInput(in)
	if err != nil {
		return Input{}, err
	}
	return in, nil
}

func validateInput(in Input) ([2]fightercondition.Snapshot, error) {
	var conditions [2]fightercondition.Snapshot
	if in.Version != Version1 || in.EngineID != EngineID ||
		in.Mode != "standard" || in.SeedKind != "explicit" ||
		in.AsOfMS <= 0 || in.AsOfMS > maxDateMS || !canonicalHash(in.Seed) ||
		!canonicalHash(in.A.ProspectID) || !canonicalHash(in.B.ProspectID) ||
		in.A.ProspectID >= in.B.ProspectID {
		return conditions, ErrInput
	}
	for i, f := range [2]FighterInput{in.A, in.B} {
		if f.BornAtMS <= 0 || f.BornAtMS > in.AsOfMS ||
			f.ProXP > 15 || f.TrainingSkill > 5 {
			return [2]fightercondition.Snapshot{}, ErrInput
		}
		// Marshal validates by bounded regeneration before encoding. It cannot
		// copy caller-provided long strings into a large JSON allocation.
		if _, err := fightermanifest.Marshal(f.Manifest); err != nil {
			return [2]fightercondition.Snapshot{}, ErrInput
		}
		c, err := fightercondition.Project(fightercondition.Version1,
			f.Manifest.DNA, f.BornAtMS, in.AsOfMS, int(f.ProXP))
		if err != nil {
			return [2]fightercondition.Snapshot{}, ErrInput
		}
		conditions[i] = c
	}
	if in.A.Manifest.Physical.Class != in.B.Manifest.Physical.Class {
		return [2]fightercondition.Snapshot{}, ErrInput
	}
	return conditions, nil
}

func inputID(in Input) (string, error) {
	raw, err := canonicalJSON(in, MaxInputBytes)
	if err != nil {
		return "", ErrInput
	}
	h := sha256.New()
	h.Write([]byte(inputDomain))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// InputID commits the complete validated input, including its explicit seed.
func InputID(in Input) (string, error) {
	if _, err := ValidateInput(in); err != nil {
		return "", err
	}
	return inputID(in)
}
