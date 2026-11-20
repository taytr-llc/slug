// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/seednode"
)

func rawID(s string) []byte { b, _ := hex.DecodeString(s); return b }
func le32(n uint32) []byte  { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, n); return b }
func lp(b []byte) []byte    { return append(le32(uint32(len(b))), b...) }
func hash(d string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(d))
	for _, b := range parts {
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func fixtureKey(checkpoint bool) ed25519.PrivateKey {
	d := "SLUG candidate result statement fixture\x00v1\x00"
	if checkpoint {
		d = "SLUG candidate result checkpoint fixture\x00v1\x00"
	}
	s := sha256.Sum256([]byte(d))
	return ed25519.NewKeyFromSeed(s[:])
}
func RulesID() string {
	return hash("SLUG candidate result rules\x00v1\x00", lp([]byte(`{"version":1,"source":"saved_c1_bout_v1","carrier":"SLUGRSLT-v1","fund":"fixture_key2_p2wpkh","inclusion":"exact_local_tx","pre_checkpoint_reorg":"same_tx_reinclude","finality":"carrier_checkpoint_60","max_step_blocks":8,"max_scan_restarts":16,"max_prior_inclusions":16,"fee_satoshis":1000,"awards":"none"}`)))
}
func CandidatePin(source gymbout.Pin) (Pin, error) {
	source, e := gymbout.ValidatePin(source)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	chain, e := seednode.ChainIdentityFor(source.SourcePin.SourcePin.SourcePin)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	zero, _ := gymaccount.FixtureIdentity("zero256")
	id, e := gymbout.ID(source)
	if e != nil {
		return Pin{}, ErrInvalid
	}
	pub := rawID("02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5")
	return Pin{Version: 1, SourceStoreID: id, SourcePin: source, PublisherGymID: zero.GymID, PublisherAccountID: chain.PublisherAccountID, PublisherSourceAuthorityID: chain.PublisherSourceAuthorityID, PublisherChainID: chain.PublisherChainID, Network: chain.Network, Genesis: chain.Genesis, FloorHeight: 0, FloorHash: chain.Genesis, PublisherPublicKey: hex.EncodeToString(fixtureKey(false).Public().(ed25519.PublicKey)), CheckpointPublicKey: hex.EncodeToString(fixtureKey(true).Public().(ed25519.PublicKey)), FundingPublicKey: hex.EncodeToString(pub), FundingScript: hex.EncodeToString(append([]byte{0, 20}, address.Hash160(pub)...)), RulesID: RulesID()}, nil
}
func ID(p Pin) (string, error) {
	b, e := MarshalPin(p)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate result store\x00v1\x00", lp(b)), nil
}
func resultKey(p Pin, card, bout string) string {
	return hash("SLUG candidate result key\x00v1\x00", le32(p.Network), rawID(p.Genesis), rawID(p.PublisherChainID), rawID(p.SourceStoreID), rawID(card), rawID(bout), rawID(p.PublisherPublicKey))
}
func ResultKey(p Pin, card, bout string) (string, error) {
	if validatePin(p) != nil || !hashID(card) || !hashID(bout) {
		return "", ErrInvalid
	}
	return resultKey(p, card, bout), nil
}
func StatementID(v Statement) (string, error) {
	var e error
	v, e = owned(v, MaxStatementSize, structuralStatement)
	if e != nil {
		return "", e
	}
	b, _ := json.Marshal(v)
	return hash("SLUG candidate result statement id\x00v1\x00", lp(b)), nil
}
func statementMessage(v Statement) []byte {
	b, _ := json.Marshal(v)
	return append([]byte("SLUG candidate result statement\x00v1\x00"), lp(b)...)
}
func Carrier(v Statement) ([73]byte, error) {
	var b [73]byte
	id, e := StatementID(v)
	if e != nil {
		return b, e
	}
	copy(b[:8], "SLUGRSLT")
	b[8] = 1
	copy(b[9:41], rawID(v.ResultKey))
	copy(b[41:], rawID(id))
	return b, nil
}
func recordDigest(v Record) string {
	b, _ := json.Marshal(v)
	return hash("SLUG candidate result record\x00v1\x00", lp(b))
}
func RecordDigest(p Pin, v Record) (string, error) {
	v, e := ValidateRecord(p, v)
	if e != nil {
		return "", e
	}
	return recordDigest(v), nil
}
func initialCursor(p Pin, key string, generation uint32) Cursor {
	id, _ := ID(p)
	d := "SLUG candidate result funding scan\x00v1\x00"
	parts := [][]byte{rawID(id), le32(generation), le32(0), rawID(p.Genesis)}
	if key != "" {
		d = "SLUG candidate result inclusion scan\x00v1\x00"
		parts = [][]byte{rawID(id), rawID(key), le32(generation), le32(0), rawID(p.Genesis)}
	}
	return Cursor{Generation: generation, Height: -1, ScanDigest: hash(d, parts...), CompleteToHeight: -1}
}
func stepDigest(previous string, height uint32, h string) string {
	return hash("SLUG candidate result scan step\x00v1\x00", rawID(previous), le32(height), rawID(h))
}
func expectedStatement(p Pin, source gymbout.ResultSource) Statement {
	id, _ := ID(p)
	return Statement{Version: 1, ResultKey: resultKey(p, source.Source.CardID, source.Source.Bout.BoutID), StoreID: id, SourceStoreID: p.SourceStoreID, Source: source, PublisherChainID: p.PublisherChainID, Network: p.Network, Genesis: p.Genesis, RulesID: p.RulesID, PublisherPublicKey: p.PublisherPublicKey, CheckpointPublicKey: p.CheckpointPublicKey}
}
