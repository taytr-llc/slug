// Copyright (c) 2026 The SLUG developers. ISC license.
package fightertransfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/btcec/v2/ecdsa"
	"github.com/taytr-llc/slug/wire/v2"
)

const fundingDomain = "SLUG fighter transfer funding\x00v1\x00"
const authorizationDomain = "SLUG fighter transfer authorization\x00v1\x00"
const transferDomain = "SLUG fighter transfer id\x00v1\x00"

func invalid(why string) error { return fmt.Errorf("%w: %s", ErrInvalid, why) }
func publicKey(raw []byte) (*btcec.PublicKey, error) {
	if len(raw) != 33 || raw[0] != 2 && raw[0] != 3 {
		return nil, invalid("compressed public key")
	}
	p, e := btcec.ParsePubKey(raw)
	if e != nil || !bytes.Equal(p.SerializeCompressed(), raw) {
		return nil, invalid("public point")
	}
	return p, nil
}
func checkIntent(i Intent) error {
	if i.PinID == ([32]byte{}) || i.PriorStateID == ([32]byte{}) {
		return invalid("zero identifier")
	}
	_, e := publicKey(i.RecipientGym[:])
	return e
}
func MarshalIntent(i Intent) ([]byte, error) {
	if e := checkIntent(i); e != nil {
		return nil, e
	}
	raw := make([]byte, IntentSize)
	copy(raw, i.PinID[:])
	copy(raw[32:], i.PriorStateID[:])
	copy(raw[64:], i.RecipientGym[:])
	copy(raw[97:], i.FundingDigest[:])
	return raw, nil
}
func ParseIntent(raw []byte) (Intent, error) {
	var i Intent
	if len(raw) != IntentSize {
		return i, invalid("intent size")
	}
	copy(i.PinID[:], raw[:32])
	copy(i.PriorStateID[:], raw[32:64])
	copy(i.RecipientGym[:], raw[64:97])
	copy(i.FundingDigest[:], raw[97:])
	if e := checkIntent(i); e != nil {
		return Intent{}, e
	}
	return i, nil
}
func intentDigest(domain string, i Intent) ([32]byte, error) {
	raw, e := MarshalIntent(i)
	if e != nil {
		return [32]byte{}, e
	}
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write(raw)
	var d [32]byte
	copy(d[:], h.Sum(nil))
	return d, nil
}
func AuthorizationDigest(i Intent) ([32]byte, error) { return intentDigest(authorizationDomain, i) }
func TransferID(i Intent) ([32]byte, error)          { return intentDigest(transferDomain, i) }
func FundingDigest(ops []wire.OutPoint) ([32]byte, error) {
	if len(ops) < 1 || len(ops) > MaxInputs {
		return [32]byte{}, invalid("funding count")
	}
	h := sha256.New()
	h.Write([]byte(fundingDomain))
	h.Write([]byte{byte(len(ops))})
	var previous [36]byte
	for n, op := range ops {
		var raw [36]byte
		copy(raw[:32], op.Hash[:])
		binary.LittleEndian.PutUint32(raw[32:], op.Index)
		if n > 0 && bytes.Compare(previous[:], raw[:]) >= 0 {
			return [32]byte{}, invalid("funding order or duplicate")
		}
		h.Write(raw[:])
		previous = raw
	}
	var d [32]byte
	copy(d[:], h.Sum(nil))
	return d, nil
}
func compactSignature(raw [64]byte) (*ecdsa.Signature, error) {
	var r, s btcec.ModNScalar
	if r.SetByteSlice(raw[:32]) || s.SetByteSlice(raw[32:]) || r.IsZero() || s.IsZero() || s.IsOverHalfOrder() {
		return nil, invalid("compact signature scalars")
	}
	return ecdsa.NewSignature(&r, &s), nil
}
func MarshalEnvelope(e Envelope) ([]byte, error) {
	i, err := MarshalIntent(e.Intent)
	if err != nil {
		return nil, err
	}
	if _, err = compactSignature(e.Signature); err != nil {
		return nil, err
	}
	raw := make([]byte, EnvelopeSize)
	copy(raw, []byte("SLUGFTR1\x01"))
	copy(raw[9:], i)
	copy(raw[138:], e.Signature[:])
	return raw, nil
}
func ParseEnvelope(raw []byte) (Envelope, error) {
	var e Envelope
	if len(raw) != EnvelopeSize || !bytes.Equal(raw[:9], []byte("SLUGFTR1\x01")) {
		return e, invalid("envelope framing")
	}
	i, err := ParseIntent(raw[9:138])
	if err != nil {
		return e, err
	}
	e.Intent = i
	copy(e.Signature[:], raw[138:])
	if _, err = compactSignature(e.Signature); err != nil {
		return Envelope{}, err
	}
	return e, nil
}
func VerifyAuthorization(expected Expected, e Envelope) error {
	if expected.PinID != e.Intent.PinID || expected.PriorStateID != e.Intent.PriorStateID || expected.SenderGym == e.Intent.RecipientGym {
		return invalid("source, predecessor or self transfer")
	}
	pub, err := publicKey(expected.SenderGym[:])
	if err != nil {
		return err
	}
	d, err := AuthorizationDigest(e.Intent)
	if err != nil {
		return err
	}
	sig, err := compactSignature(e.Signature)
	if err != nil {
		return err
	}
	if !sig.Verify(d[:], pub) {
		return invalid("gym signature")
	}
	return nil
}
func RedeemScript(e Envelope, fundingKey [33]byte) ([]byte, error) {
	if _, err := publicKey(fundingKey[:]); err != nil {
		return nil, err
	}
	raw, err := MarshalEnvelope(e)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, RedeemSize)
	result = append(result, 0x4c, 0xca)
	result = append(result, raw...)
	result = append(result, 0x75, 0x21)
	result = append(result, fundingKey[:]...)
	result = append(result, 0xac)
	return result, nil
}
func ParseRedeem(raw []byte) (Envelope, [33]byte, error) {
	var key [33]byte
	if len(raw) != RedeemSize || raw[0] != 0x4c || raw[1] != 0xca || raw[204] != 0x75 || raw[205] != 0x21 || raw[239] != 0xac {
		return Envelope{}, key, invalid("redeem template")
	}
	e, err := ParseEnvelope(raw[2:204])
	if err != nil {
		return Envelope{}, key, err
	}
	copy(key[:], raw[206:239])
	if _, err = publicKey(key[:]); err != nil {
		return Envelope{}, [33]byte{}, err
	}
	return e, key, nil
}
func CommitLocator(i Intent) ([]byte, error) {
	if e := checkIntent(i); e != nil {
		return nil, e
	}
	raw := append([]byte{0x6a, 0x4a}, []byte("SLUGFCM1\x01")...)
	raw = append(raw, i.PriorStateID[:]...)
	raw = append(raw, i.RecipientGym[:]...)
	return raw, nil
}
func RevealLocator(i Intent) ([]byte, error) {
	id, e := TransferID(i)
	if e != nil {
		return nil, e
	}
	raw := append([]byte{0x6a, 0x29}, []byte("SLUGFRV1\x01")...)
	return append(raw, id[:]...), nil
}
