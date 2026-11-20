//go:build !slugtestpow

package pilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospectnode"
)

// Real normal-genesis replay over a concrete HTTP public reader and original
// node, without a wallet fixture or arbitrary successful verification callback.
type restoredRefreshBlock struct {
	arm     atomic.Bool
	entered chan struct{}
}

func restoredRefreshFixture(t *testing.T) (*RestoredGymSession, *atomic.Uint64, *restoredRefreshBlock) {
	t.Helper()
	runtime := ownedSourceRuntimeTest(t)
	if e := runtime.openPoolIssuer(false); e != nil {
		t.Fatal(e)
	}
	o := runtime.owned
	var e error
	o.publisher, e = runtime.issuer.PublicPublication(o.config.Ownership)
	if e != nil {
		t.Fatal(e)
	}
	o.store, e = fighterownership.Create(filepath.Join(runtime.config.DataDir, "refresh-source"), o.config.Ownership, o.config.Store)
	if e != nil {
		t.Fatal(e)
	}
	var block, header bytes.Buffer
	if e = chaincfg.MainNetParams.GenesisBlock.Serialize(&block); e != nil {
		t.Fatal(e)
	}
	if e = chaincfg.MainNetParams.GenesisBlock.Header.Serialize(&header); e != nil {
		t.Fatal(e)
	}
	blocker := &restoredRefreshBlock{entered: make(chan struct{})}
	revision := new(atomic.Uint64)
	revision.Store(1)
	nodeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.NewDecoder(r.Body).Decode(&call) != nil {
			http.Error(w, "request", 400)
			return
		}
		var result any
		switch call.Method {
		case "getcurrentnet":
			result = o.config.Ownership.Source.Network
		case "getblockhash":
			result = o.config.Ownership.Source.Genesis
		case "getchainobservation":
			result = fighterownership.ChainObservation{Version: 1, InstanceID: strings.Repeat("73", 32), Revision: strconv.FormatUint(revision.Load(), 10), Hash: o.config.Ownership.Source.Genesis, Height: 0}
		case "getblockheader":
			result = hex.EncodeToString(header.Bytes())
		case "getblock":
			if blocker.arm.Swap(false) {
				close(blocker.entered)
				<-r.Context().Done()
				return
			}
			result = hex.EncodeToString(block.Bytes())
		default:
			http.Error(w, "unexpected", 400)
			return
		}
		json.NewEncoder(w).Encode(struct {
			Result any    `json:"result"`
			Error  any    `json:"error"`
			ID     uint64 `json:"id"`
		}{result, nil, call.ID})
	}))
	t.Cleanup(nodeServer.Close)
	life, cancel := context.WithCancel(context.Background())
	node, e := fighterownership.NewOwnedChainSource(life, o.config.Ownership, prospectnode.RPCConfig{Version: 1, URL: nodeServer.URL, Username: "fixture", Password: "fixture"}, life)
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	public, e := fighterownership.NewPublicReadServer(o.config.Ownership, o.publisher, o.store, life)
	if e != nil {
		node.Close()
		cancel()
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := public.Close(); e != nil {
			t.Error(e)
		}
	})
	server := httptest.NewServer(public)
	t.Cleanup(server.Close)
	root, e := openRestoredRoot(filepath.Join(runtime.config.DataDir, "refresh-consumer"), true)
	if e != nil {
		node.Close()
		cancel()
		t.Fatal(e)
	}
	s := &RestoredGymSession{life: life, cancel: cancel, gate: make(chan struct{}, 1), root: root, initial: restoredSetupInit{Ownership: o.config.Ownership}, node: node}
	s.store, e = fighterownership.Create(filepath.Join(runtime.config.DataDir, "refresh-consumer", "ownership"), o.config.Ownership, fighterownership.StoreIdentity{StoreID: strings.Repeat("79", 32)})
	if e != nil {
		s.Close()
		t.Fatal(e)
	}
	s.reader, e = fighterownership.NewPublicReader(life, o.config.Ownership, fighterownership.PublicReadConfig{URL: server.URL + "/slug/ownership/v1"}, life)
	if e != nil {
		s.Close()
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	return s, revision, blocker
}

