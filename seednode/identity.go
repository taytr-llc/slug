// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
)

// ChainIdentityFor derives a source-bound identity, never an RPC URL or tip.
func ChainIdentityFor(source gymevent.Pin) (ChainIdentity, error) {
	pin, err := gymevent.ValidatePin(source)
	if err != nil {
		return ChainIdentity{}, ErrInvalid
	}
	id, err := gymevent.ID(pin)
	if err != nil {
		return ChainIdentity{}, ErrInvalid
	}
	zero, _ := gymaccount.FixtureIdentity("zero256")
	for _, entry := range pin.Participants {
		p := entry.StablePin
		if p.Identity != zero {
			continue
		}
		r := ChainIdentity{SourceStoreID: id, PublisherAccountID: p.AccountID,
			PublisherSourceAuthorityID: p.Source.AuthorityID,
			Network:                    p.Source.Network, Genesis: p.Source.Genesis}
		h := sha256.New()
		h.Write([]byte("SLUG candidate publication chain\x00v1\x00"))
		for _, value := range []string{id, p.AccountID, p.Source.AuthorityID, p.Source.Genesis} {
			raw, e := hex.DecodeString(value)
			if e != nil || len(raw) != 32 {
				return ChainIdentity{}, ErrInvalid
			}
			h.Write(raw)
		}
		r.PublisherChainID = hex.EncodeToString(h.Sum(nil))
		return r, nil
	}
	return ChainIdentity{}, ErrInvalid
}
