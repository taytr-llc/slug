//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

type sharedOwnerFixture struct {
	binding               SharedPoolOwnerBinding
	cancel                context.CancelFunc
	primaryDone, peerDone chan struct{}
	calls                 atomic.Int64
	mu                    sync.Mutex
	hook                  func(int, string)
}

// Real empty authorities and marker/config files are combined with explicitly
// synthetic process/RPC boundaries. This is not evidence of a launched chain,
// mined funds, block acceptance or finality; the actual acceptance covers those.
func newSharedOwnerFixture(t *testing.T) *sharedOwnerFixture {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := gymaccount.Create(filepath.Join(home, "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture", Subject: "a", Fixture: "zero256"}, {Issuer: "fixture", Subject: "b", Fixture: "incrementing256"}}, MaxSessions: 8, MaxChallenges: 8, MaxEvents: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = accounts.Close() })
	descriptor, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	issuer, err := registry.CreateManifestAuthority(filepath.Join(home, "issuer"), gymaccount.FixtureNetwork, gymaccount.FixtureGenesis, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = issuer.Close() })
	info, err := issuer.Info()
	if err != nil {
		t.Fatal(err)
	}
	source := poolshare.Config{Version: 1, AuthorityPin: info.Pin, Policy: prospectnode.Policy{Version: 1, Network: gymaccount.FixtureNetwork, Genesis: gymaccount.FixtureGenesis, DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}, PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 32, MaxBytes: 1 << 20}
	pin := gymevent.Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.RulesID(), Participants: []gymentry.Pin{}}
	for i, name := range []string{"zero256", "incrementing256"} {
		identity, _ := gymaccount.FixtureIdentity(name)
		sp := gymstable.Pin{Version: 2, AccountAuthorityID: accounts.AuthorityID(), AccountID: strings.Repeat([]string{"a1", "b2"}[i], 32), Identity: identity, Source: info.Pin, ConditionVersion: fightercondition.Version1, ProjectorID: fightercondition.ProjectorID}
		sid, err := gymstable.ID(sp)
		if err != nil {
			t.Fatal(err)
		}
		ns, _ := gymentry.NamespaceID(sp.Source.Network, sp.Source.Genesis, sp.AccountAuthorityID)
		pin.Participants = append(pin.Participants, gymentry.Pin{Version: 1, NamespaceID: ns, CalendarID: gymentry.CalendarID(), RulesID: gymentry.RulesID(), StableStoreID: sid, StablePin: sp})
	}
	sort.Slice(pin.Participants, func(i, j int) bool {
		return pin.Participants[i].StablePin.Identity.GymID < pin.Participants[j].StablePin.Identity.GymID
	})
	if err = os.Mkdir(filepath.Join(home, "league-v1"), 0700); err != nil {
		t.Fatal(err)
	}
	events, err := gymevent.Create(context.Background(), filepath.Join(home, "league-v1", "events"), pin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = events.Close() })
	ei, err := events.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	life, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h := &sharedOwnerFixture{cancel: cancel, primaryDone: make(chan struct{}), peerDone: make(chan struct{})}
	h.binding = SharedPoolOwnerBinding{Source: pin, EventStoreID: ei.StoreID, EventCreatedAtMS: ei.CreatedAtMS, Pool: source, Accounts: accounts, Issuer: issuer, Events: events, Home: home, Life: life}
	h.binding.PoolBindingID, err = poolservice.BindingID(source)
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) string {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		if err = os.WriteFile(filepath.Join(home, name), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(digest[:])
	}
	h.binding.RuntimeMarkerSHA256 = write("SLUG-POOL-RUNTIME.json", map[string]any{"application": "slug-shared-pool-runtime", "account_authority_id": accounts.AuthorityID(), "version": 1})
	h.binding.LeagueIntentSHA256 = write("league-v1/SLUG-POOL-LEAGUE-INTENT.json", map[string]any{"event_pin": pin, "source_binding_id": h.binding.PoolBindingID})
	h.binding.LeagueReadySHA256 = write("league-v1/SLUG-POOL-LEAGUE-READY.json", ei)
	write("SLUG-POOL-SOURCE.json", source)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"primary", "peer"} {
		index := i
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var q resultRPCRequest
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				t.Error(err)
				return
			}
			h.calls.Add(1)
			h.mu.Lock()
			hook := h.hook
			h.mu.Unlock()
			if hook != nil {
				hook(index, q.Method)
			}
			var value any
			switch q.Method {
			case "getcurrentnet":
				value = gymaccount.FixtureNetwork
			case "getblockhash":
				value = gymaccount.FixtureGenesis
			case "getbestblock":
				value = Tip{Hash: gymaccount.FixtureGenesis}
			case "getblockchaininfo":
				deps := map[string]any{}
				for _, n := range []string{"csv", "segwit", "taproot"} {
					deps[n] = map[string]any{"status": "defined", "always_active_height": 62}
				}
				value = map[string]any{"chain": "mainnet", "blocks": 0, "bestblockhash": gymaccount.FixtureGenesis, "bip9_softforks": deps}
			case "getpeerinfo":
				value = []any{map[string]any{"addr": "127.0.0.1:1", "inbound": index == 1}}
			case "gettxout":
				value = nil
			default:
				t.Errorf("unexpected publication RPC %s", q.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "1.0", "id": q.ID, "result": value, "error": nil})
		}))
		t.Cleanup(server.Close)
		for _, suffix := range []string{"-chain", "-logs"} {
			if err = os.Mkdir(filepath.Join(home, name+suffix), 0700); err != nil {
				t.Fatal(err)
			}
		}
		connect, tail, done := "127.0.0.1:1", "nolisten=1\ntxindex=1\n", h.primaryDone
		if i == 1 {
			connect, tail, done = "127.0.0.1:2", "listen=127.0.0.1:1\n", h.peerDone
		}
		path := filepath.Join(home, name+".conf")
		config := "rpcuser=local-pilot\nrpcpass=" + strings.Repeat("01", 32) + "\nnotls=1\nrpclisten=" + strings.TrimPrefix(server.URL, "http://") + "\ndatadir=" + filepath.Join(home, name+"-chain") + "\nlogdir=" + filepath.Join(home, name+"-logs") + "\nnodnsseed=1\nlocalwalletpilot=1\nconnect=" + connect + "\n" + tail
		if err = os.WriteFile(path, []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		proc := OwnedProcess{Cmd: &exec.Cmd{Path: binary, Args: []string{binary, "--configfile=" + path}, Process: &os.Process{Pid: os.Getpid() + i}}, Done: done}
		if i == 0 {
			h.binding.Primary = proc
		} else {
			h.binding.Peer = proc
		}
	}
	return h
}