// Public BeginRefresh additionally connects the concrete installed wallet. This
// fixture enters its same private begin leaf after constructing real reader,
// node and store; the existing actual harness covers the full wallet connection.
func beginRestoredRefreshTest(t *testing.T, s *RestoredGymSession, original context.Context) fighterownership.Progress {
	t.Helper()
	work, end, e := s.acquire(original, 30*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.beginRefresh(work, original)
	end()
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestRestoredRefreshRetainsOneReplayAcrossBoundedSlices(t *testing.T) {
	s, _, _ := restoredRefreshFixture(t)
	original, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := beginRestoredRefreshTest(t, s, original)
	if p.WorkID == "" || p.Ready || p.Head != nil {
		t.Fatal("begin granted ready authority")
	}
	owner := s.refresh
	if _, e := s.Fighters(context.Background(), fighterownership.HeadRef{}, 0); !errors.Is(e, fightercarrier.ErrBusy) {
		t.Fatal("ordinary operation entered incomplete replay", e)
	}
	if _, e := s.BeginRefresh(original); !errors.Is(e, fightercarrier.ErrBusy) {
		t.Fatal("second replay owner accepted", e)
	}
	if _, e := s.RefreshStep(original, 0); !errors.Is(e, fightercarrier.ErrInvalid) {
		t.Fatal("zero step bound", e)
	}
	if _, e := s.RefreshStep(original, 129); !errors.Is(e, fightercarrier.ErrInvalid) {
		t.Fatal("step cap", e)
	}
	slices := 0
	for ; slices < 32 && !p.Ready; slices++ {
		var e error
		p, e = s.RefreshStep(original, 1)
		if e != nil {
			t.Fatal("normal bounded slice", slices, e)
		}
		if p.WorkID != owner.replay.WorkID() {
			t.Fatal("replaced original work")
		}
		if !p.Ready && (p.Head != nil || s.refresh != owner) {
			t.Fatal("incomplete head or lost concrete replay")
		}
	}
	if !p.Ready || p.Head == nil || p.Head.ChainHeight != 0 || p.Head.ChainHash != chaincfg.MainNetParams.GenesisHash.String() || slices < 3 || s.refresh != nil {
		t.Fatal("bounded real replay did not finish", slices, p)
	}
	select {
	case <-owner.joined:
	default:
		t.Fatal("completion did not join replay")
	}
	if original.Err() != nil {
		t.Fatal("completion cancelled original caller")
	}
	head := *p.Head
	p.Head.ChainHash = strings.Repeat("ff", 32)
	got, e := s.store.Head(original)
	if e != nil || got != head {
		t.Fatal("disclosed head aliases retained authority", e)
	}
	if e = s.CancelRefresh(original); e != nil {
		t.Fatal("completed cancel not idempotent", e)
	}
}
func TestRestoredRefreshOriginalCancelAndCloseJoin(t *testing.T) {
	for _, mode := range []string{"original", "explicit", "session"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := restoredRefreshFixture(t)
			original, cancel := context.WithCancel(context.Background())
			defer cancel()
			start := beginRestoredRefreshTest(t, s, original)
			owner := s.refresh
			if _, e := s.RefreshStep(original, 1); e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "original":
				cancel()
			case "explicit":
				if e := s.CancelRefresh(original); e != nil {
					t.Fatal(e)
				}
			case "session":
				if e := s.Close(); e != nil {
					t.Fatal(e)
				}
			}
			select {
			case <-owner.joined:
			case <-time.After(time.Second):
				t.Fatal("cancel did not join exact replay")
			}
			if mode == "original" {
				p, e := s.RefreshStep(context.Background(), 1)
				if !errors.Is(e, context.Canceled) || p.Ready || p.Head != nil || s.refresh != nil {
					t.Fatal("new caller rescued cancelled work", e)
				}
			}
			if mode != "session" {
				saved, e := s.store.InspectReplay(context.Background())
				if e != nil || saved.WorkID != start.WorkID || saved.Ready {
					t.Fatal("cancel discarded durable exact work", e)
				}
				if _, e = s.store.Head(context.Background()); e == nil {
					t.Fatal("cancelled partial replay became ready")
				}
			}
		})
	}
}
func TestRestoredRefreshChangedOriginalNodeRefuses(t *testing.T) {
	s, revision, _ := restoredRefreshFixture(t)
	ctx := context.Background()
	start := beginRestoredRefreshTest(t, s, ctx)
	owner := s.refresh
	if _, e := s.RefreshStep(ctx, 1); e != nil {
		t.Fatal(e)
	}
	revision.Store(2)
	p, e := s.RefreshStep(ctx, 1)
	if e == nil || p.Ready || p.Head != nil || s.refresh != nil {
		t.Fatal("changed original observation accepted", e)
	}
	select {
	case <-owner.joined:
	default:
		t.Fatal("failed original replay not joined")
	}
	saved, e := s.store.InspectReplay(ctx)
	if e != nil || saved.WorkID != start.WorkID {
		t.Fatal("failure erased exact retained work", e)
	}
}

func TestRestoredRefreshOriginalSourceClosureRefuses(t *testing.T) {
	s, _, _ := restoredRefreshFixture(t)
	ctx := context.Background()
	start := beginRestoredRefreshTest(t, s, ctx)
	owner := s.refresh
	if _, e := s.RefreshStep(ctx, 1); e != nil {
		t.Fatal(e)
	}
	if e := s.reader.Close(); e != nil {
		t.Fatal(e)
	}
	p, e := s.RefreshStep(ctx, 1)
	if e == nil || p.Ready || p.Head != nil || s.refresh != nil {
		t.Fatal("closed original reader silently replaced", e)
	}
	select {
	case <-owner.joined:
	default:
		t.Fatal("closed-source replay not joined")
	}
	saved, e := s.store.InspectReplay(ctx)
	if e != nil || saved.WorkID != start.WorkID {
		t.Fatal("original source failure erased correlation", e)
	}
}

func TestRestoredRefreshCloseJoinsActiveSlice(t *testing.T) {
	s, _, blocker := restoredRefreshFixture(t)
	ctx := context.Background()
	beginRestoredRefreshTest(t, s, ctx)
	owner := s.refresh
	blocker.arm.Store(true)
	stepDone := make(chan error, 1)
	go func() {
		for n := 0; n < 32; n++ {
			p, e := s.RefreshStep(ctx, 1)
			if e != nil {
				stepDone <- e
				return
			}
			if p.Ready {
				stepDone <- nil
				return
			}
		}
		stepDone <- errors.New("finite test slice limit")
	}()
	select {
	case <-blocker.entered:
	case e := <-stepDone:
		t.Fatal("never entered blocked concrete read", e)
	case <-time.After(5 * time.Second):
		t.Fatal("bounded genesis fixture did not reach block")
	}
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case e := <-stepDone:
		if e == nil {
			t.Fatal("cancelled slice disclosed completion")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("active slice not joined")
	}
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not join retained replay")
	}
	select {
	case <-owner.joined:
	default:
		t.Fatal("callback remained live after session close")
	}
}
