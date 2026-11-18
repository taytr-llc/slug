// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func careerPreviewBounded(v any, limit int) bool {
	raw, e := json.Marshal(v)
	return e == nil && len(raw) <= limit
}

// These arithmetic constraints describe the fixed standard-bout candidate
// policy. They validate transport shape, never the authority of an award.
func careerPreviewTotalsValid(t fightercareer.Totals) bool {
	n := uint64(t.Wins) + uint64(t.Losses) + uint64(t.Draws)
	return n <= 256 && uint64(t.XP) == min(uint64(15), n) &&
		t.BasePoints >= 10*t.Wins+4*t.Draws && t.BasePoints <= 14*t.Wins+4*t.Draws &&
		(t.BasePoints-10*t.Wins-4*t.Draws)%4 == 0 && careerPreviewBounded(t, 128)
}

func CareerPreviewConditionDigest(c fightercondition.Snapshot) (string, error) {
	// The unchanged validator accepts already-accounted XP and performs only
	// bounded DNA/condition arithmetic. Its old digest domain is not reused.
	if _, e := PreviewConditionDigest(c); e != nil {
		return "", gymstable.ErrInvalid
	}
	raw, e := json.Marshal(c)
	if e != nil {
		return "", gymstable.ErrInvalid
	}
	return previewHash("SLUG career fight preview condition\x00v1\x00", raw, true), nil
}

func CareerPreviewSeed(r CareerPreviewRecipe) (string, error) {
	if r.Version != 1 || r.EngineVersion != 1 || r.EngineID != careerPreviewEngineID || r.Rounds != 3 || r.Training != [2]uint32{} || !stableTime(r.AsOfMS) || r.Network != uint32(chaincfg.MainNetParams.Net) || r.Genesis != chaincfg.MainNetParams.GenesisHash.String() || !canonicalManifestID(r.StoreID) || !canonicalManifestID(r.SourceAuthorityID) || !canonicalManifestID(r.ActivationID) || !canonicalManifestID(r.CareerDigest) {
		return "", gymstable.ErrInvalid
	}
	if _, e := fighterdescriptor.ResolveCandidate(r.DescriptorID); e != nil {
		return "", gymstable.ErrInvalid
	}
	for _, f := range r.Fighters {
		if !canonicalManifestID(f.ProspectID) || !canonicalManifestID(f.SourceRecordDigest) || !canonicalManifestID(f.ManifestDigest) || !canonicalManifestID(f.ConditionDigest) || !careerPreviewTotalsValid(f.Totals) {
			return "", gymstable.ErrInvalid
		}
	}
	if r.Fighters[0].ProspectID >= r.Fighters[1].ProspectID {
		return "", gymstable.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > MaxCareerPreviewRecipeSize {
		return "", gymstable.ErrInvalid
	}
	return previewHash("SLUG career fight preview\x00v1\x00", raw, true), nil
}

func careerPreviewRecipe(p gymstable.CareerPreviewPair) (CareerPreviewRecipe, error) {
	r := CareerPreviewRecipe{Version: 1, EngineVersion: 1, EngineID: careerPreviewEngineID, Network: p.Status.Pin.Source.Network, Genesis: p.Status.Pin.Source.Genesis, StoreID: p.Status.StoreID, SourceAuthorityID: p.Status.Pin.Source.AuthorityID, DescriptorID: p.Status.Pin.Source.DescriptorID, ActivationID: p.Activation.ActivationID, CareerDigest: p.CareerDigest, AsOfMS: p.AsOfMS, Rounds: 3}
	for i, f := range p.Fighters {
		d, e := CareerPreviewConditionDigest(f.Condition)
		if e != nil {
			return CareerPreviewRecipe{}, e
		}
		r.Fighters[i] = CareerPreviewFighterBinding{ProspectID: f.Assignment.ProspectID, SourceRecordDigest: f.Assignment.SourceRecordDigest, ManifestDigest: f.Assignment.ManifestDigest, ConditionDigest: d, Totals: f.Totals}
	}
	if _, e := CareerPreviewSeed(r); e != nil {
		return CareerPreviewRecipe{}, e
	}
	return r, nil
}

func careerPreviewInput(p gymstable.CareerPreviewPair, seed string) (fighterbout.Input, error) {
	f := func(i int) fighterbout.FighterInput {
		v := p.Fighters[i]
		return fighterbout.FighterInput{ProspectID: v.Assignment.ProspectID, Manifest: v.Manifest, BornAtMS: v.Assignment.BornAtMS, ProXP: v.Totals.XP}
	}
	return fighterbout.NewInput(p.AsOfMS, seed, f(0), f(1))
}

func careerPreviewPairValid(p gymstable.CareerPreviewPair, v ManifestViewPin, store string) bool {
	if p.Version != 1 || !stableViewValid(v) || !stableStatusValid(p.Status, v, store) || !stableTime(p.AsOfMS) || p.Status.LastAsOfMS != p.AsOfMS || careerstate.ValidateActivation(p.Activation) != nil || p.AsOfMS < p.Activation.ActivatedAtMS || !canonicalManifestID(p.CareerDigest) || p.Fighters[0].Assignment.ProspectID >= p.Fighters[1].Assignment.ProspectID || p.Fighters[0].Manifest.Physical.Class != p.Fighters[1].Manifest.Physical.Class || !careerPreviewBounded(v, 1024) || !careerPreviewBounded(p.Status, 4096) || !careerPreviewBounded(p.Activation, 512) {
		return false
	}
	for _, f := range p.Fighters {
		a, m := f.Assignment, f.Manifest
		// Validate static assignment fields directly: the legacy Item validator
		// intentionally requires XP0 and must never be relaxed for this family.
		if a.Version != 1 || a.Sequence == 0 || a.Sequence > p.Status.Checkpoint || a.GymID != p.Status.Pin.Identity.GymID || !canonicalManifestID(a.ProspectID) || !canonicalManifestID(a.WorkHash) || !canonicalManifestID(a.ManifestDigest) || !canonicalManifestID(a.PortraitDigest) || !canonicalManifestID(a.SourceRecordDigest) || a.DescriptorID != p.Status.Pin.Source.DescriptorID || !stableTime(a.BornAtMS) || !stableTime(a.AcceptedAtMS) || a.AcceptedAtMS < a.BornAtMS || a.AcceptedAtMS >= a.BornAtMS+300000 || p.AsOfMS < a.AcceptedAtMS || !stableManifestShape(m, a.WorkHash) || !careerPreviewTotalsValid(f.Totals) {
			return false
		}
		if _, e := fightermanifest.Marshal(m); e != nil {
			return false
		}
		md, e := fightermanifest.Digest(m)
		if e != nil || md != a.ManifestDigest {
			return false
		}
		expected, e := fightercondition.Project(1, m.DNA, a.BornAtMS, p.AsOfMS, int(f.Totals.XP))
		if e != nil || expected != f.Condition || !careerPreviewBounded(f, 8448) {
			return false
		}
	}
	return careerPreviewBounded(p, MaxCareerPreviewPairSize)
}
