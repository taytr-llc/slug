// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

var serverTokenA = strings.Repeat("1", 64)
var serverTokenB = strings.Repeat("2", 64)
var serverTokenRenewed = strings.Repeat("3", 64)

// All scalar fixtures are public and have no production value. This source
// template is independently frozen; freshly issued work is actually checked
// when a test needs a share, rather than importing a fabricated acceptance.
func serverFixtureKey(n byte) (string, string, string) {
	var scalar [32]byte
	scalar[31] = n
	_, pub := btcec.PrivKeyFromBytes(scalar[:])
	h := address.Hash160(pub.SerializeCompressed())
	a, _ := address.NewAddressWitnessPubKeyHash(h, &chaincfg.MainNetParams)
	return hex.EncodeToString(pub.SerializeCompressed()), hex.EncodeToString(append([]byte{0, 20}, h...)), a.EncodeAddress()
}

type serverSessions struct {
	lease   sync.Mutex
	mu      sync.Mutex
	keys    map[string]string
	revoked map[string]bool
	nested  atomic.Int32
	swallow bool
}

func (s *serverSessions) revoke(token string) { s.mu.Lock(); s.revoked[token] = true; s.mu.Unlock() }
func (s *serverSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if !s.lease.TryLock() {
		s.nested.Add(1)
		return errors.New("nested fixture lease")
	}
	defer s.lease.Unlock()
	s.mu.Lock()
	key, ok := s.keys[token]
	revoked := s.revoked[token]
	swallow := s.swallow
	s.mu.Unlock()
	if !ok || revoked {
		return errors.New("fixture unauthorized")
	}
	check := func() error {
		s.mu.Lock()
		revoked := s.revoked[token]
		s.mu.Unlock()
		if revoked {
			return errors.New("fixture revoked")
		}
		return ctx.Err()
	}
	err := fn(key, check)
	if swallow {
		return nil
	}
	return err
}

type serverSourceNode struct {
	template       prospectwork.Template
	resolveEntered chan struct{}
	resolveRelease chan struct{}
	onSubmit       func()
	resolveCalls   atomic.Int32
}

func (n *serverSourceNode) ResolveTemplate(context.Context, prospectnode.Policy) (prospectwork.Template, error) {
	n.resolveCalls.Add(1)
	if n.resolveEntered != nil {
		select {
		case n.resolveEntered <- struct{}{}:
		default:
		}
		<-n.resolveRelease
	}
	return n.template, nil
}
func (*serverSourceNode) Proposal(context.Context, prospect.Context, string) error { return nil }
func (*serverSourceNode) RecheckContext(context.Context, prospect.Context) error   { return nil }
func (n *serverSourceNode) SubmitBlock(context.Context, prospect.Context, string) error {
	if n.onSubmit != nil {
		n.onSubmit()
	}
	return nil
}
func (*serverSourceNode) CanonicalBlock(context.Context, prospect.Context, prospect.Proof, string) error {
	return nil
}
func (*serverSourceNode) ObservePoolBlock(_ context.Context, c prospect.Context, _ prospect.Proof, h string) (prospectnode.PoolBlockObservation, error) {
	return prospectnode.PoolBlockObservation{BlockHash: h, BlockHeight: c.ParentHeight + 1, TipHash: strings.Repeat("a", 64), TipHeight: c.ParentHeight + 61, Canonical: true, Descendants: 60}, nil
}

type serverNoPayment struct{}

func (serverNoPayment) Inspect(context.Context, []poolsettle.Coin) ([]poolsettle.Funding, error) {
	return nil, errors.New("not a payout test")
}
func (serverNoPayment) EstimateFee(context.Context, []poolsettle.Coin, []poolsettle.Payment, string) (int64, error) {
	return 0, errors.New("not a payout test")
}
func (serverNoPayment) Sign(context.Context, []poolsettle.Coin, []byte) ([]byte, error) {
	return nil, errors.New("not a payout test")
}
func (serverNoPayment) ObserveTransaction(context.Context, string) (poolsettle.TransactionObservation, error) {
	return poolsettle.TransactionObservation{}, errors.New("not a payout test")
}
func (serverNoPayment) Broadcast(context.Context, []byte) (string, error) {
	return "", errors.New("not a payout test")
}

