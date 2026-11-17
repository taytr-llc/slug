// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymstable"
)

func previewHash(domain string, raw []byte, length bool) string {
	h := sha256.New()
	h.Write([]byte(domain))
	if length {
		var n [4]byte
		binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
		h.Write(n[:])
	}
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func PreviewConditionDigest(c fightercondition.Snapshot) (string, error) {
	if !stableTime(c.BornAtMS) || !stableTime(c.AsOfMS) || !canonicalManifestID(c.Seed) {
		return "", gymstable.ErrInvalid
	}
	seed, e := fighter.ParseSeed(c.Seed)
	if e != nil {
		return "", gymstable.ErrInvalid
	}
	dna, e := fighter.Generate(c.DNAVersion, seed)
	if e != nil {
		return "", gymstable.ErrInvalid
	}
	expected, e := fightercondition.Project(c.Version, dna, c.BornAtMS, c.AsOfMS, c.ProXP)
	if e != nil || expected != c {
		return "", gymstable.ErrInvalid
	}
	raw, e := json.Marshal(c)
	if e != nil || len(raw) > 4096 {
		return "", gymstable.ErrInvalid
	}
	return previewHash("SLUG fight preview condition\x00v1\x00", raw, true), nil
}
func PreviewSeed(r PreviewRecipe) (string, error) {
	if r.Version != 1 || r.EngineVersion != 1 || r.EngineID != previewEngineID || r.Rounds != 3 || r.Training != [2]uint32{} || !stableTime(r.AsOfMS) || r.Network != uint32(chaincfg.MainNetParams.Net) || r.Genesis != chaincfg.MainNetParams.GenesisHash.String() || !canonicalManifestID(r.StoreID) || !canonicalManifestID(r.SourceAuthorityID) {
		return "", gymstable.ErrInvalid
	}
	if _, e := fighterdescriptor.ResolveCandidate(r.DescriptorID); e != nil {
		return "", gymstable.ErrInvalid
	}
	for _, f := range r.Fighters {
		if !canonicalManifestID(f.ProspectID) || !canonicalManifestID(f.SourceRecordDigest) || !canonicalManifestID(f.ManifestDigest) || !canonicalManifestID(f.ConditionDigest) {
			return "", gymstable.ErrInvalid
		}
	}
	if r.Fighters[0].ProspectID >= r.Fighters[1].ProspectID {
		return "", gymstable.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > MaxPreviewRecipeSize {
		return "", gymstable.ErrInvalid
	}
	return previewHash("SLUG fight preview\x00v1\x00", raw, true), nil
}
func previewRecipe(p gymstable.PreviewPair) (PreviewRecipe, error) {
	r := PreviewRecipe{Version: 1, EngineVersion: 1, EngineID: previewEngineID, Network: p.Status.Pin.Source.Network, Genesis: p.Status.Pin.Source.Genesis, StoreID: p.Status.StoreID, SourceAuthorityID: p.Status.Pin.Source.AuthorityID, DescriptorID: p.Status.Pin.Source.DescriptorID, AsOfMS: p.AsOfMS, Rounds: 3}
	for i, f := range p.Fighters {
		d, e := PreviewConditionDigest(f.Condition)
		if e != nil {
			return PreviewRecipe{}, e
		}
		r.Fighters[i] = PreviewFighterBinding{ProspectID: f.Assignment.ProspectID, SourceRecordDigest: f.Assignment.SourceRecordDigest, ManifestDigest: f.Assignment.ManifestDigest, ConditionDigest: d}
	}
	if _, e := PreviewSeed(r); e != nil {
		return PreviewRecipe{}, e
	}
	return r, nil
}
func previewInput(p gymstable.PreviewPair, seed string) (fighterbout.Input, error) {
	f := func(i int) fighterbout.FighterInput {
		v := p.Fighters[i]
		return fighterbout.FighterInput{ProspectID: v.Assignment.ProspectID, Manifest: v.Manifest, BornAtMS: v.Assignment.BornAtMS}
	}
	return fighterbout.NewInput(p.AsOfMS, seed, f(0), f(1))
}
func previewPairValid(p gymstable.PreviewPair, v ManifestViewPin, store string) bool {
	if p.Version != 1 || !stableStatusValid(p.Status, v, store) || !stableTime(p.AsOfMS) || p.Status.LastAsOfMS != p.AsOfMS || p.Fighters[0].Assignment.ProspectID >= p.Fighters[1].Assignment.ProspectID || p.Fighters[0].Manifest.Physical.Class != p.Fighters[1].Manifest.Physical.Class {
		return false
	}
	for _, f := range p.Fighters {
		if !stableItemValid(f, p.Status, p.AsOfMS) {
			return false
		}
		if _, e := fightermanifest.Marshal(f.Manifest); e != nil {
			return false
		}
		md, e := fightermanifest.Digest(f.Manifest)
		if e != nil || md != f.Assignment.ManifestDigest {
			return false
		}
		if _, e := PreviewConditionDigest(f.Condition); e != nil {
			return false
		}
		raw, _ := json.Marshal(f)
		if len(raw) > gymstable.MaxItemSize {
			return false
		}
	}
	raw, _ := json.Marshal(p)
	return len(raw) <= MaxPreviewPairSize
}
