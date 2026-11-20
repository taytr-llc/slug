// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// OwnershipPoolOwnerBinding is independently retained by the concrete runtime.
// Marker hashes corroborate that runtime; reading a file never establishes the
// expected public source, ownership store or account authority.
type OwnershipPoolOwnerBinding struct {
	Source              gymevent.OwnedPin
	OwnershipPin        fighterownership.Pin
	OwnershipStoreID    string
	EventStoreID        string
	EventCreatedAtMS    int64
	Pool                poolshare.Config
	PoolBindingID       string
	Accounts            *gymaccount.Store
	Issuer              *registry.ManifestStore
	Ownership           *fighterownership.Store
	Events              *gymevent.OwnedStore
	Home                string
	Primary, Peer       OwnedProcess
	Life                context.Context
	RuntimeMarkerSHA256 string
	LeagueIntentSHA256  string
	LeagueReadySHA256   string
}

func OwnershipChainIdentityFor(source gymevent.OwnedPin) (ChainIdentity, error) {
	p, e := gymevent.ValidateOwnedPin(source)
	if e != nil {
		return ChainIdentity{}, ErrInvalid
	}
	id, e := gymevent.OwnedID(p)
	if e != nil {
		return ChainIdentity{}, ErrInvalid
	}
	zero, _ := gymaccount.FixtureIdentity("zero256")
	for _, entry := range p.Participants {
		s := entry.StablePin.Stable
		if s.Identity != zero {
			continue
		}
		v := ChainIdentity{SourceStoreID: id, PublisherAccountID: s.AccountID, PublisherSourceAuthorityID: s.Source.AuthorityID, Network: s.Source.Network, Genesis: s.Source.Genesis}
		h := sha256.New()
		h.Write([]byte("SLUG candidate publication chain\x00v1\x00"))
		for _, value := range []string{id, s.AccountID, s.Source.AuthorityID, s.Source.Genesis} {
			raw, e := hex.DecodeString(value)
			if e != nil || len(raw) != 32 {
				return ChainIdentity{}, ErrInvalid
			}
			h.Write(raw)
		}
		v.PublisherChainID = hex.EncodeToString(h.Sum(nil))
		return v, nil
	}
	return ChainIdentity{}, ErrInvalid
}

func validateOwnershipPoolBinding(b OwnershipPoolOwnerBinding) (OwnershipPoolOwnerBinding, error) {
	if b.Accounts == nil || b.Issuer == nil || b.Ownership == nil || b.Events == nil || b.Life == nil || b.Life.Err() != nil || b.EventCreatedAtMS <= 0 || b.EventCreatedAtMS > gymevent.MaxTimestampMS {
		return b, ErrInvalid
	}
	for _, id := range []string{b.OwnershipStoreID, b.EventStoreID, b.PoolBindingID, b.Accounts.AuthorityID(), b.RuntimeMarkerSHA256, b.LeagueIntentSHA256, b.LeagueReadySHA256} {
		if !hexValue(id, 32, true) {
			return b, ErrInvalid
		}
	}
	var e error
	b.Source, e = gymevent.ValidateOwnedPin(b.Source)
	if e != nil {
		return b, ErrInvalid
	}
	if _, e = fighterownership.ValidatePin(b.OwnershipPin); e != nil || b.Source.Ownership != b.OwnershipPin || b.Ownership.Pin() != b.OwnershipPin || b.Ownership.Identity().StoreID != b.OwnershipStoreID {
		return b, ErrInvalid
	}
	id, e := gymevent.OwnedID(b.Source)
	if e != nil || id != b.EventStoreID {
		return b, ErrInvalid
	}
	id, e = poolservice.BindingID(b.Pool)
	if e != nil || id != b.PoolBindingID || b.OwnershipPin.Source != b.Pool.AuthorityPin || b.Pool.PayoutAddress != poolsettlefixture.PoolAddress() || b.Pool.AuthorityPin.Network != gymaccount.FixtureNetwork || b.Pool.AuthorityPin.Genesis != gymaccount.FixtureGenesis {
		return b, ErrInvalid
	}
	zero, _ := gymaccount.FixtureIdentity("zero256")
	other, _ := gymaccount.FixtureIdentity("incrementing256")
	seen := map[gymaccount.Identity]bool{}
	for _, entry := range b.Source.Participants {
		p := entry.StablePin.Stable
		if p.Version != gymstable.SharedPinVersion2 || p.AccountAuthorityID != b.Accounts.AuthorityID() || p.Source != b.Pool.AuthorityPin || (p.Identity != zero && p.Identity != other) || seen[p.Identity] {
			return b, ErrInvalid
		}
		seen[p.Identity] = true
	}
	if len(seen) != 2 {
		return b, ErrInvalid
	}
	return b, nil
}

