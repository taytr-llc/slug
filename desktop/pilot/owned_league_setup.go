// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

func OpenOwnedLeague(ctx context.Context, r *PoolRuntime, expected OwnedLeagueConfig) (*OwnedLeague, error) {
	if r == nil {
		return nil, ErrPoolRuntime
	}
	work, done, e := r.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	return openOwnedLeague(work, r, expected)
}

// Caller owns r.gate. Both actual wallet recipients have been verified in this
// runtime epoch. Every authority/pin comes from expected, never an opened store.
func openOwnedLeague(ctx context.Context, r *PoolRuntime, expected OwnedLeagueConfig) (out *OwnedLeague, resultErr error) {
	if wire.TestPoW || ctx == nil || ctx.Err() != nil || r == nil || ValidateOwnedLeagueConfig(expected) != nil {
		return nil, ErrPoolRuntime
	}
	r.mu.Lock()
	valid := !r.closed && r.life.Err() == nil && r.owned != nil && r.owned.config == expected.Source && r.source == expected.PoolSource && r.owned.store != nil
	if valid {
		for _, p := range expected.Participants {
			b, ok := r.recipients[p.Fixture]
			if !ok || !r.verified[p.Fixture] || b.Config.AccountAuthorityID != p.StablePin.Stable.AccountAuthorityID || b.Config.AccountID != p.StablePin.Stable.AccountID || b.Config.Identity != p.StablePin.Stable.Identity {
				valid = false
				break
			}
		}
	}
	var l *OwnedLeague
	if valid {
		life, cancel := context.WithCancel(r.life)
		l = &OwnedLeague{config: expected, runtime: r, source: r.owned.store, root: filepath.Join(r.config.DataDir, ownedLeagueDirectory), life: life, cancel: cancel}
	}
	r.mu.Unlock()
	if !valid {
		return nil, ErrPoolRuntime
	}
	defer func() {
		if resultErr != nil {
			l.retainCleanup(resultErr)
			ce := l.Close()
			resultErr = errors.Join(resultErr, ce)
			if ce != nil {
				out = l
			} else {
				out = nil
			}
		}
	}()
	b, e := loadOwnedLeague(l.root, expected)
	if e != nil {
		return nil, e
	}
	established, e := poolRuntimeExists(filepath.Join(r.config.DataDir, poolRuntimeReadyName))
	if e != nil || established && b.ready == nil {
		return nil, ErrPoolRuntime
	}
	if e = poolLeagueMkdir(l.root); e != nil {
		return nil, e
	}
	if e = ownedLeagueWrite(filepath.Join(l.root, ownedLeagueIntentName), b.intent); e != nil {
		return nil, e
	}
	if e = l.openParticipants(ctx, b); e != nil {
		return nil, e
	}
	path, exists, e := l.stagePath(b, 4)
	if e != nil {
		return nil, e
	}
	if exists {
		l.events, e = gymevent.OpenOwned(ctx, path, expected.EventPin, l.source)
	} else {
		l.events, e = gymevent.CreateOwned(ctx, path, expected.EventPin, l.source)
	}
	if e != nil {
		return nil, e
	}
	info, e := l.events.Info(ctx)
	if e != nil || info.StoreID != expected.EventStoreID || !reflect.DeepEqual(info.Pin, expected.EventPin) {
		return nil, ErrPoolRuntime
	}
	if e = l.commitStage(b, 4, info.CreatedAtMS, 0); e != nil {
		return nil, e
	}
	if e = l.openDownstream(ctx, b); e != nil {
		return nil, e
	}
	prepared, e := b.prepared()
	if e != nil {
		return nil, e
	}
	if !b.activating {
		if e = l.results.Close(); e != nil {
			return nil, e
		}
		l.results = nil
		if e = gymresult.CheckOwnedCareerActivation(ctx, filepath.Join(l.root, "results"), expected.ResultPin, l.bouts); e != nil {
			return nil, e
		}
		if e = ownedLeagueWrite(filepath.Join(l.root, ownedLeagueActivatingName), prepared); e != nil {
			return nil, e
		}
		b.activating = true
		l.results, e = gymresult.ResumeOwnedCareerActivation(ctx, filepath.Join(l.root, "results"), expected.ResultPin, l.bouts)
		if e != nil {
			return nil, e
		}
	}
	activation, e := l.results.CareerInfo(ctx)
	if e != nil {
		return nil, e
	}
	if b.activation != nil && *b.activation != activation {
		return nil, ErrPoolRuntime
	}
	if e = ownedLeagueWrite(filepath.Join(l.root, ownedLeagueActivationName), activation); e != nil {
		return nil, e
	}
	b.activation = &activation
	l.activation = activation
	l.career, e = l.results.CareerReader()
	if e != nil {
		return nil, e
	}
	for _, s := range l.stable {
		if b.ready != nil {
			e = s.AttachCareer(ctx, l.career)
		} else {
			e = s.BindCareer(ctx, l.career)
		}
		if e != nil {
			return nil, e
		}
	}
	if b.ready != nil {
		e = l.cards.AttachCareer(ctx, l.career)
	} else {
		e = l.cards.BindCareer(ctx, l.career)
	}
	if e != nil {
		return nil, e
	}
	ready := ownedLeagueReady{Version: 1, Prepared: prepared, Career: activation}
	if b.ready != nil && *b.ready != ready {
		return nil, ErrPoolRuntime
	}
	if ctx.Err() != nil || l.life.Err() != nil {
		return nil, context.Canceled
	}
	if e = ownedLeagueFiles(l.root); e != nil {
		return nil, e
	}
	if e = ownedLeagueWrite(filepath.Join(l.root, ownedLeagueReadyName), ready); e != nil {
		return nil, e
	}
	l.ready = &ready
	return l, nil
}

