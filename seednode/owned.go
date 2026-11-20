// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type ownedConfig struct {
	rpc, connect, listen, user, password, path, executable string
	digest                                                 [32]byte
}

// Publisher is a short-lived borrow of the actual owner's fixed node pair.
// It serializes RPC/hash work and never stops or takes ownership of the nodes.
type Publisher struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	watchDone chan struct{}
	binding   OwnerBinding
	shared    *SharedPoolOwnerBinding
	ownership *OwnershipPoolOwnerBinding
	identity  ChainIdentity
	configs   [2]ownedConfig
	nodes     [2]*nodeRPC
	checked   map[string]string
	closed    bool
}

func safeDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n\t") {
		return ErrInvalid
	}
	for p := path; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if e != nil || !s.IsDir() || s.Mode()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return nil
}

func readRegular(path string, limit int64) ([]byte, error) {
	if safeDirectory(filepath.Dir(path)) != nil {
		return nil, ErrInvalid
	}
	s, e := os.Lstat(path)
	if e != nil || !s.Mode().IsRegular() || s.Size() > limit {
		return nil, ErrInvalid
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrInvalid
	}
	defer f.Close()
	n, e := f.Stat()
	if e != nil || !n.Mode().IsRegular() || !os.SameFile(s, n) || n.Size() > limit {
		return nil, ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, ErrInvalid
	}
	return b, nil
}

func loopback(s string) bool {
	h, p, e := net.SplitHostPort(s)
	n, pe := strconv.ParseUint(p, 10, 16)
	return e == nil && pe == nil && h == "127.0.0.1" && n > 0 && p == strconv.FormatUint(n, 10)
}

func live(p OwnedProcess) bool {
	if p.Cmd == nil || p.Cmd.Process == nil || p.Cmd.Process.Pid <= 0 || p.Done == nil {
		return false
	}
	select {
	case <-p.Done:
		return false
	default:
		return true
	}
}

func configFor(home, name string, p OwnedProcess) (ownedConfig, error) {
	return configForMode(home, name, p, false)
}

func configForMode(home, name string, p OwnedProcess, shared bool) (ownedConfig, error) {
	var c ownedConfig
	if !live(p) || safeDirectory(home) != nil {
		return c, ErrOffline
	}
	c.path = filepath.Join(home, name+".conf")
	if len(p.Cmd.Args) != 2 || p.Cmd.Args[0] != p.Cmd.Path || p.Cmd.Args[1] != "--configfile="+c.path || !filepath.IsAbs(p.Cmd.Path) || filepath.Clean(p.Cmd.Path) != p.Cmd.Path || p.Cmd.Stdin != nil {
		return c, ErrInvalid
	}
	s, e := os.Lstat(p.Cmd.Path)
	if e != nil || !s.Mode().IsRegular() || s.Mode()&0111 == 0 {
		return c, ErrInvalid
	}
	c.executable = p.Cmd.Path
	b, e := readRegular(c.path, 8192)
	if e != nil {
		return c, e
	}
	c.digest = sha256.Sum256(b)
	if len(b) == 0 || b[len(b)-1] != '\n' || bytes.ContainsAny(b, "\r\x00") {
		return c, ErrInvalid
	}
	lines := strings.Split(string(b[:len(b)-1]), "\n")
	count := 10
	if shared && name == "primary" {
		count++
	}
	if len(lines) != count {
		return c, ErrInvalid
	}
	m := make(map[string]string, 10)
	for _, line := range lines {
		k, v, ok := strings.Cut(line, "=")
		if !ok || k == "" || v == "" || m[k] != "" {
			return c, ErrInvalid
		}
		m[k] = v
	}
	for k, v := range map[string]string{"rpcuser": "local-pilot", "notls": "1", "datadir": filepath.Join(home, name+"-chain"), "logdir": filepath.Join(home, name+"-logs"), "nodnsseed": "1", "localwalletpilot": "1"} {
		if m[k] != v {
			return c, ErrInvalid
		}
	}
	c.rpc, c.connect, c.user, c.password = m["rpclisten"], m["connect"], m["rpcuser"], m["rpcpass"]
	if !loopback(c.rpc) || !loopback(c.connect) || !hexValue(c.password, 32, false) {
		return c, ErrInvalid
	}
	if name == "primary" {
		if m["nolisten"] != "1" || shared && m["txindex"] != "1" {
			return c, ErrInvalid
		}
	} else {
		c.listen = m["listen"]
		if !loopback(c.listen) {
			return c, ErrInvalid
		}
	}
	if safeDirectory(m["datadir"]) != nil || safeDirectory(m["logdir"]) != nil {
		return c, ErrInvalid
	}
	return c, nil
}

