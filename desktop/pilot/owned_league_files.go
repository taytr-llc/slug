// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

const ownedLeagueDirectory = "owned-league"
const ownedLeagueIntentName = "SLUG-OWNED-LEAGUE-INTENT.json"
const ownedLeagueReadyName = "SLUG-OWNED-LEAGUE-READY.json"
const ownedLeagueActivatingName = "SLUG-OWNED-LEAGUE-ACTIVATING.json"
const ownedLeagueActivationName = "SLUG-OWNED-LEAGUE-ACTIVATION.json"
const ownedLeagueMarkerLimit = 64 * 1024

var ownedLeagueNames = [9]string{"zero256-stable", "zero256-entries", "incrementing256-stable", "incrementing256-entries", "events", "seeds", "cards", "bouts", "results"}

type ownedLeagueIntent struct {
	Format  string            `json:"format"`
	Version uint32            `json:"version"`
	Config  OwnedLeagueConfig `json:"config"`
}
type ownedLeagueStage struct {
	Version           uint32 `json:"version"`
	Name              string `json:"name"`
	StoreID           string `json:"store_id"`
	CreatedAtMS       int64  `json:"created_at_ms"`
	SourceCreatedAtMS int64  `json:"source_created_at_ms"`
}
type ownedLeaguePrepared struct {
	Version      uint32              `json:"version"`
	IntentSHA256 string              `json:"intent_sha256"`
	Stages       [9]ownedLeagueStage `json:"stages"`
}
type ownedLeagueReady struct {
	Version  uint32                 `json:"version"`
	Prepared ownedLeaguePrepared    `json:"prepared"`
	Career   careerstate.Activation `json:"career"`
}
type ownedLeagueBootstrap struct {
	intent     ownedLeagueIntent
	stages     [9]*ownedLeagueStage
	activating bool
	activation *careerstate.Activation
	ready      *ownedLeagueReady
}