// Only an unacknowledged next initializer may have an empty directory left by
// its initial mkdir. Established receipts forbid a missing/empty replacement.
func (l *OwnedLeague) stagePath(b *ownedLeagueBootstrap, index int) (string, bool, error) {
	p := filepath.Join(l.root, ownedLeagueNames[index])
	exists, e := poolLeagueDirectoryExists(p)
	if e != nil {
		return p, false, e
	}
	if b.stages[index] != nil {
		if !exists {
			return p, false, ErrPoolRuntime
		}
		return p, true, nil
	}
	if exists && poolLeagueEmpty(p) {
		if e = os.Remove(p); e != nil {
			return p, false, e
		}
		if e = syncDirectory(l.root); e != nil {
			return p, false, e
		}
		exists = false
	}
	return p, exists, nil
}
func (l *OwnedLeague) openParticipants(ctx context.Context, b *ownedLeagueBootstrap) error {
	for i, p := range l.config.Participants {
		path, exists, e := l.stagePath(b, i*2)
		if e != nil {
			return e
		}
		if !exists {
			l.stable[i], e = gymstable.CreateOwned(path, p.StablePin, l.source)
		} else if b.activation != nil {
			bindings := gymstable.OwnedCareerBindings{Current: *b.activation}
			if b.ready != nil {
				l.stable[i], e = gymstable.OpenOwnedWithCareerBindings(path, p.StablePin, l.source, bindings)
			} else {
				l.stable[i], e = gymstable.ResumeOwnedCareerBinding(path, p.StablePin, l.source, bindings)
			}
		} else {
			l.stable[i], e = gymstable.OpenOwned(path, p.StablePin, l.source)
		}
		if e != nil {
			return e
		}
		status, e := l.stable[i].SourceInfo(ctx)
		if e != nil || status.StoreID != p.StableStoreID || status.Pin != p.StablePin {
			return ErrPoolRuntime
		}
		if e = l.commitStage(b, i*2, 0, 0); e != nil {
			return e
		}
		path, exists, e = l.stagePath(b, i*2+1)
		if e != nil {
			return e
		}
		if exists {
			l.entry[i], e = gymentry.OpenOwned(ctx, path, p.EntryPin, l.stable[i])
		} else {
			l.entry[i], e = gymentry.CreateOwned(ctx, path, p.EntryPin, l.stable[i])
		}
		if e != nil {
			return e
		}
		entry, e := l.entry[i].SnapshotSourceInfo(ctx)
		if e != nil || entry.StoreID != p.EntryStoreID || entry.Pin != p.EntryPin {
			return ErrPoolRuntime
		}
		if e = l.commitStage(b, i*2+1, 0, 0); e != nil {
			return e
		}
	}
	return nil
}
func (l *OwnedLeague) openDownstream(ctx context.Context, b *ownedLeagueBootstrap) error {
	c := l.config
	path, exists, e := l.stagePath(b, 5)
	if e != nil {
		return e
	}
	if !exists {
		l.seeds, e = gymseed.CreateOwned(ctx, path, c.SeedPin, l.events)
	} else if b.stages[5] == nil {
		l.seeds, e = gymseed.ResumeOwnedPrepared(ctx, path, c.SeedPin, l.events)
	} else {
		l.seeds, e = gymseed.OpenOwned(ctx, path, c.SeedPin, l.events)
	}
	if e != nil {
		return e
	}
	seed, e := l.seeds.Info(ctx)
	if e != nil || seed.StoreID != c.SeedStoreID || !reflect.DeepEqual(seed.Pin, c.SeedPin) {
		return ErrPoolRuntime
	}
	if e = l.commitStage(b, 5, seed.CreatedAtMS, b.stages[4].CreatedAtMS); e != nil {
		return e
	}
	path, exists, e = l.stagePath(b, 6)
	if e != nil {
		return e
	}
	if !exists {
		l.cards, e = gymcard.CreateOwned(ctx, path, c.CardPin, l.seeds)
	} else if b.activation != nil {
		if b.ready != nil {
			l.cards, e = gymcard.OpenOwnedCareer(ctx, path, c.CardPin, l.seeds, *b.activation)
		} else {
			l.cards, e = gymcard.ResumeOwnedCareerBinding(ctx, path, c.CardPin, l.seeds, *b.activation)
		}
	} else if b.stages[6] == nil {
		l.cards, e = gymcard.ResumeOwnedPrepared(ctx, path, c.CardPin, l.seeds)
	} else {
		l.cards, e = gymcard.OpenOwned(ctx, path, c.CardPin, l.seeds)
	}
	if e != nil {
		return e
	}
	card, e := l.cards.Info(ctx)
	if e != nil || card.StoreID != c.CardStoreID || !reflect.DeepEqual(card.Pin, c.CardPin) || card.SourceCreatedAtMS != seed.CreatedAtMS {
		return ErrPoolRuntime
	}
	if e = l.commitStage(b, 6, card.CreatedAtMS, card.SourceCreatedAtMS); e != nil {
		return e
	}
	path, exists, e = l.stagePath(b, 7)
	if e != nil {
		return e
	}
	if !exists {
		l.bouts, e = gymbout.CreateOwned(ctx, path, c.BoutPin, l.cards)
	} else if b.stages[7] == nil {
		l.bouts, e = gymbout.ResumeOwnedPrepared(ctx, path, c.BoutPin, l.cards)
	} else {
		l.bouts, e = gymbout.OpenOwned(ctx, path, c.BoutPin, l.cards)
	}
	if e != nil {
		return e
	}
	bout, e := l.bouts.Info(ctx)
	if e != nil || bout.StoreID != c.BoutStoreID || !reflect.DeepEqual(bout.Pin, c.BoutPin) || bout.SourceCreatedAtMS != card.CreatedAtMS {
		return ErrPoolRuntime
	}
	if e = l.commitStage(b, 7, bout.CreatedAtMS, bout.SourceCreatedAtMS); e != nil {
		return e
	}
	path, exists, e = l.stagePath(b, 8)
	if e != nil {
		return e
	}
	if !exists {
		l.results, e = gymresult.CreateOwned(ctx, path, c.ResultPin, l.bouts)
	} else if b.ready != nil {
		l.results, e = gymresult.OpenOwnedCareer(ctx, path, c.ResultPin, l.bouts)
	} else if b.activating {
		l.results, e = gymresult.ResumeOwnedCareerActivation(ctx, path, c.ResultPin, l.bouts)
	} else if b.stages[8] == nil {
		l.results, e = gymresult.ResumeOwnedPrepared(ctx, path, c.ResultPin, l.bouts)
	} else {
		l.results, e = gymresult.OpenOwned(ctx, path, c.ResultPin, l.bouts)
	}
	if e != nil {
		return e
	}
	result, e := l.results.Info(ctx)
	if e != nil || result.StoreID != c.ResultStoreID || !reflect.DeepEqual(result.Pin, c.ResultPin) || result.SourceCreatedAtMS != bout.CreatedAtMS {
		return ErrPoolRuntime
	}
	return l.commitStage(b, 8, result.CreatedAtMS, result.SourceCreatedAtMS)
}