// checkMarker checks the same established source markers the owner validated.
// Marker data remains public identity only; it cannot manufacture ownership.
func checkMarker(b OwnerBinding, id ChainIdentity) error {
	raw, e := readRegular(filepath.Join(b.Home, "SLUG-MANIFEST-PILOT.json"), 8192)
	if e != nil {
		return e
	}
	for _, name := range []string{"SLUG-MANIFEST-PILOT-READY.json", "SLUG-MANIFEST-PILOT-CHAINS.json"} {
		other, e := readRegular(filepath.Join(b.Home, name), 8192)
		if e != nil || !bytes.Equal(raw, other) {
			return ErrInvalid
		}
	}
	m, e := fields(raw, "version", "mode", "phase", "account_authority_id", "account_id", "fixture", "network", "genesis", "activation_height", "recovery_profile", "authority_port", "descriptor_id", "identity", "payout", "authority_pin", "transport_trust_id")
	if e != nil || len(m) != 16 {
		return ErrInvalid
	}
	var v struct {
		Version            uint32 `json:"version"`
		Mode               string `json:"mode"`
		Phase              string `json:"phase"`
		AccountAuthorityID string `json:"account_authority_id"`
		AccountID          string `json:"account_id"`
		Fixture            string `json:"fixture"`
		Network            uint32 `json:"network"`
		Genesis            string `json:"genesis"`
		Activation         uint32 `json:"activation_height"`
		Profile            string `json:"recovery_profile"`
		Port               uint16 `json:"authority_port"`
		Descriptor         string `json:"descriptor_id"`
		Identity           struct {
			RootPublicKey string `json:"root_public_key"`
			GymPublicKey  string `json:"gym_public_key"`
			GymID         string `json:"gym_id"`
		} `json:"identity"`
		Authority registry.ManifestAuthorityPin `json:"authority_pin"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Version != 1 || v.Mode != "local-manifest-pilot" || v.Phase != "ready" || v.Fixture != "zero256" || v.AccountID != id.PublisherAccountID || v.Network != id.Network || v.Genesis != id.Genesis || v.Activation != 62 || v.Port == 0 {
		return ErrInvalid
	}
	zero, _ := gymaccount.FixtureIdentity("zero256")
	for _, entry := range b.Source.Participants {
		p := entry.StablePin
		if p.Identity == zero {
			if p.AccountAuthorityID != v.AccountAuthorityID || p.Identity.Profile != v.Profile || p.Source != v.Authority || p.Source.DescriptorID != v.Descriptor || v.Identity.RootPublicKey != zero.RootPublicKey || v.Identity.GymPublicKey != zero.GymPublicKey || v.Identity.GymID != zero.GymID {
				return ErrInvalid
			}
			return nil
		}
	}
	return ErrInvalid
}

func AttachOwned(ctx context.Context, b OwnerBinding) (*Publisher, error) {
	return attachOwned(ctx, b, nil)
}

func attachOwned(ctx context.Context, b OwnerBinding, shared *SharedPoolOwnerBinding) (*Publisher, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrInvalid
	}
	id, e := ChainIdentityFor(b.Source)
	if e != nil {
		return nil, e
	}
	// Own the validated source slice; the caller retains its input value.
	b.Source, e = gymevent.ValidatePin(b.Source)
	if e != nil {
		return nil, ErrInvalid
	}
	if !live(b.Primary) || !live(b.Peer) || b.Primary.Cmd.Process.Pid == b.Peer.Cmd.Process.Pid {
		return nil, ErrOffline
	}
	if shared == nil {
		if e = checkMarker(b, id); e != nil {
			return nil, e
		}
	}
	return attachNodes(ctx, b, id, shared, nil)
}

// attachNodes owns only process/RPC lifecycle mechanics. A caller must first
// validate exactly one concrete owner family; it cannot create source authority.
func attachNodes(ctx context.Context, b OwnerBinding, id ChainIdentity, shared *SharedPoolOwnerBinding, ownership *OwnershipPoolOwnerBinding) (*Publisher, error) {
	sharedConfig := shared != nil || ownership != nil
	a, e := configForMode(b.Home, "primary", b.Primary, sharedConfig)
	if e != nil {
		return nil, e
	}
	z, e := configForMode(b.Home, "peer", b.Peer, sharedConfig)
	if e != nil {
		return nil, e
	}
	if a.executable != z.executable || a.connect != z.listen {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, v := range []string{a.rpc, z.rpc, z.listen, z.connect} {
		if seen[v] {
			return nil, ErrInvalid
		}
		seen[v] = true
	}
	child, cancel := context.WithTimeout(ctx, 30*time.Second)
	p := &Publisher{ctx: child, cancel: cancel, watchDone: make(chan struct{}), binding: b, shared: shared, ownership: ownership, identity: id, configs: [2]ownedConfig{a, z}, nodes: [2]*nodeRPC{newRPC(a), newRPC(z)}, checked: map[string]string{}}
	var ownerDone <-chan struct{}
	if shared != nil {
		ownerDone = shared.Life.Done()
	}
	if ownership != nil {
		ownerDone = ownership.Life.Done()
	}
	go func() {
		defer close(p.watchDone)
		select {
		case <-b.Primary.Done:
		case <-b.Peer.Done:
		case <-ownerDone:
		case <-child.Done():
		}
		cancel()
	}()
	if _, e = p.Observe(ctx); e != nil {
		p.Close()
		return nil, e
	}
	return p, nil
}

func (p *Publisher) Identity() ChainIdentity {
	if p == nil {
		return ChainIdentity{}
	}
	return p.identity
}
func (p *Publisher) Close() error {
	if p == nil || p.cancel == nil {
		return nil
	}
	p.cancel()
	p.mu.Lock()
	p.closed = true
	for _, r := range p.nodes {
		r.client.CloseIdleConnections()
	}
	p.mu.Unlock()
	<-p.watchDone
	return nil
}
func (p *Publisher) begin(ctx context.Context) (context.Context, func(), error) {
	if p == nil || p.ctx == nil || ctx == nil {
		return nil, nil, ErrOffline
	}
	p.mu.Lock()
	if p.closed || p.ctx.Err() != nil || ctx.Err() != nil || !live(p.binding.Primary) || !live(p.binding.Peer) {
		p.mu.Unlock()
		return nil, nil, ErrOffline
	}
	c, cancel := context.WithCancel(p.ctx)
	stop := context.AfterFunc(ctx, cancel)
	end := func() { stop(); cancel(); p.mu.Unlock() }
	if p.shared != nil || p.ownership != nil {
		if err := p.checkOwnerContext(c); err != nil {
			end()
			return nil, nil, err
		}
	}
	return c, end, nil
}

func hexValue(s string, n int, nonzero bool) bool {
	if len(s) != 2*n || s != strings.ToLower(s) {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && (!nonzero || !bytes.Equal(b, make([]byte, n)))
}