func ownedLeagueRead(path string, out any) error {
	st, e := os.Lstat(path)
	if e != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() < 2 || st.Size() > ownedLeagueMarkerLimit {
		return ErrPoolRuntime
	}
	f, e := os.Open(path)
	if e != nil {
		return ErrPoolRuntime
	}
	now, e := f.Stat()
	if e != nil || !os.SameFile(st, now) {
		f.Close()
		return ErrPoolRuntime
	}
	raw, re := io.ReadAll(io.LimitReader(f, ownedLeagueMarkerLimit+1))
	ce := f.Close()
	if re != nil || ce != nil || int64(len(raw)) != st.Size() || raw[len(raw)-1] != '\n' {
		return ErrPoolRuntime
	}
	if ownedcodec.Decode(raw[:len(raw)-1], ownedLeagueMarkerLimit, out) != nil {
		return ErrPoolRuntime
	}
	canonical, e := json.Marshal(out)
	if e != nil || !bytes.Equal(canonical, raw[:len(raw)-1]) {
		return ErrPoolRuntime
	}
	return nil
}
func ownedLeagueWrite(path string, value any) error {
	return writePoolRuntimeRecordLimit(path, value, ownedLeagueMarkerLimit)
}
func ownedLeagueIDs(c OwnedLeagueConfig) [9]string {
	return [9]string{c.Participants[0].StableStoreID, c.Participants[0].EntryStoreID, c.Participants[1].StableStoreID, c.Participants[1].EntryStoreID, c.EventStoreID, c.SeedStoreID, c.CardStoreID, c.BoutStoreID, c.ResultStoreID}
}
func (b *ownedLeagueBootstrap) prepared() (ownedLeaguePrepared, error) {
	p := ownedLeaguePrepared{Version: 1, IntentSHA256: poolCanonicalHash(b.intent)}
	for i, s := range b.stages {
		if s == nil {
			return p, ErrPoolRuntime
		}
		p.Stages[i] = *s
	}
	return p, nil
}
func ownedLeagueFiles(root string) error {
	allowed := map[string]bool{ownedLeagueIntentName: true, ownedLeagueReadyName: true, ownedLeagueActivatingName: true, ownedLeagueActivationName: true}
	for _, n := range ownedLeagueNames {
		allowed[n] = true
		allowed[n+"-READY.json"] = true
	}
	f, e := os.Open(root)
	if e != nil {
		return e
	}
	entries, re := f.ReadDir(len(allowed) + 1)
	ce := f.Close()
	if re != nil && !errors.Is(re, io.EOF) || ce != nil || len(entries) > len(allowed) {
		return ErrPoolRuntime
	}
	for _, x := range entries {
		if !allowed[x.Name()] || x.Type()&os.ModeSymlink != 0 {
			return ErrPoolRuntime
		}
	}
	return nil
}
func loadOwnedLeague(root string, c OwnedLeagueConfig) (*ownedLeagueBootstrap, error) {
	if ValidateOwnedLeagueConfig(c) != nil {
		return nil, ErrPoolRuntime
	}
	exists, e := poolLeagueDirectoryExists(root)
	if e != nil {
		return nil, e
	}
	b := &ownedLeagueBootstrap{intent: ownedLeagueIntent{"slug-owned-league", 1, c}}
	if !exists {
		return b, nil
	}
	if e = ownedLeagueFiles(root); e != nil {
		return nil, e
	}
	exists, e = poolRuntimeExists(filepath.Join(root, ownedLeagueIntentName))
	if e != nil {
		return nil, e
	}
	if !exists {
		if !poolLeagueEmpty(root) {
			return nil, ErrPoolRuntime
		}
		return b, nil
	}
	var intent ownedLeagueIntent
	if ownedLeagueRead(filepath.Join(root, ownedLeagueIntentName), &intent) != nil || !reflect.DeepEqual(intent, b.intent) {
		return nil, ErrPoolRuntime
	}
	ids := ownedLeagueIDs(c)
	missing := false
	for i, n := range ownedLeagueNames {
		dir, e := poolLeagueDirectoryExists(filepath.Join(root, n))
		if e != nil {
			return nil, e
		}
		has, e := poolRuntimeExists(filepath.Join(root, n+"-READY.json"))
		if e != nil {
			return nil, e
		}
		if has {
			if missing || !dir {
				return nil, ErrPoolRuntime
			}
			var v ownedLeagueStage
			if ownedLeagueRead(filepath.Join(root, n+"-READY.json"), &v) != nil || v.Version != 1 || v.Name != n || v.StoreID != ids[i] {
				return nil, ErrPoolRuntime
			}
			if i < 4 {
				if v.CreatedAtMS != 0 || v.SourceCreatedAtMS != 0 {
					return nil, ErrPoolRuntime
				}
			} else {
				sourceMS := int64(0)
				if i > 4 {
					sourceMS = b.stages[i-1].CreatedAtMS
				}
				if v.SourceCreatedAtMS != sourceMS || v.CreatedAtMS <= 0 || v.CreatedAtMS < sourceMS || v.CreatedAtMS > 253402300799999 {
					return nil, ErrPoolRuntime
				}
			}
			b.stages[i] = &v
		} else {
			if missing && dir {
				return nil, ErrPoolRuntime
			}
			missing = true
		}
	}
	b.activating, e = poolRuntimeExists(filepath.Join(root, ownedLeagueActivatingName))
	if e != nil {
		return nil, e
	}
	if b.activating {
		want, e := b.prepared()
		if e != nil {
			return nil, e
		}
		var got ownedLeaguePrepared
		if ownedLeagueRead(filepath.Join(root, ownedLeagueActivatingName), &got) != nil || got != want {
			return nil, ErrPoolRuntime
		}
	}
	has, e := poolRuntimeExists(filepath.Join(root, ownedLeagueActivationName))
	if e != nil {
		return nil, e
	}
	if has {
		if !b.activating {
			return nil, ErrPoolRuntime
		}
		var a careerstate.Activation
		if ownedLeagueRead(filepath.Join(root, ownedLeagueActivationName), &a) != nil || careerstate.ValidateActivation(a) != nil || a.ResultStoreID != c.ResultStoreID || a.ResultCreatedAtMS != b.stages[8].CreatedAtMS || a.SourceCreatedAtMS != b.stages[8].SourceCreatedAtMS {
			return nil, ErrPoolRuntime
		}
		b.activation = &a
	}
	has, e = poolRuntimeExists(filepath.Join(root, ownedLeagueReadyName))
	if e != nil {
		return nil, e
	}
	if has {
		if b.activation == nil {
			return nil, ErrPoolRuntime
		}
		prepared, e := b.prepared()
		if e != nil {
			return nil, e
		}
		var ready ownedLeagueReady
		want := ownedLeagueReady{1, prepared, *b.activation}
		if ownedLeagueRead(filepath.Join(root, ownedLeagueReadyName), &ready) != nil || ready != want {
			return nil, ErrPoolRuntime
		}
		b.ready = &ready
	}
	return b, nil
}
func (l *OwnedLeague) commitStage(b *ownedLeagueBootstrap, index int, created, sourceMS int64) error {
	ids := ownedLeagueIDs(l.config)
	v := ownedLeagueStage{1, ownedLeagueNames[index], ids[index], created, sourceMS}
	if index < 4 {
		if created != 0 || sourceMS != 0 {
			return ErrPoolRuntime
		}
	} else if created <= 0 || created < sourceMS || created > 253402300799999 {
		return ErrPoolRuntime
	}
	if b.stages[index] != nil && *b.stages[index] != v {
		return ErrPoolRuntime
	}
	if e := ownedLeagueWrite(filepath.Join(l.root, v.Name+"-READY.json"), v); e != nil {
		return e
	}
	b.stages[index] = &v
	return nil
}
