// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
)

type OwnedLeagueParticipant struct {
	Fixture       string             `json:"fixture"`
	StablePin     gymstable.OwnedPin `json:"stable_pin"`
	StableStoreID string             `json:"stable_store_id"`
	EntryPin      gymentry.OwnedPin  `json:"entry_pin"`
	EntryStoreID  string             `json:"entry_store_id"`
}

// Config is independently derived from the retained outer authority/source and
// actual enrolled accounts. A marker may corroborate it, never supply its pins.
type OwnedLeagueConfig struct {
	Version         uint32                    `json:"version"`
	Source          OwnedSourceConfig         `json:"source"`
	PoolSource      poolshare.Config          `json:"pool_source"`
	SourceBindingID string                    `json:"source_binding_id"`
	Participants    [2]OwnedLeagueParticipant `json:"participants"`
	EventPin        gymevent.OwnedPin         `json:"event_pin"`
	EventStoreID    string                    `json:"event_store_id"`
	SeedPin         gymseed.OwnedPin          `json:"seed_pin"`
	SeedStoreID     string                    `json:"seed_store_id"`
	CardPin         gymcard.OwnedPin          `json:"card_pin"`
	CardStoreID     string                    `json:"card_store_id"`
	BoutPin         gymbout.OwnedPin          `json:"bout_pin"`
	BoutStoreID     string                    `json:"bout_store_id"`
	ResultPin       gymresult.OwnedPin        `json:"result_pin"`
	ResultStoreID   string                    `json:"result_store_id"`
}

