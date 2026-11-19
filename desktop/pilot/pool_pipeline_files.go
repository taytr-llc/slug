// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"io"
	"os"
	"path/filepath"
)

var poolPipelineNames = [4]string{"seeds", "cards", "bouts", "results"}

func poolCanonicalHash(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func derivePoolPipeline(l poolLeagueReady) (poolPipelineIntent, poolPipelineChildBinding, error) {
	var b poolPipelineChildBinding
	var e error
	b.SeedPin, e = gymseed.CandidatePin(l.Intent.EventPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	b.CardPin, e = gymcard.CandidatePin(b.SeedPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	b.BoutPin, e = gymbout.CandidatePin(b.CardPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	b.ResultPin, e = gymresult.CandidatePin(b.BoutPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	b.SeedStoreID, e = gymseed.ID(b.SeedPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	b.CardStoreID, e = gymcard.ID(b.CardPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	b.BoutStoreID, e = gymbout.ID(b.BoutPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	b.ResultStoreID, e = gymresult.ID(b.ResultPin)
	if e != nil {
		return poolPipelineIntent{}, b, e
	}
	v := poolPipelineIntent{"slug-shared-pool-event-cycle", 1, l.Intent.SourceBindingID, poolCanonicalHash(l), l.Intent.EventStoreID, l.EventCreatedMS, b.SeedStoreID, b.CardStoreID, b.BoutStoreID, b.ResultStoreID}
	return v, b, nil
}
func poolPipelineFiles(root string) error {
	allowed := map[string]bool{poolPipelineIntentName: true, poolPipelineActivatingName: true, poolPipelineActivationName: true, poolPipelineReadyName: true}
	for _, n := range poolPipelineNames {
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
func (b *poolPipelineBootstrap) prepared() (poolPipelinePrepared, error) {
	for _, s := range b.stages {
		if s == nil {
			return poolPipelinePrepared{}, ErrPoolRuntime
		}
	}
	return poolPipelinePrepared{1, b.intent, *b.stages[0], *b.stages[1], *b.stages[2], *b.stages[3]}, nil
}

// Read bootstrap identity before opening guarded participant stores. No disk
// value replaces a recipient/source pin; all store IDs are derived again.
func (r *PoolRuntime) loadPipelineBootstrap(l poolLeagueReady, leagueReady bool) error {
	r.pipelineBootstrap = nil
	root := filepath.Join(r.config.DataDir, poolPipelineDirectory)
	exists, e := poolLeagueDirectoryExists(root)
	if e != nil {
		return e
	}
	if !exists {
		return nil
	}
	if !leagueReady {
		return ErrPoolRuntime
	}
	intent, _, e := derivePoolPipeline(l)
	if e != nil {
		return e
	}
	if e = poolPipelineFiles(root); e != nil {
		return e
	}
	hasIntent, e := poolRuntimeExists(filepath.Join(root, poolPipelineIntentName))
	if e != nil {
		return e
	}
	if !hasIntent {
		if !poolLeagueEmpty(root) {
			return ErrPoolRuntime
		}
		return nil
	}
	if e = readPoolRuntimeRecord(filepath.Join(root, poolPipelineIntentName), intent); e != nil {
		return e
	}
	b := &poolPipelineBootstrap{intent: intent}
	ids := [4]string{intent.SeedStoreID, intent.CardStoreID, intent.BoutStoreID, intent.ResultStoreID}
	sourceCreated := intent.EventCreatedMS
	missing := false
	for i, n := range poolPipelineNames {
		marker := filepath.Join(root, n+"-READY.json")
		ready, e := poolRuntimeExists(marker)
		if e != nil {
			return e
		}
		dir, e := poolLeagueDirectoryExists(filepath.Join(root, n))
		if e != nil {
			return e
		}
		if ready {
			if missing || !dir {
				return ErrPoolRuntime
			}
			var s poolPipelineStage
			if e = readManifestJSON(marker, poolRuntimeMarkerLimit, &s); e != nil {
				return e
			}
			if s.Version != 1 || s.StoreID != ids[i] || s.SourceCreatedMS != sourceCreated || s.CreatedMS < s.SourceCreatedMS || s.CreatedMS <= 0 {
				return ErrPoolRuntime
			}
			if e = readPoolRuntimeRecord(marker, s); e != nil {
				return e
			}
			b.stages[i] = &s
			sourceCreated = s.CreatedMS
		} else {
			// Only the next initializer can exist without its acknowledgment.
			if missing && dir {
				return ErrPoolRuntime
			}
			missing = true
		}
	}
	b.activating, e = poolRuntimeExists(filepath.Join(root, poolPipelineActivatingName))
	if e != nil {
		return e
	}
	if b.activating {
		p, e := b.prepared()
		if e != nil {
			return e
		}
		if e = readPoolRuntimeRecord(filepath.Join(root, poolPipelineActivatingName), p); e != nil {
			return e
		}
	}
	hasActivation, e := poolRuntimeExists(filepath.Join(root, poolPipelineActivationName))
	if e != nil {
		return e
	}
	if hasActivation {
		if !b.activating {
			return ErrPoolRuntime
		}
		var a careerstate.Activation
		if e = readManifestJSON(filepath.Join(root, poolPipelineActivationName), poolRuntimeMarkerLimit, &a); e != nil {
			return e
		}
		if careerstate.ValidateActivation(a) != nil || a.ResultStoreID != intent.ResultStoreID || a.ResultCreatedAtMS != b.stages[3].CreatedMS || a.SourceCreatedAtMS != b.stages[3].SourceCreatedMS {
			return ErrPoolRuntime
		}
		if e = readPoolRuntimeRecord(filepath.Join(root, poolPipelineActivationName), a); e != nil {
			return e
		}
		b.activation = &a
	}
	hasReady, e := poolRuntimeExists(filepath.Join(root, poolPipelineReadyName))
	if e != nil {
		return e
	}
	if hasReady {
		if b.activation == nil {
			return ErrPoolRuntime
		}
		p, e := b.prepared()
		if e != nil {
			return e
		}
		ready := poolPipelineReady{p, *b.activation}
		if e = readPoolRuntimeRecord(filepath.Join(root, poolPipelineReadyName), ready); e != nil {
			return e
		}
		b.ready = &ready
	}
	r.pipelineBootstrap = b
	return nil
}
func (r *PoolRuntime) openPipelineStable(path string, pin gymstable.Pin, f *poolLeagueFactories) (*gymstable.Store, error) {
	b := r.pipelineBootstrap
	if b == nil || b.activation == nil {
		return f.openStable(path, pin, r.issuer)
	}
	if b.ready != nil {
		return f.openSharedCareer(path, pin, r.issuer, *b.activation)
	}
	return f.resumeSharedCareer(path, pin, r.issuer, *b.activation)
}