func (h *sharedOwnerFixture) setHook(hook func(int, string)) {
	h.mu.Lock()
	h.hook = hook
	h.mu.Unlock()
}

func TestSharedPublisherConcreteIdentityAndPurpose(t *testing.T) {
	h := newSharedOwnerFixture(t)
	p, err := AttachSharedPool(context.Background(), h.binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	want, err := ChainIdentityFor(h.binding.Source)
	if err != nil || p.Identity() != want {
		t.Fatal("different chain identity", err)
	}
	deadline, ok := p.ctx.Deadline()
	if !ok || time.Until(deadline) > 30*time.Second {
		t.Fatal("unbounded borrow")
	}
	if _, err = AttachOwned(context.Background(), p.binding); err == nil {
		t.Fatal("solo constructor accepted shared runtime")
	}
	r, err := AttachSharedPoolResult(context.Background(), h.binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	key1, script1, _ := fundingKey()
	key2, script2, _ := resultFundingKey()
	if r.Identity().Chain != want || r.Identity().FundingPublicKey != hex.EncodeToString(key2.PubKey().SerializeCompressed()) || r.Identity().FundingScript != hex.EncodeToString(script2) || bytes.Equal(script1, script2) || hex.EncodeToString(script1) == poolsettlefixture.PoolScript() || hex.EncodeToString(script2) == poolsettlefixture.PoolScript() || key1.PubKey().IsEqual(key2.PubKey()) {
		t.Fatal("funding purpose changed")
	}
	// Mutating the supplied participant slice cannot retarget the retained borrow.
	h.binding.Source.Participants[0].StablePin.AccountID = strings.Repeat("e3", 32)
	if _, err = p.Observe(context.Background()); err != nil {
		t.Fatal("caller input aliased borrow", err)
	}
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	if !live(h.binding.Primary) || !live(h.binding.Peer) {
		t.Fatal("borrow closed owner processes")
	}
	if _, err = h.binding.Issuer.Info(); err != nil {
		t.Fatal("borrow closed issuer", err)
	}
}

func TestSharedPublisherRejectsForeignBindingsBeforeRPC(t *testing.T) {
	h := newSharedOwnerFixture(t)
	cases := []struct {
		name   string
		change func(*SharedPoolOwnerBinding)
	}{
		{"issuer_nil", func(b *SharedPoolOwnerBinding) { b.Issuer = nil }},
		{"events_nil", func(b *SharedPoolOwnerBinding) { b.Events = nil }},
		{"accounts_nil", func(b *SharedPoolOwnerBinding) { b.Accounts = nil }},
		{"runtime_nil", func(b *SharedPoolOwnerBinding) { b.Life = nil }},
		{"pin1", func(b *SharedPoolOwnerBinding) { b.Source.Participants[0].StablePin.Version = 1 }},
		{"foreign_source", func(b *SharedPoolOwnerBinding) {
			b.Source.Participants[0].StablePin.Source.AuthorityID = strings.Repeat("e3", 32)
		}},
		{"source_caps", func(b *SharedPoolOwnerBinding) { b.Pool.MaxWorks++ }},
		{"store_id", func(b *SharedPoolOwnerBinding) { b.EventStoreID = strings.Repeat("e3", 32) }},
		{"event_time", func(b *SharedPoolOwnerBinding) { b.EventCreatedAtMS++ }},
		{"marker", func(b *SharedPoolOwnerBinding) { b.RuntimeMarkerSHA256 = strings.Repeat("e3", 32) }},
		{"same_process", func(b *SharedPoolOwnerBinding) { b.Peer = b.Primary }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := h.binding
			b.Source.Participants = append([]gymentry.Pin(nil), b.Source.Participants...)
			c.change(&b)
			before := h.calls.Load()
			if p, err := AttachSharedPool(context.Background(), b); err == nil {
				_ = p.Close()
				t.Fatal("foreign binding accepted")
			}
			if h.calls.Load() != before {
				t.Fatal("invalid owner reached node RPC")
			}
		})
	}
}

func TestSharedPublisherRechecksEveryReadAndOwnerClosure(t *testing.T) {
	for _, cause := range []string{"runtime_marker", "league_intent", "league_ready", "source", "config", "issuer", "events", "life", "primary", "peer"} {
		t.Run(cause, func(t *testing.T) {
			h := newSharedOwnerFixture(t)
			p, err := AttachSharedPool(context.Background(), h.binding)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Close() })
			switch cause {
			case "issuer":
				_ = h.binding.Issuer.Close()
			case "events":
				_ = h.binding.Events.Close()
			case "life":
				h.cancel()
			case "primary":
				close(h.primaryDone)
			case "peer":
				close(h.peerDone)
			default:
				path := map[string]string{"runtime_marker": "SLUG-POOL-RUNTIME.json", "league_intent": "league-v1/SLUG-POOL-LEAGUE-INTENT.json", "league_ready": "league-v1/SLUG-POOL-LEAGUE-READY.json", "source": "SLUG-POOL-SOURCE.json", "config": "primary.conf"}[cause]
				if err = os.WriteFile(filepath.Join(h.binding.Home, path), []byte("{}\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := h.calls.Load()
			if hash, err := p.CanonicalHash(context.Background(), 0); err == nil || hash != "" {
				t.Fatal("stale owner disclosed canonical hash", err)
			}
			if out, err := p.Unspent(context.Background(), Outpoint{TxID: strings.Repeat("01", 32)}); err == nil || out != nil {
				t.Fatal("stale owner disclosed output", err)
			}
			if h.calls.Load() != before {
				t.Fatal("stale owner reached RPC")
			}
		})
	}
}

func TestSharedPublisherRefusesPostRPCSubstitution(t *testing.T) {
	h := newSharedOwnerFixture(t)
	p, err := AttachSharedPool(context.Background(), h.binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	var once sync.Once
	h.setHook(func(_ int, method string) {
		if method == "getblockhash" {
			once.Do(func() { _ = h.binding.Events.Close() })
		}
	})
	if hash, err := p.CanonicalHash(context.Background(), 0); err == nil || hash != "" {
		t.Fatal("disclosed result after event closure", err)
	}
}

func TestSharedPublisherConfigModeIsExact(t *testing.T) {
	h := newSharedOwnerFixture(t)
	if _, err := configFor(h.binding.Home, "primary", h.binding.Primary); err == nil {
		t.Fatal("solo config accepted txindex")
	}
	path := filepath.Join(h.binding.Home, "primary.conf")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []string{strings.ReplaceAll(string(raw), "txindex=1\n", ""), strings.ReplaceAll(string(raw), "txindex=1\n", "txindex=0\n"), string(raw) + "miningaddr=bad\n", string(raw) + "txindex=1\n"} {
		if err = os.WriteFile(path, []byte(changed), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = configForMode(h.binding.Home, "primary", h.binding.Primary, true); err == nil {
			t.Fatal("shared config accepted widened flags")
		}
	}
}

func TestSharedPublisherCloseCancelsPendingRPC(t *testing.T) {
	h := newSharedOwnerFixture(t)
	p, err := AttachSharedPool(context.Background(), h.binding)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h.setHook(func(_ int, method string) {
		if method == "getblockhash" {
			once.Do(func() { close(entered); <-release })
		}
	})
	done := make(chan error, 1)
	go func() { _, err := p.CanonicalHash(context.Background(), 0); done <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		_ = p.Close()
		t.Fatal("RPC did not begin")
	}
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("borrow Close did not cancel and join RPC")
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("cancelled RPC disclosed hash")
	}
	if _, err = p.CanonicalHash(context.Background(), 0); !errors.Is(err, ErrOffline) {
		t.Fatal("closed borrow resumed", err)
	}
	if !live(h.binding.Primary) || !live(h.binding.Peer) {
		t.Fatal("borrow stopped owner")
	}
}

// Hold the real issuer gate through a rejecting issuance guard. It creates no
// job, samples no issuance time and performs no proof hash.
func holdSharedIssuer(t *testing.T, h *sharedOwnerFixture) func() {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	refused := errors.New("fixture guard refuses issuance")
	c := prospect.Context{Version: 1, Network: gymaccount.FixtureNetwork, Genesis: gymaccount.FixtureGenesis, DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64), ParentHash: gymaccount.FixtureGenesis, ExpectedBits: 0x207fffff, MinTimestamp: 1, MaxTimestamp: 1}
	key := h.binding.Source.Participants[0].StablePin.Identity.GymPublicKey
	go func() {
		_, err := h.binding.Issuer.IssueManifestJob(c, key, func() error { close(entered); <-release; return refused })
		done <- err
	}()
	var once sync.Once
	end := func() {
		once.Do(func() {
			close(release)
			if err := <-done; !errors.Is(err, refused) {
				t.Error("unexpected fixture issuer result", err)
			}
		})
	}
	t.Cleanup(end)
	select {
	case <-entered:
	case <-time.After(time.Second):
		end()
		t.Fatal("issuer guard did not enter")
	}
	return end
}

func TestSharedPublisherWaitsForRealIssuerContention(t *testing.T) {
	h := newSharedOwnerFixture(t)
	p, err := AttachSharedPool(context.Background(), h.binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	release := holdSharedIssuer(t, h)
	done := make(chan error, 1)
	go func() {
		hash, err := p.CanonicalHash(context.Background(), 0)
		if err == nil && hash != gymaccount.FixtureGenesis {
			err = errors.New("wrong canonical hash")
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatal("transient contention escaped before release", err)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("released issuer remained unavailable", err)
		}
	case <-time.After(time.Second):
		t.Fatal("released issuer did not resume bounded read")
	}
	info, err := h.binding.Issuer.TryInfo()
	if err != nil || info.Jobs != 0 || info.Records != 0 {
		t.Fatal("waiting created authority effects", info.Jobs, info.Records, err)
	}
}

func TestSharedPublisherIssuerWaitHonorsEveryLifetime(t *testing.T) {
	for _, kind := range []string{"request", "runtime", "close", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			h := newSharedOwnerFixture(t)
			p, err := AttachSharedPool(context.Background(), h.binding)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Close() })
			release := holdSharedIssuer(t, h)
			ctx, cancel := context.WithCancel(context.Background())
			if kind == "deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				hash, err := p.CanonicalHash(ctx, 0)
				if hash != "" {
					err = errors.New("cancelled issuer wait disclosed hash")
				}
				done <- err
			}()
			if kind != "deadline" {
				select {
				case err := <-done:
					t.Fatal("contention failed before cancellation", err)
				case <-time.After(20 * time.Millisecond):
				}
			}
			switch kind {
			case "request":
				cancel()
			case "runtime":
				h.cancel()
			case "close":
				if err := p.Close(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if !errors.Is(err, ErrOffline) {
					t.Fatal("lifetime did not abort issuer wait", err)
				}
			case <-time.After(time.Second):
				t.Fatal("issuer wait ignored lifetime")
			}
			release()
		})
	}
}

func TestSharedIssuerWaitDoesNotRetrySubstantiveErrors(t *testing.T) {
	for _, cause := range []error{registry.ErrClosed, registry.ErrPoisoned, registry.ErrReadOnly, registry.ErrManifestAuthority} {
		calls := 0
		_, err := awaitSharedIssuer(context.Background(), context.Background(), func() (registry.ManifestAuthorityInfo, error) {
			calls++
			return registry.ManifestAuthorityInfo{}, cause
		})
		if calls != 1 || !errors.Is(err, cause) {
			t.Fatal("substantive error was delayed or rewritten", calls, err)
		}
	}
}
