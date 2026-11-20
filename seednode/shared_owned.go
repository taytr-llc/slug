// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const sharedMarkerLimit = 32 << 10

// SharedPoolOwnerBinding is supplied only by the concrete shared runtime that
// owns these stores and processes. Marker hashes are SHA256 of json.Marshal on
// its already validated, retained public values (without the trailing LF).
// Reading files and blessing their hashes is not a way to establish this trust.
// Accounts' authority ID is immutable; Life must end before the runtime closes
// its borrowed account authority or any of its owned dependencies.
type SharedPoolOwnerBinding struct {
	Source              gymevent.Pin
	EventStoreID        string
	EventCreatedAtMS    int64
	Pool                poolshare.Config
	PoolBindingID       string
	Accounts            *gymaccount.Store
	Issuer              *registry.ManifestStore
	Events              *gymevent.Store
	Home                string
	Primary, Peer       OwnedProcess
	Life                context.Context
	RuntimeMarkerSHA256 string
	LeagueIntentSHA256  string
	LeagueReadySHA256   string
}

func validateSharedPoolBinding(b SharedPoolOwnerBinding) (SharedPoolOwnerBinding, error) {
	if b.Accounts == nil || b.Issuer == nil || b.Events == nil || b.Life == nil || b.Life.Err() != nil || b.EventCreatedAtMS <= 0 || b.EventCreatedAtMS > gymevent.MaxTimestampMS {
		return SharedPoolOwnerBinding{}, ErrInvalid
	}
	for _, id := range []string{b.EventStoreID, b.PoolBindingID, b.Accounts.AuthorityID(), b.RuntimeMarkerSHA256, b.LeagueIntentSHA256, b.LeagueReadySHA256} {
		if !hexValue(id, 32, true) {
			return SharedPoolOwnerBinding{}, ErrInvalid
		}
	}
	var err error
	b.Source, err = gymevent.ValidatePin(b.Source)
	if err != nil {
		return SharedPoolOwnerBinding{}, ErrInvalid
	}
	id, err := gymevent.ID(b.Source)
	if err != nil || id != b.EventStoreID {
		return SharedPoolOwnerBinding{}, ErrInvalid
	}
	id, err = poolservice.BindingID(b.Pool)
	if err != nil || id != b.PoolBindingID || b.Pool.PayoutAddress != poolsettlefixture.PoolAddress() || b.Pool.AuthorityPin.Network != gymaccount.FixtureNetwork || b.Pool.AuthorityPin.Genesis != gymaccount.FixtureGenesis {
		return SharedPoolOwnerBinding{}, ErrInvalid
	}
	zero, _ := gymaccount.FixtureIdentity("zero256")
	incrementing, _ := gymaccount.FixtureIdentity("incrementing256")
	seen := make(map[gymaccount.Identity]bool, 2)
	for _, entry := range b.Source.Participants {
		p := entry.StablePin
		if p.Version != gymstable.SharedPinVersion2 || p.AccountAuthorityID != b.Accounts.AuthorityID() || p.Source != b.Pool.AuthorityPin || (p.Identity != zero && p.Identity != incrementing) || seen[p.Identity] {
			return SharedPoolOwnerBinding{}, ErrInvalid
		}
		seen[p.Identity] = true
	}
	if len(seen) != 2 {
		return SharedPoolOwnerBinding{}, ErrInvalid
	}
	return b, nil
}

func sharedMarker(path, expected string) error {
	raw, err := readRegular(path, sharedMarkerLimit)
	if err != nil || len(raw) < 2 || raw[len(raw)-1] != '\n' {
		return ErrInvalid
	}
	digest := sha256.Sum256(raw[:len(raw)-1])
	if hex.EncodeToString(digest[:]) != expected {
		return ErrInvalid
	}
	return nil
}