func (p *Publisher) checkOwnershipOwner(ctx context.Context) error {
	b := p.ownership
	if b == nil || ctx.Err() != nil || b.Life.Err() != nil {
		return ErrOffline
	}
	info, e := awaitSharedIssuer(ctx, b.Life, b.Issuer.TryInfo)
	if e != nil || info.ReadOnly || info.Pin != b.Pool.AuthorityPin {
		return ErrUnavailable
	}
	if b.Ownership.Pin() != b.OwnershipPin || b.Ownership.Identity().StoreID != b.OwnershipStoreID {
		return ErrUnavailable
	}
	if _, e = b.Ownership.Head(ctx); e != nil {
		return ErrUnavailable
	}
	ev, e := b.Events.Info(ctx)
	if e != nil || ev.StoreID != b.EventStoreID || ev.CreatedAtMS != b.EventCreatedAtMS || !reflect.DeepEqual(ev.Pin, b.Source) {
		return ErrUnavailable
	}
	if b.Accounts.AuthorityID() != b.Source.Participants[0].StablePin.Stable.AccountAuthorityID {
		return ErrOffline
	}
	for _, m := range []struct{ path, digest string }{{"SLUG-POOL-RUNTIME.json", b.RuntimeMarkerSHA256}, {"owned-league/SLUG-OWNED-LEAGUE-INTENT.json", b.LeagueIntentSHA256}, {"owned-league/SLUG-OWNED-LEAGUE-READY.json", b.LeagueReadySHA256}} {
		if e = ownershipMarker(filepath.Join(b.Home, m.path), m.digest); e != nil {
			return e
		}
	}
	raw, e := readRegular(filepath.Join(b.Home, "SLUG-POOL-SOURCE.json"), sharedMarkerLimit)
	want, me := json.Marshal(b.Pool)
	if e != nil || me != nil || !bytes.Equal(raw, append(want, '\n')) {
		return ErrInvalid
	}
	if ctx.Err() != nil || b.Life.Err() != nil {
		return ErrOffline
	}
	return nil
}

func ownershipMarker(path, expected string) error {
	raw, e := readRegular(path, 64*1024)
	if e != nil || len(raw) < 2 || raw[len(raw)-1] != '\n' {
		return ErrInvalid
	}
	digest := sha256.Sum256(raw[:len(raw)-1])
	if hex.EncodeToString(digest[:]) != expected {
		return ErrInvalid
	}
	return nil
}

func AttachOwnershipPool(ctx context.Context, b OwnershipPoolOwnerBinding) (*Publisher, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrInvalid
	}
	b, e := validateOwnershipPoolBinding(b)
	if e != nil {
		return nil, e
	}
	if !live(b.Primary) || !live(b.Peer) || b.Primary.Cmd.Process.Pid == b.Peer.Cmd.Process.Pid {
		return nil, ErrOffline
	}
	id, e := OwnershipChainIdentityFor(b.Source)
	if e != nil {
		return nil, e
	}
	// Source is intentionally absent from this private process-only argument.
	// attachNodes/checkOwnerContext dispatch through the retained owned binding.
	return attachNodes(ctx, OwnerBinding{Home: b.Home, Primary: b.Primary, Peer: b.Peer}, id, nil, &b)
}
func AttachOwnershipPoolResult(ctx context.Context, b OwnershipPoolOwnerBinding) (*ResultPublisher, error) {
	p, e := AttachOwnershipPool(ctx, b)
	if e != nil {
		return nil, e
	}
	return &ResultPublisher{owner: p}, nil
}