type serverHarness struct {
	server                       *Server
	pool                         *poolshare.Pool
	rounds                       *poolround.Store
	settlement                   *poolsettle.Store
	sessions                     *serverSessions
	node                         *serverSourceNode
	source                       poolshare.Config
	publicA, publicB, gymA, gymB string
}

func newServerHarness(t *testing.T) *serverHarness {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal-PoW concrete source; refusal tested separately")
	}
	raw, e := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("independent template fixture changed")
	}
	var corpus struct {
		Sources []struct {
			Name     string                `json:"name"`
			Template prospectwork.Template `json:"template"`
		} `json:"sources"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("invalid frozen template")
	}
	var template prospectwork.Template
	for _, s := range corpus.Sources {
		if s.Name == "coinbase-only" {
			template = s.Template
		}
	}
	if template.Context.Version == 0 {
		t.Fatal("missing frozen source")
	}
	descriptor, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if e != nil {
		t.Fatal(e)
	}
	issuer, e := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), template.Context.Network, template.Context.Genesis, descriptor)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := issuer.Close(); e != nil {
			t.Error(e)
		}
	})
	info, e := issuer.Info()
	if e != nil {
		t.Fatal(e)
	}
	publicA, _, _ := serverFixtureKey(1)
	publicB, _, _ := serverFixtureKey(2)
	_, poolScript, poolAddress := serverFixtureKey(3)
	_, destA, _ := serverFixtureKey(4)
	_, destB, _ := serverFixtureKey(5)
	ctx := template.Context
	gymA, e := gymauth.GymID(ctx, publicA)
	if e != nil {
		t.Fatal(e)
	}
	gymB, e := gymauth.GymID(ctx, publicB)
	if e != nil {
		t.Fatal(e)
	}
	sessions := &serverSessions{keys: map[string]string{serverTokenA: publicA, serverTokenB: publicB, serverTokenRenewed: publicA}, revoked: make(map[string]bool)}
	node := &serverSourceNode{template: template}
	config := poolshare.Config{Version: 1, AuthorityPin: info.Pin, Policy: prospectnode.Policy{Version: 1, Network: ctx.Network, Genesis: ctx.Genesis, DNAVersion: ctx.DNAVersion, ProspectTarget: ctx.ProspectTarget}, PayoutAddress: poolAddress, PoolTarget: strings.Repeat("f", 64), MaxWorks: 16, MaxSubmissions: 16, MaxBytes: 16 << 20}
	pool, e := poolshare.Create(filepath.Join(t.TempDir(), "pool"), config, issuer, node, sessions)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := pool.Close(); e != nil {
			t.Error(e)
		}
	})
	rounds, e := poolround.Create(filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: 1, MaxRounds: 16, MaxBytes: 16 << 20}, pool, node)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := rounds.Close(); e != nil {
			t.Error(e)
		}
	})
	dests := []poolsettle.Destination{{GymID: gymA, Script: destA}, {GymID: gymB, Script: destB}}
	sort.Slice(dests, func(i, j int) bool { return dests[i].GymID < dests[j].GymID })
	settlement, e := poolsettle.Create(filepath.Join(t.TempDir(), "settlement"), poolsettle.Config{Version: 1, ChangeScript: poolScript, Destinations: dests, MaxRounds: 16, MaxBatches: 4}, rounds, serverNoPayment{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := settlement.Close(); e != nil {
			t.Error(e)
		}
	})
	server, e := NewServer(ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, pool, settlement, sessions)
	if e != nil {
		t.Fatal(e)
	}
	return &serverHarness{server: server, pool: pool, rounds: rounds, settlement: settlement, sessions: sessions, node: node, source: config, publicA: publicA, publicB: publicB, gymA: gymA, gymB: gymB}
}
