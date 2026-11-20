// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/seednode"
)

func CandidateOwnedPin(source gymbout.OwnedPin) (OwnedPin, error) {
	source, e := gymbout.ValidateOwnedPin(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	chain, e := seednode.OwnershipChainIdentityFor(source.SourcePin.SourcePin.SourcePin)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	zero, _ := gymaccount.FixtureIdentity("zero256")
	id, e := gymbout.OwnedID(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	pub := rawID("02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5")
	return OwnedPin{Version: 1, SourceStoreID: id, SourcePin: source, PublisherGymID: zero.GymID, PublisherAccountID: chain.PublisherAccountID, PublisherSourceAuthorityID: chain.PublisherSourceAuthorityID, PublisherChainID: chain.PublisherChainID, Network: chain.Network, Genesis: chain.Genesis, FloorHeight: 0, FloorHash: chain.Genesis, PublisherPublicKey: hex.EncodeToString(fixtureKey(false).Public().(ed25519.PublicKey)), CheckpointPublicKey: hex.EncodeToString(fixtureKey(true).Public().(ed25519.PublicKey)), FundingPublicKey: hex.EncodeToString(pub), FundingScript: hex.EncodeToString(append([]byte{0, 20}, address.Hash160(pub)...)), RulesID: RulesID()}, nil
}
func validateOwnedPin(p OwnedPin) error {
	v, e := CandidateOwnedPin(p.SourcePin)
	if e != nil || !equal(v, p) {
		return ErrInvalid
	}
	return nil
}
func OwnedID(p OwnedPin) (string, error) {
	if validateOwnedPin(p) != nil {
		return "", ErrInvalid
	}
	return ownedcodec.Hash("result store", p)
}
func ownedResultKey(p OwnedPin, card, bout string) string {
	id, _ := OwnedID(p)
	out, _ := ownedcodec.Hash("result key", struct {
		Version uint32 `json:"version"`
		StoreID string `json:"store_id"`
		BoutID  string `json:"bout_id"`
	}{1, id, bout})
	return out
}
func OwnedResultKey(p OwnedPin, card, bout string) (string, error) {
	if validateOwnedPin(p) != nil || !hashID(card) || !hashID(bout) {
		return "", ErrInvalid
	}
	return ownedResultKey(p, card, bout), nil
}
func OwnedStatementID(v OwnedStatement) (string, error) {
	if ownedStructuralStatement(v) != nil {
		return "", ErrInvalid
	}
	return ownedcodec.Hash("result statement", v)
}
func ownedStatementMessage(v OwnedStatement) []byte {
	id, _ := ownedcodec.Hash("result statement signature", v)
	return rawID(id)
}
func OwnedCarrier(v OwnedStatement) ([73]byte, error) {
	var out [73]byte
	id, e := OwnedStatementID(v)
	if e != nil {
		return out, e
	}
	copy(out[:8], "SLUGRSLT")
	out[8] = 1
	copy(out[9:41], rawID(v.ResultKey))
	copy(out[41:], rawID(id))
	return out, nil
}
func ownedRecordDigest(v OwnedRecord) string { id, _ := ownedcodec.Hash("result record", v); return id }
func OwnedRecordDigest(p OwnedPin, v OwnedRecord) (string, error) {
	if _, e := ValidateOwnedRecord(p, v); e != nil {
		return "", e
	}
	return ownedRecordDigest(v), nil
}
func physicalOwnedPin(p OwnedPin) Pin {
	return Pin{Network: p.Network, Genesis: p.Genesis, FundingPublicKey: p.FundingPublicKey, FundingScript: p.FundingScript, CheckpointPublicKey: p.CheckpointPublicKey}
}
func ownedValue[T any](v T, limit int, validate func(T) error) (T, error) {
	var z T
	if e := validate(v); e != nil {
		return z, e
	}
	b, e := json.Marshal(v)
	if e != nil || len(b) > limit {
		return z, ErrInvalid
	}
	if json.Unmarshal(b, &z) != nil {
		return z, ErrInvalid
	}
	return z, nil
}
func ownedDecode(raw []byte, limit int, out any, _ bool) error {
	if ownedcodec.Decode(raw, limit, out) != nil {
		return ErrInvalid
	}
	return nil
}
func ownedTimestamp(v int64) bool { return v > 0 && v <= 253402300799999 }

func ownedInitialCursor(p OwnedPin, key string, generation uint32) Cursor {
	id, _ := OwnedID(p)
	d := "SLUG candidate result funding scan\x00v1\x00"
	parts := [][]byte{rawID(id), le32(generation), le32(0), rawID(p.Genesis)}
	if key != "" {
		d = "SLUG candidate result inclusion scan\x00v1\x00"
		parts = [][]byte{rawID(id), rawID(key), le32(generation), le32(0), rawID(p.Genesis)}
	}
	return Cursor{Generation: generation, Height: -1, ScanDigest: hash(d, parts...), CompleteToHeight: -1}
}
func ownedExpectedStatement(p OwnedPin, source gymbout.OwnedResultSource) OwnedStatement {
	id, _ := OwnedID(p)
	return OwnedStatement{Version: 1, ResultKey: ownedResultKey(p, source.Source.CardID, source.Source.Bout.BoutID), StoreID: id, SourceStoreID: p.SourceStoreID, Source: source, PublisherChainID: p.PublisherChainID, Network: p.Network, Genesis: p.Genesis, RulesID: p.RulesID, PublisherPublicKey: p.PublisherPublicKey, CheckpointPublicKey: p.CheckpointPublicKey}
}