// checkSharedOwner never takes an account lease. Concrete source/event reads
// are bounded by a nonblocking issuer read and the publisher's lifetime. Marker
// files corroborate those handles; they cannot replace either one.
func (p *Publisher) checkSharedOwner(ctx context.Context) error {
	b := p.shared
	if ctx.Err() != nil || b.Life.Err() != nil {
		return ErrOffline
	}
	info, err := awaitSharedIssuer(ctx, b.Life, b.Issuer.TryInfo)
	if err != nil || info.ReadOnly || info.Pin != b.Pool.AuthorityPin {
		if ctx.Err() != nil || b.Life.Err() != nil {
			return ErrOffline
		}
		return ErrUnavailable
	}
	event, err := b.Events.Info(ctx)
	if err != nil || event.StoreID != b.EventStoreID || event.CreatedAtMS != b.EventCreatedAtMS || !reflect.DeepEqual(event.Pin, b.Source) {
		return ErrUnavailable
	}
	if b.Accounts.AuthorityID() != b.Source.Participants[0].StablePin.AccountAuthorityID {
		return ErrOffline
	}
	for _, marker := range []struct{ path, digest string }{
		{filepath.Join(b.Home, "SLUG-POOL-RUNTIME.json"), b.RuntimeMarkerSHA256},
		{filepath.Join(b.Home, "league-v1", "SLUG-POOL-LEAGUE-INTENT.json"), b.LeagueIntentSHA256},
		{filepath.Join(b.Home, "league-v1", "SLUG-POOL-LEAGUE-READY.json"), b.LeagueReadySHA256},
	} {
		if err := sharedMarker(marker.path, marker.digest); err != nil {
			return err
		}
	}
	raw, err := readRegular(filepath.Join(b.Home, "SLUG-POOL-SOURCE.json"), sharedMarkerLimit)
	want, marshalErr := json.Marshal(b.Pool)
	if err != nil || marshalErr != nil || !bytes.Equal(raw, append(want, '\n')) {
		return ErrInvalid
	}
	if ctx.Err() != nil || b.Life.Err() != nil {
		return ErrOffline
	}
	return nil
}

// Temporary issuer contention is not a changed source. The only production
// caller supplies the concrete borrowed store's TryInfo method. The wait is
// outside account/event leases and never extends the request, publisher or
// runtime lifetime. All substantive errors and successful metadata return
// immediately; their exact pin/read-only checks remain with the caller.
func awaitSharedIssuer(ctx, life context.Context, read func() (registry.ManifestAuthorityInfo, error)) (registry.ManifestAuthorityInfo, error) {
	for {
		if ctx.Err() != nil || life.Err() != nil {
			return registry.ManifestAuthorityInfo{}, ErrOffline
		}
		info, err := read()
		if !errors.Is(err, registry.ErrBusy) {
			return info, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
		case <-life.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
}

func (p *Publisher) finishShared(ctx context.Context) error {
	if p.shared != nil || p.ownership != nil {
		return p.checkOwnerContext(ctx)
	}
	return nil
}

// AttachSharedPool borrows the parent's existing shared loopback node pair. It
// does not create a source, start a node, mine, fund or acquire account authority.
func AttachSharedPool(ctx context.Context, b SharedPoolOwnerBinding) (*Publisher, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrInvalid
	}
	owned, err := validateSharedPoolBinding(b)
	if err != nil {
		return nil, err
	}
	return attachOwned(ctx, OwnerBinding{Source: owned.Source, Home: owned.Home, Primary: owned.Primary, Peer: owned.Peer}, &owned)
}

// AttachSharedPoolResult uses the same concrete shared owner with the existing
// separate result funding purpose. Neither publisher spends pool rewards.
func AttachSharedPoolResult(ctx context.Context, b SharedPoolOwnerBinding) (*ResultPublisher, error) {
	owner, err := AttachSharedPool(ctx, b)
	if err != nil {
		return nil, err
	}
	return &ResultPublisher{owner: owner}, nil
}
