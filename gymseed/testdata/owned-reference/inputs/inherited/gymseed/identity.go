// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/seednode"
)

func rawID(s string) []byte { b, _ := hex.DecodeString(s); return b }
func le32(n uint32) []byte  { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, n); return b }
func lp(b []byte) []byte    { return append(le32(uint32(len(b))), b...) }
func hash(domain string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func fixtureKey(checkpoint bool) ed25519.PrivateKey {
	domain := "SLUG candidate seed declaration fixture\x00v1\x00"
	if checkpoint {
		domain = "SLUG candidate seed checkpoint fixture\x00v1\x00"
	}
	seed := sha256.Sum256([]byte(domain))
	return ed25519.NewKeyFromSeed(seed[:])
}
func RulesID() string {
	return hash("SLUG candidate seed rules\x00v1\x00", lp([]byte(`{"version":1,"target":"first_anchor_next_block","floor":"genesis_inclusive","carrier":"SLUGSEED-v1","finality":"exact_target_checkpoint_60","max_step_blocks":8,"max_scan_restarts":16,"fee_satoshis":1000}`)))
}
func CandidatePin(source gymevent.Pin) (Pin, error) {
	source, e := gymevent.ValidatePin(source)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	identity, e := seednode.ChainIdentityFor(source)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	zero, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		return Pin{}, ErrInvalid
	}
	one := make([]byte, 32)
	one[31] = 1
	_, pub := btcec.PrivKeyFromBytes(one)
	compressed := pub.SerializeCompressed()
	script := append([]byte{0, 20}, address.Hash160(compressed)...)
	return Pin{Version: 1, SourceStoreID: identity.SourceStoreID, SourcePin: source, PublisherGymID: zero.GymID, PublisherAccountID: identity.PublisherAccountID, PublisherSourceAuthorityID: identity.PublisherSourceAuthorityID, PublisherChainID: identity.PublisherChainID, Network: identity.Network, Genesis: identity.Genesis, FloorHeight: 0, FloorHash: identity.Genesis, PublisherPublicKey: hex.EncodeToString(fixtureKey(false).Public().(ed25519.PublicKey)), CheckpointPublicKey: hex.EncodeToString(fixtureKey(true).Public().(ed25519.PublicKey)), FundingPublicKey: hex.EncodeToString(compressed), FundingScript: hex.EncodeToString(script), RulesID: RulesID()}, nil
}
func ID(p Pin) (string, error) {
	b, e := MarshalPin(p)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate seed store\x00v1\x00", lp(b)), nil
}
func EventKey(p Pin, ev gymevent.Event) (string, error) {
	if validatePin(p) != nil {
		return "", ErrInvalid
	}
	if _, e := gymevent.ValidateEvent(p.SourcePin, ev); e != nil {
		return "", ErrInvalid
	}
	return eventKey(p, ev), nil
}
func eventKey(p Pin, ev gymevent.Event) string {
	return hash("SLUG candidate seed event\x00v1\x00", le32(p.Network), rawID(p.Genesis), rawID(p.SourceStoreID), rawID(ev.EventID), rawID(p.PublisherPublicKey))
}
func DeclarationID(v Declaration) (string, error) {
	if structuralDeclaration(v) != nil {
		return "", ErrInvalid
	}
	b, e := json.Marshal(v)
	if e != nil {
		return "", ErrInvalid
	}
	return hash("SLUG candidate seed declaration id\x00v1\x00", lp(b)), nil
}
func declarationMessage(v Declaration) []byte {
	b, _ := json.Marshal(v)
	return append([]byte("SLUG candidate seed declaration\x00v1\x00"), lp(b)...)
}
func expectedDeclaration(p Pin, r gymevent.SnapshotReceipt) (Declaration, error) {
	rd, e := gymevent.ReceiptDigest(p.SourcePin, r)
	if e != nil {
		return Declaration{}, ErrInvalid
	}
	id, e := ID(p)
	if e != nil {
		return Declaration{}, e
	}
	v := Declaration{Version: 1, EventKey: eventKey(p, r.Input.Event), StoreID: id, SourceStoreID: p.SourceStoreID, Event: r.Input.Event, SnapshotDigest: r.SnapshotDigest, ReceiptDigest: rd, PublisherChainID: p.PublisherChainID, Network: p.Network, Genesis: p.Genesis, FloorHeight: 0, FloorHash: p.Genesis, RulesID: p.RulesID, PublisherPublicKey: p.PublisherPublicKey, CheckpointPublicKey: p.CheckpointPublicKey}
	return v, nil
}
func declarationFor(p Pin, r gymevent.SnapshotReceipt) (SignedDeclaration, error) {
	v, e := expectedDeclaration(p, r)
	if e != nil {
		return SignedDeclaration{}, e
	}
	return SignedDeclaration{Version: 1, Declaration: v, Signature: hex.EncodeToString(ed25519.Sign(fixtureKey(false), declarationMessage(v)))}, nil
}

func carrier(v Declaration) ([73]byte, error) {
	var b [73]byte
	id, e := DeclarationID(v)
	if e != nil {
		return b, e
	}
	copy(b[:8], "SLUGSEED")
	b[8] = 1
	copy(b[9:41], rawID(v.EventKey))
	copy(b[41:], rawID(id))
	return b, nil
}
func RecordDigest(p Pin, r Record) (string, error) {
	b, e := MarshalRecord(p, r)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate seed record\x00v1\x00", lp(b)), nil
}
func initialCursor(p Pin, event string, generation uint32) Cursor {
	id, _ := ID(p)
	domain := "SLUG candidate funding scan\x00v1\x00"
	parts := [][]byte{rawID(id), le32(generation), le32(0), rawID(p.Genesis)}
	if event != "" {
		domain = "SLUG candidate seed scan\x00v1\x00"
		parts = [][]byte{rawID(id), rawID(event), le32(generation), le32(0), rawID(p.Genesis)}
	}
	return Cursor{Generation: generation, Height: -1, ScanDigest: hash(domain, parts...), CompleteToHeight: -1}
}
