// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func pipelineTest(t *testing.T) *PoolRuntime {
	t.Helper()
	r := poolLeagueTest(t)
	if e := r.prepareLeague(context.Background()); e != nil {
		t.Fatal(e)
	}
	return r
}
func pipelineRelease(t *testing.T, r *PoolRuntime) {
	t.Helper()
	for _, close := range []func() error{r.results.Close, r.bouts.Close, r.cards.Close, r.seeds.Close} {
		if e := close(); e != nil {
			t.Fatal(e)
		}
	}
	r.pipeline = nil
	r.results = nil
	r.bouts = nil
	r.cards = nil
	r.seeds = nil
	r.careerReader = nil
	poolLeagueReleaseOwner(t, r)
}
func TestPoolPipelineActivatesBeforePublicationAndReopensExact(t *testing.T) {
	r := pipelineTest(t)
	ctx := context.Background()
	if e := r.preparePipeline(ctx); e != nil {
		t.Fatal(e)
	}
	want := *r.pipeline
	if want.Career.ResultStoreID != want.Prepared.Intent.ResultStoreID {
		t.Fatal("unbound activation")
	}
	for _, p := range r.league.Intent.Participants {
		path := filepath.Join(r.config.DataDir, "gyms", p.AccountID, "stable")
		if s, e := gymstable.OpenShared(path, p.EntryPin.StablePin, r.issuer); e == nil {
			s.Close()
			t.Fatal("legacy open bypassed career guard")
		}
		s, e := r.leagueFactories.openSharedCareer(path, p.EntryPin.StablePin, r.issuer, want.Career)
		if e != nil {
			t.Fatal(e)
		}
		e = errors.Join(s.AttachCareer(ctx, r.careerReader), s.Close())
		if e != nil {
			t.Fatal(e)
		}
	}
	root := filepath.Join(r.config.DataDir, poolPipelineDirectory)
	before, e := os.ReadFile(filepath.Join(root, poolPipelineReadyName))
	if e != nil {
		t.Fatal(e)
	}
	pipelineRelease(t, r)
	f := defaultPoolLeagueFactories()
	f.openStable = func(string, gymstable.Pin, *registry.ManifestStore) (*gymstable.Store, error) {
		t.Fatal("unguarded stable reopen")
		return nil, nil
	}
	f.resumeSharedCareer = func(string, gymstable.Pin, *registry.ManifestStore, careerstate.Activation) (*gymstable.Store, error) {
		t.Fatal("ready resumed instead of opened")
		return nil, nil
	}
	f.resumeCareerResult = func(context.Context, string, gymresult.Pin, *gymbout.Store) (*gymresult.Store, error) {
		t.Fatal("ready activation resumed")
		return nil, nil
	}
	r.leagueFactories = f
	if e = r.prepareLeague(ctx); e != nil {
		t.Fatal(e)
	}
	if e = r.preparePipeline(ctx); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(root, poolPipelineReadyName))
	if !reflect.DeepEqual(*r.pipeline, want) || string(before) != string(after) {
		t.Fatal("restart replaced activation or store epochs")
	}
}
func TestPoolPipelineResumesOriginalActivationAfterUncertainCommit(t *testing.T) {
	for _, where := range []string{"activation", "second-stable"} {
		t.Run(where, func(t *testing.T) {
			r := pipelineTest(t)
			ctx := context.Background()
			lost := errors.New("public test uncertain activation")
			f := defaultPoolLeagueFactories()
			var original careerstate.Activation
			if where == "activation" {
				f.resumeCareerResult = func(ctx context.Context, p string, pin gymresult.Pin, src *gymbout.Store) (*gymresult.Store, error) {
					s, e := gymresult.ResumeCareerActivation(ctx, p, pin, src)
					if e != nil {
						return nil, e
					}
					original, e = s.CareerInfo(ctx)
					return nil, errors.Join(lost, e, s.Close())
				}
			}
			if where == "second-stable" {
				calls := 0
				f.resumeSharedCareer = func(p string, pin gymstable.Pin, src *registry.ManifestStore, a careerstate.Activation) (*gymstable.Store, error) {
					calls++
					original = a
					if calls == 2 {
						return nil, lost
					}
					return gymstable.ResumeSharedCareerBinding(p, pin, src, a)
				}
			}
			r.leagueFactories = f
			if e := r.preparePipeline(ctx); !errors.Is(e, lost) {
				t.Fatal("missing uncertainty", e)
			}
			if r.pipeline != nil || r.results != nil {
				t.Fatal("partial authority published")
			}
			poolLeagueReleaseOwner(t, r)
			r.leagueFactories = defaultPoolLeagueFactories()
			if e := r.prepareLeague(ctx); e != nil {
				t.Fatal("bootstrap", e)
			}
			if e := r.preparePipeline(ctx); e != nil {
				t.Fatal("resume", e)
			}
			if original != r.pipeline.Career {
				t.Fatal("retry generated different activation")
			}
		})
	}
}
func TestPoolPipelineEstablishedLossRefusesRecreation(t *testing.T) {
	for _, name := range []string{"seeds", "cards", "bouts", "results", "cards-READY.json", poolPipelineIntentName, poolPipelineActivatingName, poolPipelineActivationName, "stable"} {
		t.Run(name, func(t *testing.T) {
			r := pipelineTest(t)
			ctx := context.Background()
			if e := r.preparePipeline(ctx); e != nil {
				t.Fatal(e)
			}
			id := r.league.Intent.Participants[0].AccountID
			pipelineRelease(t, r)
			path := filepath.Join(r.config.DataDir, poolPipelineDirectory, name)
			if name == "stable" {
				path = filepath.Join(r.config.DataDir, "gyms", id, "stable")
			}
			if e := os.Rename(path, filepath.Join(t.TempDir(), "retained")); e != nil {
				t.Fatal(e)
			}
			e := r.prepareLeague(ctx)
			if e == nil {
				e = r.preparePipeline(ctx)
			}
			if e == nil {
				t.Fatal("lost established component accepted")
			}
			if _, e := os.Lstat(path); !os.IsNotExist(e) {
				t.Fatal("component recreated", e)
			}
		})
	}
}
func TestPoolPipelineRefusesOrphanAndForeignIntent(t *testing.T) {
	for _, kind := range []string{"orphan", "foreign", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			r := pipelineTest(t)
			root := filepath.Join(r.config.DataDir, poolPipelineDirectory)
			if e := os.Mkdir(root, 0700); e != nil {
				t.Fatal(e)
			}
			v, _, e := derivePoolPipeline(*r.league)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "orphan":
				e = os.WriteFile(filepath.Join(root, poolPipelineReadyName), []byte("{}\n"), 0600)
			case "foreign":
				v.SourceBindingID = strings.Repeat("f", 64)
				e = writePoolRuntimeRecord(filepath.Join(root, poolPipelineIntentName), v)
			case "symlink":
				e = os.Symlink(t.TempDir(), filepath.Join(root, "seeds"))
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = r.preparePipeline(context.Background()); e == nil {
				t.Fatal("untrusted pipeline accepted")
			}
			if r.results != nil || r.pipeline != nil {
				t.Fatal("invalid authority published")
			}
		})
	}
}
func TestPoolPipelineCloseJoinsChildBeforeCareerOwner(t *testing.T) {
	r := pipelineTest(t)
	if e := r.preparePipeline(context.Background()); e != nil {
		t.Fatal(e)
	}
	results, reader := r.results, r.careerReader
	b := &poolRuntimeBorrow{account: "public fixture", done: make(chan struct{})}
	b.close = func() error {
		if _, e := reader.Activation(); e != nil {
			t.Error("reader closed before child", e)
		}
		if _, e := results.CareerInfo(context.Background()); e != nil {
			t.Error("result closed before child", e)
		}
		r.mu.Lock()
		delete(r.borrows, b)
		close(b.done)
		r.mu.Unlock()
		return nil
	}
	r.borrows[b] = true
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := results.CareerInfo(context.Background()); e == nil {
		t.Fatal("result owner retained")
	}
}
