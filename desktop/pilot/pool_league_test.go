// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
)

func poolLeagueTest(t *testing.T) *PoolRuntime {
	t.Helper()
	r := poolRuntimeTest(t)
	canonical, e := filepath.EvalSymlinks(r.config.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	r.config.DataDir = canonical
	if e := r.openPoolIssuer(false); e != nil {
		t.Fatal(e)
	}
	for _, fixture := range []string{"zero256", "incrementing256"} {
		b := poolRuntimeTestBinding(t, r, fixture)
		poolRuntimeTestPublishGym(t, r, b, true)
		r.recipients[fixture], r.verified[fixture] = b, true
	}
	return r
}

func poolLeagueReleaseOwner(t *testing.T, r *PoolRuntime) {
	t.Helper()
	if r.events != nil {
		if e := r.events.Close(); e != nil {
			t.Fatal(e)
		}
	}
	r.events, r.league = nil, nil
}

func TestPoolLeagueExactSetupReopenAndOwnedBindings(t *testing.T) {
	r := poolLeagueTest(t)
	if e := r.prepareLeague(context.Background()); e != nil {
		t.Fatal(e)
	}
	want := *r.league
	if want.Intent.SourceBindingID == "" || len(want.Intent.Participants) != 2 || want.EventCreatedMS <= 0 {
		t.Fatal("incomplete league")
	}
	for _, p := range want.Intent.Participants {
		if p.EntryPin.StablePin.Version != gymstable.SharedPinVersion2 || p.EntryPin.StablePin.Source != r.source.AuthorityPin {
			t.Fatal("source changed")
		}
		b, e := r.leagueChild(p.AccountID, p.EntryPin.StablePin)
		if e != nil {
			t.Fatal(e)
		}
		b.EventPin.Participants[0].StablePin.AccountID = strings.Repeat("f", 64)
		if !reflect.DeepEqual(r.league.Intent, want.Intent) {
			t.Fatal("borrowed binding mutated parent")
		}
	}
	path := filepath.Join(r.config.DataDir, poolLeagueDirectory, poolLeagueReadyName)
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	poolLeagueReleaseOwner(t, r)
	f := defaultPoolLeagueFactories()
	f.createStable = func(string, gymstable.Pin, *registry.ManifestStore) (*gymstable.Store, error) {
		t.Fatal("recreated stable")
		return nil, nil
	}
	f.createEntry = func(context.Context, string, gymentry.Pin, *gymstable.Store) (*gymentry.Store, error) {
		t.Fatal("recreated entries")
		return nil, nil
	}
	f.createEvent = func(context.Context, string, gymevent.Pin) (*gymevent.Store, error) {
		t.Fatal("recreated event")
		return nil, nil
	}
	r.leagueFactories = f
	if e = r.prepareLeague(context.Background()); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(*r.league, want) {
		t.Fatal("reopen changed immutable identity/timestamp")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("ready rewritten")
	}
}

func TestPoolLeagueRecoversCompletedStoreAfterLostAcknowledgment(t *testing.T) {
	for _, where := range []string{"entry", "event"} {
		t.Run(where, func(t *testing.T) {
			r := poolLeagueTest(t)
			f := defaultPoolLeagueFactories()
			lost := errors.New("public test lost initializer acknowledgment")
			if where == "entry" {
				f.createEntry = func(ctx context.Context, p string, pin gymentry.Pin, src *gymstable.Store) (*gymentry.Store, error) {
					s, e := gymentry.Create(ctx, p, pin, src)
					if e != nil {
						return nil, e
					}
					return nil, errors.Join(lost, s.Close())
				}
			}
			var created int64
			if where == "event" {
				f.createEvent = func(ctx context.Context, p string, pin gymevent.Pin) (*gymevent.Store, error) {
					s, e := gymevent.Create(ctx, p, pin)
					if e != nil {
						return nil, e
					}
					v, e := s.Info(ctx)
					created = v.CreatedAtMS
					return nil, errors.Join(lost, e, s.Close())
				}
			}
			r.leagueFactories = f
			if e := r.prepareLeague(context.Background()); !errors.Is(e, lost) {
				t.Fatal("missing injected uncertainty", e)
			}
			if r.league != nil || r.events != nil {
				t.Fatal("uncertain setup published")
			}
			r.leagueFactories = defaultPoolLeagueFactories()
			if e := r.prepareLeague(context.Background()); e != nil {
				t.Fatal("retry failed", e)
			}
			if created != 0 && r.league.EventCreatedMS != created {
				t.Fatal("lost ack changed event creation time")
			}
		})
	}
}

func TestPoolLeagueEstablishedLossNeverRecreates(t *testing.T) {
	for _, component := range []string{"stable", "entries", "events", "participant-ready"} {
		t.Run(component, func(t *testing.T) {
			r := poolLeagueTest(t)
			if e := r.prepareLeague(context.Background()); e != nil {
				t.Fatal(e)
			}
			p := r.league.Intent.Participants[0]
			poolLeagueReleaseOwner(t, r)
			root := filepath.Join(r.config.DataDir, poolLeagueDirectory)
			path := filepath.Join(r.config.DataDir, "gyms", p.AccountID, component)
			if component == "events" {
				path = filepath.Join(root, "events")
			}
			if component == "participant-ready" {
				path = filepath.Join(root, p.AccountID+"-READY.json")
			}
			// Move only this owned test artifact out of the configured namespace.
			if e := os.Rename(path, filepath.Join(t.TempDir(), "retained")); e != nil {
				t.Fatal(e)
			}
			if e := r.prepareLeague(context.Background()); e == nil {
				t.Fatal("lost established component accepted")
			}
			if _, e := os.Lstat(path); !os.IsNotExist(e) {
				t.Fatal("lost component recreated", e)
			}
		})
	}
}

func TestPoolLeagueIntentCannotRepinOrAdoptOrphans(t *testing.T) {
	for _, kind := range []string{"foreign-source", "swapped-account", "orphan-ready", "symlink", "nonempty-incomplete"} {
		t.Run(kind, func(t *testing.T) {
			r := poolLeagueTest(t)
			root := filepath.Join(r.config.DataDir, poolLeagueDirectory)
			if e := os.Mkdir(root, 0700); e != nil {
				t.Fatal(e)
			}
			intent, e := r.leagueIntent(nil)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "foreign-source":
				intent.SourceBindingID = strings.Repeat("f", 64)
			case "swapped-account":
				intent.Participants[0].EntryPin.StablePin.AccountID = intent.Participants[1].AccountID
			case "orphan-ready":
				if e = os.WriteFile(filepath.Join(root, poolLeagueReadyName), []byte("{}\n"), 0600); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				if e = os.Symlink(t.TempDir(), filepath.Join(root, "events")); e != nil {
					t.Fatal(e)
				}
			case "nonempty-incomplete":
				p := filepath.Join(r.config.DataDir, "gyms", intent.Participants[0].AccountID, "stable")
				if e = os.Mkdir(p, 0700); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(p, "partial"), []byte("retained incomplete initialization"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if kind != "orphan-ready" {
				raw, _ := json.Marshal(intent)
				if e = os.WriteFile(filepath.Join(root, poolLeagueIntentName), append(raw, '\n'), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if e = r.prepareLeague(context.Background()); e == nil {
				t.Fatal("invalid initialization adopted")
			}
			if r.league != nil || r.events != nil {
				t.Fatal("invalid setup published")
			}
		})
	}
}

func TestPoolLeagueExistingStableRecordedBeforeCreation(t *testing.T) {
	r := poolLeagueTest(t)
	b := r.recipients["zero256"]
	path := filepath.Join(r.config.DataDir, "gyms", b.Config.AccountID, "stable")
	s, e := gymstable.CreateShared(path, poolStablePin(b), r.issuer)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	intent, e := r.leagueIntent(nil)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, p := range intent.Participants {
		if p.AccountID == b.Config.AccountID {
			found = p.StableExisting
		}
	}
	if !found {
		t.Fatal("existing history not pinned")
	}
	root := filepath.Join(r.config.DataDir, poolLeagueDirectory)
	if e = os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	if e = writePoolRuntimeRecord(filepath.Join(root, poolLeagueIntentName), intent); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(path, filepath.Join(t.TempDir(), "retained")); e != nil {
		t.Fatal(e)
	}
	if e = r.prepareLeague(context.Background()); e == nil {
		t.Fatal("existing stable loss recreated before READY")
	}
}

func TestPoolLeagueCloseJoinsBorrowerBeforeEventOwner(t *testing.T) {
	r := poolLeagueTest(t)
	if e := r.prepareLeague(context.Background()); e != nil {
		t.Fatal(e)
	}
	events := r.events
	b := &poolRuntimeBorrow{account: "public fixture", done: make(chan struct{})}
	b.close = func() error {
		if _, e := events.Info(context.Background()); e != nil {
			t.Error("event owner closed before borrower", e)
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
	if _, e := events.Info(context.Background()); e == nil {
		t.Fatal("event owner retained after runtime close")
	}
}