func OwnedLeagueConfigFor(source OwnedSourceConfig, pool poolshare.Config, authorityID, zeroAccountID, incrementingAccountID string) (OwnedLeagueConfig, error) {
	var out OwnedLeagueConfig
	if ValidateOwnedGymSource(source) != nil || pool.AuthorityPin != source.Ownership.Source || !payoutNonzeroID(authorityID) || !payoutNonzeroID(zeroAccountID) || !payoutNonzeroID(incrementingAccountID) || zeroAccountID == incrementingAccountID {
		return out, ErrPoolRuntime
	}
	sourceID, e := poolservice.BindingID(pool)
	if e != nil {
		return out, e
	}
	out.Version, out.Source, out.PoolSource, out.SourceBindingID = 1, source, pool, sourceID
	ids := [2]string{zeroAccountID, incrementingAccountID}
	for i, fixture := range [2]string{"zero256", "incrementing256"} {
		identity, e := gymaccount.FixtureIdentity(fixture)
		if e != nil {
			return OwnedLeagueConfig{}, e
		}
		stable := gymstable.OwnedPin{Version: 1, Stable: gymstable.Pin{Version: gymstable.SharedPinVersion2, AccountAuthorityID: authorityID, AccountID: ids[i], Identity: identity, Source: pool.AuthorityPin, ConditionVersion: 1, ProjectorID: fightercondition.ProjectorID}, Ownership: source.Ownership}
		sid, e := gymstable.OwnedID(stable)
		if e != nil {
			return OwnedLeagueConfig{}, e
		}
		namespace, e := gymentry.NamespaceID(identity.Network, identity.Genesis, authorityID)
		if e != nil {
			return OwnedLeagueConfig{}, e
		}
		entry := gymentry.OwnedPin{Version: 1, NamespaceID: namespace, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: sid, StablePin: stable}
		eid, e := gymentry.OwnedID(entry)
		if e != nil {
			return OwnedLeagueConfig{}, e
		}
		out.Participants[i] = OwnedLeagueParticipant{fixture, stable, sid, entry, eid}
	}
	participants := []gymentry.OwnedPin{out.Participants[0].EntryPin, out.Participants[1].EntryPin}
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].StablePin.Stable.Identity.GymID < participants[j].StablePin.Stable.Identity.GymID
	})
	out.EventPin = gymevent.OwnedPin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.OwnedRulesID(), Ownership: source.Ownership, Participants: participants}
	if out.EventStoreID, e = gymevent.OwnedID(out.EventPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.SeedPin, e = gymseed.CandidateOwnedPin(out.EventPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.SeedStoreID, e = gymseed.OwnedID(out.SeedPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.CardPin, e = gymcard.CandidateOwnedPin(out.SeedPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.CardStoreID, e = gymcard.OwnedID(out.CardPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.BoutPin, e = gymbout.CandidateOwnedPin(out.CardPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.BoutStoreID, e = gymbout.OwnedID(out.BoutPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.ResultPin, e = gymresult.CandidateOwnedPin(out.BoutPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	if out.ResultStoreID, e = gymresult.OwnedID(out.ResultPin); e != nil {
		return OwnedLeagueConfig{}, e
	}
	return out, nil
}
func ValidateOwnedLeagueConfig(c OwnedLeagueConfig) error {
	want, e := OwnedLeagueConfigFor(c.Source, c.PoolSource, c.Participants[0].StablePin.Stable.AccountAuthorityID, c.Participants[0].StablePin.Stable.AccountID, c.Participants[1].StablePin.Stable.AccountID)
	if e != nil || poolCanonicalHash(want) != poolCanonicalHash(c) {
		return ErrPoolRuntime
	}
	return nil
}

type OwnedLeague struct {
	failed       []io.Closer
	config       OwnedLeagueConfig
	runtime      *PoolRuntime
	source       *fighterownership.Store
	root         string
	life         context.Context
	cancel       context.CancelFunc
	stable       [2]*gymstable.OwnedStore
	entry        [2]*gymentry.OwnedStore
	events       *gymevent.OwnedStore
	seeds        *gymseed.OwnedStore
	cards        *gymcard.OwnedStore
	bouts        *gymbout.OwnedStore
	results      *gymresult.OwnedStore
	career       *careerstate.Reader
	activation   careerstate.Activation
	ready        *ownedLeagueReady
	cutoffReplay *fighterownership.Replay
	cutoffSource *fighterownership.PublicSource
	cutoffEvent  string
	cutoffDate   string
	calendarNow  func() time.Time // nil uses real time; only the tagged idle fixture seam sets it
	closeOnce    sync.Once
	closeErr     error
}

func (l *OwnedLeague) Config() OwnedLeagueConfig {
	if l == nil {
		return OwnedLeagueConfig{}
	}
	raw, _ := json.Marshal(l.config)
	var out OwnedLeagueConfig
	_ = json.Unmarshal(raw, &out)
	return out
}
func (l *OwnedLeague) Participant(fixture string) (*gymstable.OwnedStore, *gymentry.OwnedStore, error) {
	if l == nil || l.life.Err() != nil || l.ready == nil {
		return nil, nil, ErrPoolRuntime
	}
	for i, p := range l.config.Participants {
		if p.Fixture == fixture && l.stable[i] != nil && l.entry[i] != nil {
			return l.stable[i], l.entry[i], nil
		}
	}
	return nil, nil, ErrPoolRuntime
}
func (l *OwnedLeague) Events() *gymevent.OwnedStore {
	if l == nil || l.life.Err() != nil || l.ready == nil {
		return nil
	}
	return l.events
}
func (l *OwnedLeague) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
		if l.cutoffReplay != nil {
			l.closeErr = errors.Join(l.closeErr, l.cutoffReplay.Close())
		}
		// The parent joins all children before this method; only then can career
		// readers and their immutable source chain be closed in reverse order.
		for _, owner := range l.failed {
			l.closeErr = errors.Join(l.closeErr, owner.Close())
		}
		if l.results != nil {
			l.closeErr = errors.Join(l.closeErr, l.results.Close())
		}
		if l.bouts != nil {
			l.closeErr = errors.Join(l.closeErr, l.bouts.Close())
		}
		if l.cards != nil {
			l.closeErr = errors.Join(l.closeErr, l.cards.Close())
		}
		if l.seeds != nil {
			l.closeErr = errors.Join(l.closeErr, l.seeds.Close())
		}
		if l.events != nil {
			l.closeErr = errors.Join(l.closeErr, l.events.Close())
		}
		for i := 1; i >= 0; i-- {
			if l.entry[i] != nil {
				l.closeErr = errors.Join(l.closeErr, l.entry[i].Close())
			}
			if l.stable[i] != nil {
				l.closeErr = errors.Join(l.closeErr, l.stable[i].Close())
			}
		}
	})
	return l.closeErr
}
