// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterevidence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"reflect"

	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
)

func checksum(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func lpDigest(domain string, raw []byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func framedID(domain string, context prospect.Context, tail string, policy bool) string {
	h := sha256.New()
	h.Write([]byte(domain))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], context.Network)
	h.Write(n[:])
	b, _ := hex.DecodeString(context.Genesis)
	h.Write(b)
	if policy {
		binary.LittleEndian.PutUint32(n[:], context.DNAVersion)
		h.Write(n[:])
	}
	b, _ = hex.DecodeString(tail)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateSuppliedShape checks cheap consistency, not normal work, admission,
// completeness or authority. Its return value owns all mutable slices.
func ValidateSuppliedShape(expected gymstable.Pin, a Archive) (Archive, error) {
	if _, ok := typedSize(reflect.ValueOf(expected), pinShape, 1); !ok {
		return Archive{}, ErrSourceMismatch
	}
	if _, err := gymstable.ValidatePin(expected); err != nil {
		return Archive{}, ErrSourceMismatch
	}
	if _, ok := typedSize(reflect.ValueOf(a), archiveShape, 1); !ok || a.Format != Format || a.Version != Version {
		return Archive{}, ErrInvalid
	}
	a.Admission.Proof.MerkleBranch = append([]string{}, a.Admission.Proof.MerkleBranch...)
	a.Rows = append([]gymstable.CareerHistoryRow{}, a.Rows...)
	r, c := a.Admission, a.Capture
	storeID, err := gymstable.ID(expected)
	if err != nil || c.Status.Pin != expected || c.Status.StoreID != storeID {
		return Archive{}, ErrSourceMismatch
	}
	j, p, v := r.Job, r.Proof, r.Verified
	if r.Version != registry.Version5 || r.Sequence < 1 || r.Sequence > gymstable.MaxAssignments || prospectjob.ValidateManifest(j) != nil {
		return Archive{}, ErrInvalid
	}
	if j.AuthorityID != expected.Source.AuthorityID || j.DescriptorID != expected.Source.DescriptorID || j.Context.Network != expected.Source.Network || j.Context.Genesis != expected.Source.Genesis || j.PublicKey != expected.Identity.GymPublicKey || j.GymID != expected.Identity.GymID {
		return Archive{}, ErrSourceMismatch
	}
	if r.Context != j.Context || p.Version != prospect.Version2 || p.JobID != j.ID || p.GymID != j.GymID || r.Authorization.Version != gymauth.Version2 || r.Authorization.PublicKey != j.PublicKey {
		return Archive{}, ErrInvalid
	}
	if prospect.ValidateProofStructure(r.Context, p) != nil || gymauth.VerifyAuthorization(r.Context, p, r.Authorization) != nil {
		return Archive{}, ErrInvalid
	}
	b := r.Birth
	if b.RuleVersion != j.RuleVersion || b.BornAtMS != j.IssuedAtMS || b.AcceptedAtMS < j.IssuedAtMS || b.AcceptedAtMS >= j.ExpiresAtMS {
		return Archive{}, ErrInvalid
	}
	if v.Version != prospect.Version2 || v.Verification != "supplied-context" || v.Network != r.Context.Network || v.Genesis != r.Context.Genesis || v.GymID != p.GymID || v.ParentHash != r.Context.ParentHash || v.ParentHeight != r.Context.ParentHeight ||
		v.PolicyID != framedID("SLUG prospect policy\x00v1\x00", r.Context, r.Context.ProspectTarget, true) || v.ProspectID != framedID("SLUG prospect id\x00v1\x00", r.Context, v.WorkHash, false) || v.WorkHash > r.Context.ProspectTarget {
		return Archive{}, ErrInvalid
	}
	// This call checks only the observation's consistency with supplied fields.
	// VerifySupplied repeats it after normal work establishes those fields.
	if registry.ValidateEligibility(r.Context, v, r.Eligibility) != nil {
		return Archive{}, ErrInvalid
	}
	d, err := fighterdescriptor.ResolveCandidate(expected.Source.DescriptorID)
	if err != nil {
		return Archive{}, ErrSourceMismatch
	}
	m, err := fightermanifest.GenerateText(d.ManifestVersion, v.WorkHash)
	if err != nil || m.DNA != v.DNA || m.DNA.DNAVersion != d.DNAVersion {
		return Archive{}, ErrInvalid
	}
	md, err := fightermanifest.Digest(m)
	if err != nil {
		return Archive{}, ErrInvalid
	}
	pd, err := fighterportrait.Digest(d.RendererVersion, m)
	if err != nil {
		return Archive{}, ErrInvalid
	}
	if r.StaticBinding != (registry.StaticBinding{Version: registry.StaticBindingVersion1, DescriptorID: expected.Source.DescriptorID, ManifestDigest: md, PortraitDigest: pd}) {
		return Archive{}, ErrInvalid
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > registry.MaxRecordSize {
		return Archive{}, ErrInvalid
	}
	assignment := gymstable.Assignment{Version: gymstable.Version1, Sequence: r.Sequence, ProspectID: v.ProspectID, GymID: v.GymID, WorkHash: v.WorkHash,
		BornAtMS: b.BornAtMS, AcceptedAtMS: b.AcceptedAtMS, DescriptorID: r.StaticBinding.DescriptorID, ManifestDigest: md, PortraitDigest: pd,
		SourceRecordDigest: lpDigest("SLUG stable source record\x00v1\x00", raw)}
	if assignment != c.Assignment {
		return Archive{}, ErrInvalid
	}
	if _, err = gymstable.ValidateCareerHistoryCapture(c); err != nil || len(a.Rows) != int(c.EventCount) {
		return Archive{}, ErrInvalid
	}
	awards, results := make(map[string]bool, len(a.Rows)), make(map[string]bool, len(a.Rows))
	for i, row := range a.Rows {
		if _, err = gymstable.ValidateCareerHistoryRow(row, c); err != nil || row.Ordinal != c.EventCount-uint32(i) || awards[row.Event.AwardID] || results[row.Event.ResultKey] {
			return Archive{}, ErrInvalid
		}
		awards[row.Event.AwardID], results[row.Event.ResultKey] = true, true
		if i > 0 {
			newer := a.Rows[i-1].Event
			e := row.Event
			if e.EffectiveAtMS > newer.EffectiveAtMS || e.EffectiveAtMS == newer.EffectiveAtMS && e.AwardID >= newer.AwardID {
				return Archive{}, ErrInvalid
			}
		}
	}
	total := fightercareer.Totals{}
	for i := len(a.Rows) - 1; i >= 0; i-- {
		row := a.Rows[i]
		before := total.XP
		total, err = fightercareer.Apply(total, row.Earned)
		if err != nil || row.XPBefore != before || row.XPApplied != total.XP-before || row.TotalsAfter != total {
			return Archive{}, ErrInvalid
		}
	}
	if total != c.Totals {
		return Archive{}, ErrInvalid
	}
	return a, nil
}

// Marshal serializes only bounded, cheap-checked supplied evidence. It never
// hashes proof work and never reports that work has been verified.
func Marshal(expected gymstable.Pin, archive Archive) ([]byte, error) {
	a, err := ValidateSuppliedShape(expected, archive)
	if err != nil {
		return nil, err
	}
	return checkedBytes(a)
}
