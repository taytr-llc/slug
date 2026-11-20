// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

const (
	PublicVersion          uint32 = 1
	MaxPublicAdmissionSize        = 69632
	MaxPublicSmallSize            = 8192
	MaxPublicTimestampMS   int64  = 253402300799999
	publicZero                    = "0000000000000000000000000000000000000000000000000000000000000000"
)

var ErrPublicOwnership = errors.New("invalid public ownership evidence")
var publicLimits = PublicOwnershipLimits{1000, 16384, 4096, 1024, 256, 65536, 268435456}

func PublicOwnershipDefaultLimits() PublicOwnershipLimits { return publicLimits }
func publicFixtureKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x73}, 32))
}
func PublicOwnershipFixtureKey() string {
	return hex.EncodeToString(publicFixtureKey().Public().(ed25519.PublicKey))
}
func PublicOwnershipCheckpointKey() string {
	seed := sha256.Sum256([]byte("SLUG candidate seed checkpoint fixture\x00v1\x00"))
	return hex.EncodeToString(ed25519.NewKeyFromSeed(seed[:]).Public().(ed25519.PublicKey))
}
func PublicOwnershipRuleID() string {
	d := sha256.Sum256([]byte("SLUG ownership and direct transfer\x00v1\x00"))
	return hex.EncodeToString(d[:])
}
func publicHex(v string, n int, nonzero bool) bool {
	if len(v) != n*2 || nonzero && v == strings.Repeat("0", n*2) {
		return false
	}
	for _, c := range []byte(v) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func publicPrefix(n uint32, h string) bool {
	return publicHex(h, 32, false) && (n == 0) == (h == publicZero)
}
func publicHash(domain string, raw []byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{0, 'v', '1', 0})
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

// PublicOwnershipDigest is a byte commitment only, never verified evidence.
func PublicOwnershipDigest(domain string, value any) (string, error) {
	raw, e := json.Marshal(value)
	if e != nil || len(raw) > MaxPublicAdmissionSize {
		return "", ErrPublicOwnership
	}
	return publicHash(domain, raw), nil
}

// publicDecode rejects aliases and excess allocation before materializing DTOs.
// A final typed roundtrip also rejects missing fields, number aliases, reordered
// members (including nested DTOs), noncanonical escapes and unknown members.
func publicDecode(raw []byte, cap int, value any) error {
	if len(raw) == 0 || len(raw) > cap {
		return ErrPublicOwnership
	}
	for _, c := range raw {
		if c > 127 {
			return ErrPublicOwnership
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	tokens := 0
	var visit func(int) error
	visit = func(depth int) error {
		tokens++
		if tokens > 16384 || depth > 12 {
			return ErrPublicOwnership
		}
		t, e := d.Token()
		if e != nil {
			return ErrPublicOwnership
		}
		switch x := t.(type) {
		case json.Delim:
			if x == '{' {
				seen := make(map[string]bool)
				for d.More() {
					k, e := d.Token()
					s, ok := k.(string)
					if e != nil || !ok || seen[s] || len(seen) >= 64 {
						return ErrPublicOwnership
					}
					seen[s] = true
					if e = visit(depth + 1); e != nil {
						return e
					}
				}
				v, e := d.Token()
				if e != nil || v != json.Delim('}') {
					return ErrPublicOwnership
				}
			} else if x == '[' {
				n := 0
				for d.More() {
					n++
					if n > 1000 {
						return ErrPublicOwnership
					}
					if e = visit(depth + 1); e != nil {
						return e
					}
				}
				v, e := d.Token()
				if e != nil || v != json.Delim(']') {
					return ErrPublicOwnership
				}
			} else {
				return ErrPublicOwnership
			}
		case json.Number:
			if strings.ContainsAny(string(x), ".eE+") {
				return ErrPublicOwnership
			}
		}
		return nil
	}
	if e := visit(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrPublicOwnership
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(value) != nil {
		return ErrPublicOwnership
	}
	canonical, e := json.Marshal(value)
	if e != nil || !bytes.Equal(canonical, raw) {
		return ErrPublicOwnership
	}
	return nil
}
func ValidatePublicManifestPin(p PublicManifestPin) error {
	if _, e := validateManifestPin(p.Source); e != nil {
		return ErrPublicOwnership
	}
	if p.Version != 1 || !publicHex(p.Source.AuthorityID, 32, true) || !publicHex(p.PublicationID, 32, true) || p.AdmissionKey != PublicOwnershipFixtureKey() || p.PublicationKey != p.AdmissionKey || p.CutoffKey != p.AdmissionKey || p.CheckpointKey != PublicOwnershipCheckpointKey() || p.RuleID != PublicOwnershipRuleID() || p.Limits != publicLimits || p.ScanOrigin != (PublicChainPoint{0, p.Source.Genesis}) {
		return ErrPublicOwnership
	}
	return nil
}
func NewPublicManifestPin(source ManifestAuthorityPin, publicationID string) (PublicManifestPin, error) {
	p := PublicManifestPin{1, source, publicationID, PublicOwnershipFixtureKey(), PublicOwnershipFixtureKey(), PublicOwnershipFixtureKey(), PublicOwnershipCheckpointKey(), PublicOwnershipRuleID(), PublicChainPoint{0, source.Genesis}, publicLimits}
	return p, ValidatePublicManifestPin(p)
}
func PublicManifestPinID(p PublicManifestPin) (string, error) {
	if e := ValidatePublicManifestPin(p); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership pin", p)
}
func ParsePublicManifestPin(raw []byte) (PublicManifestPin, error) {
	var p PublicManifestPin
	if e := publicDecode(raw, MaxPublicSmallSize, &p); e != nil {
		return p, e
	}
	return p, ValidatePublicManifestPin(p)
}
func MarshalPublicManifestPin(p PublicManifestPin) ([]byte, error) {
	if e := ValidatePublicManifestPin(p); e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func publicUnsigned(value any) ([]byte, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return nil, e
	}
	i := bytes.LastIndex(raw, []byte(",\"signature\":"))
	if i < 0 {
		return nil, ErrPublicOwnership
	}
	return append(append([]byte(nil), raw[:i]...), '}'), nil
}
func publicSignature(domain string, value any, key, sig string) error {
	if !publicHex(key, 32, true) || !publicHex(sig, 64, false) {
		return ErrPublicOwnership
	}
	raw, e := publicUnsigned(value)
	if e != nil {
		return e
	}
	hash, _ := hex.DecodeString(publicHash(domain, raw))
	pub, _ := hex.DecodeString(key)
	signature, _ := hex.DecodeString(sig)
	if !ed25519.Verify(pub, hash, signature) {
		return ErrPublicOwnership
	}
	return nil
}
func publicSign(domain string, value any) (string, error) {
	raw, e := publicUnsigned(value)
	if e != nil {
		return "", e
	}
	h, _ := hex.DecodeString(publicHash(domain, raw))
	return hex.EncodeToString(ed25519.Sign(publicFixtureKey(), h)), nil
}

// ValidatePublicManifestRecordShape is cheap binding/auth/static consistency.
// It does not prove work. Public replay separately performs normal verification.
func ValidatePublicManifestRecordShape(pin ManifestAuthorityPin, r ManifestRecord) error {
	d, e := validateManifestPin(pin)
	if e != nil {
		return ErrPublicOwnership
	}
	if r.Version != 5 || r.Sequence == 0 || r.Sequence > 1000 || r.Job.AuthorityID != pin.AuthorityID || r.Job.DescriptorID != pin.DescriptorID || r.Context.Network != pin.Network || r.Context.Genesis != pin.Genesis || r.Context != r.Job.Context || r.Proof.JobID != r.Job.ID || r.Proof.GymID != r.Job.GymID || r.Authorization.PublicKey != r.Job.PublicKey || prospectjob.ValidateManifest(r.Job) != nil {
		return ErrPublicOwnership
	}
	proof, auth, e := manifestEvidence(r.Proof, r.Authorization)
	if e != nil || r.Proof.MerkleBranch == nil || !manifestSameProof(proof, r.Proof) || auth != r.Authorization || prospect.ValidateProofStructure(r.Context, r.Proof) != nil || gymauth.VerifyAuthorization(r.Context, r.Proof, r.Authorization) != nil {
		return ErrPublicOwnership
	}
	if r.Birth.RuleVersion != r.Job.RuleVersion || r.Birth.BornAtMS != r.Job.IssuedAtMS || r.Birth.AcceptedAtMS < r.Job.IssuedAtMS || r.Birth.AcceptedAtMS >= r.Job.ExpiresAtMS || r.Birth.AcceptedAtMS > MaxPublicTimestampMS || r.Job.IssuedAtMS <= 0 || r.Job.ExpiresAtMS > MaxPublicTimestampMS {
		return ErrPublicOwnership
	}
	v := r.Verified
	if v.Version != prospect.Version2 || v.Verification != "supplied-context" || v.Network != pin.Network || v.Genesis != pin.Genesis || v.GymID != r.Job.GymID || v.ParentHash != r.Context.ParentHash || v.ParentHeight != r.Context.ParentHeight || v.DNA.DNAVersion != r.Context.DNAVersion || v.DNA.Seed != v.WorkHash {
		return ErrPublicOwnership
	}
	for _, h := range []string{v.ProspectID, v.PolicyID, v.WorkHash, v.ParentHash, v.GymID} {
		if !manifestCanonicalHash(h) {
			return ErrPublicOwnership
		}
	}
	if ValidateEligibility(r.Context, v, r.Eligibility) != nil {
		return ErrPublicOwnership
	}
	s := ManifestStore{meta: manifestMetadata{DescriptorID: pin.DescriptorID, Descriptor: d}}
	binding, e := s.binding(v)
	if e != nil || binding != r.StaticBinding {
		return ErrPublicOwnership
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > MaxRecordSize {
		return ErrPublicOwnership
	}
	return nil
}
func ValidatePublicAdmission(p PublicManifestPin, a PublicAdmissionStatement) error {
	id, e := PublicManifestPinID(p)
	if e != nil || a.Version != 1 || a.PinID != id || a.AdmissionSequence == 0 || a.AdmissionSequence > p.Limits.Admissions || uint64(a.AdmissionSequence) != a.Record.Sequence || !publicPrefix(a.AdmissionSequence-1, a.PreviousAdmissionID) || ValidatePublicManifestRecordShape(p.Source, a.Record) != nil {
		return ErrPublicOwnership
	}
	rd, e := PublicOwnershipDigest("SLUG fighter ownership admission record", a.Record)
	if e != nil || rd != a.RecordDigest {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership admission statement", a, p.AdmissionKey, a.Signature)
}
func PublicAdmissionID(p PublicManifestPin, a PublicAdmissionStatement) (string, error) {
	if e := ValidatePublicAdmission(p, a); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership admission id", a)
}
func ParsePublicAdmission(p PublicManifestPin, raw []byte) (PublicAdmissionStatement, error) {
	var a PublicAdmissionStatement
	if e := publicDecode(raw, MaxPublicAdmissionSize, &a); e != nil {
		return a, e
	}
	return a, ValidatePublicAdmission(p, a)
}
func MarshalPublicAdmission(p PublicManifestPin, a PublicAdmissionStatement) ([]byte, error) {
	if e := ValidatePublicAdmission(p, a); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(a)
	if len(raw) > MaxPublicAdmissionSize {
		return nil, ErrPublicOwnership
	}
	return raw, e
}
func validatePublicPayload(p PublicManifestPin, e PublicPublicationEntry) error {
	switch e.Kind {
	case "admission":
		var v PublicAdmissionReference
		if publicDecode(e.Payload, MaxPublicSmallSize, &v) != nil || v.AdmissionSequence == 0 || v.AdmissionSequence > 1000 || !publicHex(v.AdmissionID, 32, true) {
			return ErrPublicOwnership
		}
	case "checkpoint":
		var v struct {
			Checkpoint chaincfg.SignedCheckpoint `json:"checkpoint"`
		}
		if publicDecode(e.Payload, MaxPublicSmallSize, &v) != nil {
			return ErrPublicOwnership
		}
		if er := validatePublicCheckpoint(p, v.Checkpoint); er != nil {
			return er
		}
	case "chain":
		var v PublicChainReference
		if publicDecode(e.Payload, MaxPublicSmallSize, &v) != nil || v.Height > math.MaxInt32 || !publicHex(v.Hash, 32, false) || !publicHex(v.PreviousChainEntryID, 32, false) {
			return ErrPublicOwnership
		}
	case "cutoff":
		var v PublicCutoffReference
		if publicDecode(e.Payload, MaxPublicSmallSize, &v) != nil || !publicHex(v.CutoffID, 32, true) {
			return ErrPublicOwnership
		}
	default:
		return ErrPublicOwnership
	}
	return nil
}
func validatePublicCheckpoint(p PublicManifestPin, c chaincfg.SignedCheckpoint) error {
	// Signature verification deliberately uses the separately pinned fixture role.
	pub, _ := hex.DecodeString(p.CheckpointKey)
	params := chaincfg.MainNetParams
	if uint32(c.Network) != p.Source.Network || c.GenesisHash.String() != p.Source.Genesis {
		return ErrPublicOwnership
	}
	if e := c.Verify(&params, ed25519.PublicKey(pub)); e != nil {
		return ErrPublicOwnership
	}
	return nil
}
func ValidatePublicPublication(p PublicManifestPin, e PublicPublicationEntry) error {
	id, err := PublicManifestPinID(p)
	if err != nil || e.Version != 1 || e.PinID != id || e.Sequence == 0 || e.Sequence > p.Limits.Publications || !publicPrefix(e.Sequence-1, e.PreviousID) || e.AttestedAtMS < 0 || e.AttestedAtMS > MaxPublicTimestampMS || validatePublicPayload(p, e) != nil {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership publication statement", e, p.PublicationKey, e.Signature)
}
func PublicPublicationID(p PublicManifestPin, e PublicPublicationEntry) (string, error) {
	if err := ValidatePublicPublication(p, e); err != nil {
		return "", err
	}
	return PublicOwnershipDigest("SLUG fighter ownership publication id", e)
}
func ParsePublicPublication(p PublicManifestPin, raw []byte) (PublicPublicationEntry, error) {
	var e PublicPublicationEntry
	if err := publicDecode(raw, MaxPublicSmallSize, &e); err != nil {
		return e, err
	}
	return e, ValidatePublicPublication(p, e)
}
func MarshalPublicPublication(p PublicManifestPin, e PublicPublicationEntry) ([]byte, error) {
	if err := ValidatePublicPublication(p, e); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(e)
	if len(raw) > MaxPublicSmallSize {
		return nil, ErrPublicOwnership
	}
	return raw, err
}
func ValidatePublicCutoffIntent(p PublicManifestPin, v PublicCutoffIntent) error {
	id, e := PublicManifestPinID(p)
	if e != nil || v.Version != 1 || v.PinID != id || !publicHex(v.EventID, 32, true) || v.CutoffMS <= 0 || v.CutoffMS > MaxPublicTimestampMS || v.AcceptedAdmissionSequence > 1000 || v.PublicationSequence > 16384 || !publicPrefix(v.PublicationSequence, v.PublicationHead) || v.FinalitySequence > 4096 || !publicPrefix(v.FinalitySequence, v.FinalityBodyHead) || !publicHex(v.PriorCutoffID, 32, false) {
		return ErrPublicOwnership
	}
	for _, o := range []PublicBoundaryObservation{v.First, v.Second} {
		r, e := strconv.ParseUint(o.Revision, 10, 64)
		if e != nil || r == 0 || strconv.FormatUint(r, 10) != o.Revision || !publicHex(o.InstanceID, 32, true) || !publicHex(o.Hash, 32, false) || o.Height > math.MaxInt32 || o.RequestStartMS < 0 || o.ResponseEndMS < o.RequestStartMS || o.ResponseEndMS > MaxPublicTimestampMS || o.ResponseEndMS-o.RequestStartMS > 5000 {
			return ErrPublicOwnership
		}
	}
	a, b := v.First, v.Second
	if a.InstanceID != b.InstanceID || a.Revision != b.Revision || a.Height != b.Height || a.Hash != b.Hash || a.ResponseEndMS >= v.CutoffMS || b.RequestStartMS < v.CutoffMS || b.ResponseEndMS-a.RequestStartMS > 10000 {
		return ErrPublicOwnership
	}
	return nil
}
func ValidatePublicBoundary(p PublicManifestPin, b PublicSignedBoundary) error {
	id, e := PublicManifestPinID(p)
	if e != nil || b.Version != 1 || b.PinID != id || ValidatePublicCutoffIntent(p, b.Intent) != nil {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership boundary statement", b, p.CutoffKey, b.Signature)
}
func PublicBoundaryID(p PublicManifestPin, b PublicSignedBoundary) (string, error) {
	if e := ValidatePublicBoundary(p, b); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership boundary id", b)
}
func ParsePublicBoundary(p PublicManifestPin, raw []byte) (PublicSignedBoundary, error) {
	var b PublicSignedBoundary
	if e := publicDecode(raw, MaxPublicSmallSize, &b); e != nil {
		return b, e
	}
	return b, ValidatePublicBoundary(p, b)
}
func MarshalPublicBoundary(p PublicManifestPin, b PublicSignedBoundary) ([]byte, error) {
	if e := ValidatePublicBoundary(p, b); e != nil {
		return nil, e
	}
	return json.Marshal(b)
}

func ValidatePublicOwnershipHead(p PublicManifestPin, h PublicOwnershipHead) error {
	id, e := PublicManifestPinID(p)
	if e != nil || h.Version != 1 || h.PinID != id || h.AdmissionSequence > 1000 || h.PublicationSequence > 16384 || !publicPrefix(h.AdmissionSequence, h.AdmissionHead) || !publicPrefix(h.PublicationSequence, h.PublicationHead) || h.ChainHeight > math.MaxInt32 || !publicHex(h.ChainHash, 32, true) || !publicHex(h.ReplayDigest, 32, true) || !publicHex(h.FinalizedPrefixID, 32, true) || h.ConflictID != "" && !publicHex(h.ConflictID, 32, true) {
		return ErrPublicOwnership
	}
	return nil
}
func ValidatePublicCutoff(p PublicManifestPin, c PublicCutoffRecord) error {
	id, e := PublicManifestPinID(p)
	if e != nil || c.Version != 1 || c.PinID != id || ValidatePublicBoundary(p, c.Boundary) != nil || ValidatePublicOwnershipHead(p, c.Head) != nil || !publicHex(c.OwnershipDigest, 32, true) {
		return ErrPublicOwnership
	}
	v := c.Boundary.Intent
	if c.EventID != v.EventID || c.CutoffMS != v.CutoffMS || c.AdmissionSequence != v.AcceptedAdmissionSequence || c.AdmissionSequence != c.Head.AdmissionSequence || c.AdmissionHead != c.Head.AdmissionHead || c.Head.PublicationSequence != v.PublicationSequence || c.Head.PublicationHead != v.PublicationHead || c.Head.ChainHeight != v.First.Height || c.Head.ChainHash != v.First.Hash {
		return ErrPublicOwnership
	}
	return publicSignature("SLUG fighter ownership cutoff statement", c, p.CutoffKey, c.Signature)
}
func ParsePublicCutoff(p PublicManifestPin, raw []byte) (PublicCutoffRecord, error) {
	var c PublicCutoffRecord
	if e := publicDecode(raw, MaxPublicSmallSize, &c); e != nil {
		return c, e
	}
	return c, ValidatePublicCutoff(p, c)
}
func MarshalPublicCutoff(p PublicManifestPin, c PublicCutoffRecord) ([]byte, error) {
	if e := ValidatePublicCutoff(p, c); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(c)
	if len(raw) > MaxPublicSmallSize {
		return nil, ErrPublicOwnership
	}
	return raw, e
}

func PublicCutoffID(p PublicManifestPin, v PublicCutoffRecord) (string, error) {
	if e := ValidatePublicCutoff(p, v); e != nil {
		return "", e
	}
	return PublicOwnershipDigest("SLUG fighter ownership cutoff id", v)
}