// Constructor errors may retain a failed concrete owner even when the return
// pointer is nil. Keep that owner attached to the aggregate until cleanup is
// definitively complete; no namespace lock is discarded on this path.
func (l *OwnedLeague) retainCleanup(e error) {
	var stable *gymstable.OwnedCleanupError
	var entry *gymentry.OwnedCleanupError
	var event *gymevent.OwnedCleanupError
	var seed *gymseed.OwnedCleanupError
	var card *gymcard.OwnedCleanupError
	var bout *gymbout.OwnedCleanupError
	var result *gymresult.OwnedCleanupError
	add := func(v io.Closer) { l.failed = append(l.failed, v) }
	if errors.As(e, &stable) && stable.Owner != nil {
		add(stable.Owner)
	}
	if errors.As(e, &entry) && entry.Owner != nil {
		add(entry.Owner)
	}
	if errors.As(e, &event) && event.Owner != nil {
		add(event.Owner)
	}
	if errors.As(e, &seed) && seed.Owner != nil {
		add(seed.Owner)
	}
	if errors.As(e, &card) && card.Owner != nil {
		add(card.Owner)
	}
	if errors.As(e, &bout) && bout.Owner != nil {
		add(bout.Owner)
	}
	if errors.As(e, &result) && result.Owner != nil {
		add(result.Owner)
	}
}

// Keep career activation type explicit across the setup receipt and live owner.
