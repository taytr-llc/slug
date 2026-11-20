// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/seednode"
)

func CandidateOwnedPin(source gymevent.OwnedPin) (OwnedPin, error) {
	source, e := gymevent.ValidateOwnedPin(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	id, e := seednode.OwnershipChainIdentityFor(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	zero, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	one := make([]byte, 32)
	one[31] = 1
	_, pub := btcec.PrivKeyFromBytes(one)
	compressed := pub.SerializeCompressed()
	script := append([]byte{0, 20}, address.Hash160(compressed)...)
	return OwnedPin{1, id.SourceStoreID, source, zero.GymID, id.PublisherAccountID, id.PublisherSourceAuthorityID, id.PublisherChainID, id.Network, id.Genesis, 0, id.Genesis, hex.EncodeToString(fixtureKey(false).Public().(ed25519.PublicKey)), hex.EncodeToString(fixtureKey(true).Public().(ed25519.PublicKey)), hex.EncodeToString(compressed), hex.EncodeToString(script), RulesID()}, nil
}

func validateOwnedPin(p OwnedPin) error {
	want, e := CandidateOwnedPin(p.SourcePin)
	if e != nil || !equal(p, want) {
		return ErrInvalid
	}
	return nil
}
func OwnedID(p OwnedPin) (string, error) {
	if validateOwnedPin(p) != nil {
		return "", ErrInvalid
	}
	return ownedcodec.Hash("seed store", p)
}
func ownedEventKey(p OwnedPin, ev gymevent.Event) string {
	id, _ := OwnedID(p)
	v, _ := ownedcodec.Hash("seed event", struct {
		Version uint32 `json:"version"`
		StoreID string `json:"store_id"`
		EventID string `json:"event_id"`
	}{1, id, ev.EventID})
	return v
}
func OwnedEventKey(p OwnedPin, ev gymevent.Event) (string, error) {
	if validateOwnedPin(p) != nil {
		return "", ErrInvalid
	}
	if _, e := gymevent.ValidateOwnedEvent(p.SourcePin, ev); e != nil {
		return "", ErrInvalid
	}
	return ownedEventKey(p, ev), nil
}
func structuralOwnedDeclaration(v OwnedDeclaration) error {
	return structuralDeclaration(Declaration(v))
}
func OwnedDeclarationID(v OwnedDeclaration) (string, error) {
	if structuralOwnedDeclaration(v) != nil {
		return "", ErrInvalid
	}
	return ownedcodec.Hash("seed declaration", v)
}
func ownedDeclarationMessage(v OwnedDeclaration) []byte {
	id, _ := ownedcodec.Hash("seed declaration signature", v)
	return rawID(id)
}
func expectedOwnedDeclaration(p OwnedPin, r gymevent.OwnedSnapshotReceipt) (OwnedDeclaration, error) {
	rd, e := gymevent.OwnedReceiptDigest(p.SourcePin, r)
	if e != nil {
		return OwnedDeclaration{}, ErrInvalid
	}
	id, e := OwnedID(p)
	if e != nil {
		return OwnedDeclaration{}, e
	}
	return OwnedDeclaration{1, ownedEventKey(p, r.Input.Event), id, p.SourceStoreID, r.Input.Event, r.SnapshotDigest, rd, p.PublisherChainID, p.Network, p.Genesis, 0, p.Genesis, p.RulesID, p.PublisherPublicKey, p.CheckpointPublicKey}, nil
}
func ownedDeclarationFor(p OwnedPin, r gymevent.OwnedSnapshotReceipt) (OwnedSignedDeclaration, error) {
	v, e := expectedOwnedDeclaration(p, r)
	if e != nil {
		return OwnedSignedDeclaration{}, e
	}
	return OwnedSignedDeclaration{1, v, hex.EncodeToString(ed25519.Sign(fixtureKey(false), ownedDeclarationMessage(v)))}, nil
}
func ownedCarrier(v OwnedDeclaration) ([73]byte, error) {
	var b [73]byte
	id, e := OwnedDeclarationID(v)
	if e != nil {
		return b, e
	}
	copy(b[:8], "SLUGSEED")
	b[8] = 1
	copy(b[9:41], rawID(v.EventKey))
	copy(b[41:], rawID(id))
	return b, nil
}
func OwnedRecordDigest(p OwnedPin, r OwnedRecord) (string, error) {
	if _, e := ValidateOwnedRecord(p, r); e != nil {
		return "", e
	}
	return ownedcodec.Hash("seed record", r)
}
func initialOwnedCursor(p OwnedPin, event string, generation uint32) Cursor {
	id, _ := OwnedID(p)
	domain := "SLUG candidate funding scan\x00v1\x00"
	parts := [][]byte{rawID(id), le32(generation), le32(0), rawID(p.Genesis)}
	if event != "" {
		domain = "SLUG candidate seed scan\x00v1\x00"
		parts = [][]byte{rawID(id), rawID(event), le32(generation), le32(0), rawID(p.Genesis)}
	}
	return Cursor{Generation: generation, Height: -1, ScanDigest: hash(domain, parts...), CompleteToHeight: -1}
}

// Only private physical validators receive this projection. It has no event
// source or account identity and is never accepted by an old Pin/Store API.
// Funding scripts, transaction rules, headers and checkpoint arithmetic remain
// the exact inherited engine leaves.
func physicalOwnedPin(p OwnedPin) Pin {
	return Pin{Network: p.Network, Genesis: p.Genesis, FundingPublicKey: p.FundingPublicKey, FundingScript: p.FundingScript, CheckpointPublicKey: p.CheckpointPublicKey}
}

func absentOwnedStatus(p OwnedPin, ev gymevent.Event, n int64) OwnedStatus {
	id, _ := OwnedID(p)
	return OwnedStatus{Version: 1, StoreID: id, EventKey: ownedEventKey(p, ev), Event: ev, Phase: "absent", PublicationState: "none", ScanHeight: -1, ScanTipHeight: -1, ObservedAtMS: n, NodeState: "unobserved", FundingState: "unobserved", EscalationDue: n >= ev.IncidentDeadlineMS}
}

func ownedValue[T any](v T, limit int, validate func(T) error) (T, error) {
	var zero T
	if validate(v) != nil {
		return zero, ErrInvalid
	}
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > limit {
		return zero, ErrInvalid
	}
	if e = json.Unmarshal(raw, &zero); e != nil {
		return zero, ErrInvalid
	}
	return zero, nil
}

func ownedDecode(raw []byte, limit int, dst any, _ bool) error {
	if e := ownedcodec.Decode(raw, limit, dst); e != nil {
		return ErrInvalid
	}
	return nil
}

func ownedTimestamp(n int64) bool { return n > 0 && n <= 253402300799999 }

func ownedCardProspectsUnique(receipt gymevent.OwnedSnapshotReceipt) bool {
	seen := make(map[string]bool, 10)
	for _, p := range receipt.Input.Participants {
		for _, m := range p.Members {
			if seen[m.Birth.ProspectID] {
				return false
			}
			seen[m.Birth.ProspectID] = true
		}
	}
	return true
}
