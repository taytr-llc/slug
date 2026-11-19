// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
)

// These fakes exercise the desktop's orchestration boundary. Their progress is
// an explicit claim, not a scanner/PoW expected vector or a verified chain.
type poolRestoreSteps struct {
	status func(context.Context) (wallet.RecoveryProgress, error)
	step   func(context.Context, wallet.RecoveryBlockSource) (wallet.RecoveryProgress, error)
}

func (s poolRestoreSteps) Status(ctx context.Context) (wallet.RecoveryProgress, error) {
	return s.status(ctx)
}
func (s poolRestoreSteps) Step(ctx context.Context, source wallet.RecoveryBlockSource) (wallet.RecoveryProgress, error) {
	return s.step(ctx, source)
}

type poolRestoreSource struct {
	best func(context.Context) (wallet.RecoveryTip, error)
}

func (s poolRestoreSource) BestBlock(ctx context.Context) (wallet.RecoveryTip, error) {
	return s.best(ctx)
}
func (poolRestoreSource) Identity(context.Context) (wallet.RecoverySourceIdentity, error) {
	return wallet.RecoverySourceIdentity{}, errors.New("unexpected desktop identity request")
}
func (poolRestoreSource) BlockHash(context.Context, int32) (chainhash.Hash, error) {
	return chainhash.Hash{}, errors.New("unexpected desktop block-hash request")
}
func (poolRestoreSource) Block(context.Context, chainhash.Hash) (*wire.MsgBlock, error) {
	return nil, errors.New("unexpected desktop block request")
}

func poolRestoreCompleteProgress() wallet.RecoveryProgress {
	return wallet.RecoveryProgress{
		Version: 1, Phase: "complete", Ready: true, Derived: 160, Target: 160,
		ScannedHeight: 0, TargetHeight: 0,
		ScannedHash: chaincfg.MainNetParams.GenesisHash.String(),
		TargetHash:  chaincfg.MainNetParams.GenesisHash.String(),
	}
}

func TestPoolBackupRestoreCompletionRequiresExactSourceCheckpoint(t *testing.T) {
	for _, name := range []string{"exact_genesis", "same_height_other_hash", "same_hash_other_height", "source_unavailable"} {
		t.Run(name, func(t *testing.T) {
			var order []string
			complete := poolRestoreCompleteProgress()
			source := poolRestoreSource{best: func(context.Context) (wallet.RecoveryTip, error) {
				order = append(order, "best")
				tip := wallet.RecoveryTip{Hash: *chaincfg.MainNetParams.GenesisHash}
				switch name {
				case "same_height_other_hash":
					tip.Hash[0] ^= 1
				case "same_hash_other_height":
					tip.Height++
				case "source_unavailable":
					return wallet.RecoveryTip{}, errors.New("fixture source unavailable")
				}
				return tip, nil
			}}
			session := poolRestoreSteps{
				status: func(context.Context) (wallet.RecoveryProgress, error) {
					order = append(order, "status")
					return wallet.RecoveryProgress{Version: 1, Phase: "deriving", Derived: 128, Target: 160}, nil
				},
				step: func(_ context.Context, got wallet.RecoveryBlockSource) (wallet.RecoveryProgress, error) {
					order = append(order, "step")
					if _, ok := got.(poolRestoreSource); !ok {
						t.Fatal("session received a substituted source")
					}
					return complete, nil
				},
			}
			err := completePoolBackupRecovery(t.Context(), session, source)
			if (err == nil) != (name == "exact_genesis") {
				t.Fatalf("completion accepted wrong source checkpoint: %v", err)
			}
			if !reflect.DeepEqual(order, []string{"status", "step", "best"}) {
				t.Fatal("source tip was not sampled after scanner completion", order)
			}
		})
	}
}

func TestPoolBackupRestoreRejectsIncompleteCheckpointClaims(t *testing.T) {
	mutations := map[string]func(*wallet.RecoveryProgress){
		"version":         func(p *wallet.RecoveryProgress) { p.Version = 2 },
		"derivation_only": func(p *wallet.RecoveryProgress) { p.Phase = "deriving" },
		"inventory":       func(p *wallet.RecoveryProgress) { p.Derived-- },
		"negative_height": func(p *wallet.RecoveryProgress) { p.ScannedHeight, p.TargetHeight = -1, -1 },
		"target_height":   func(p *wallet.RecoveryProgress) { p.TargetHeight++ },
		"target_hash":     func(p *wallet.RecoveryProgress) { p.TargetHash = strings.Repeat("a", 64) },
		"empty_hash":      func(p *wallet.RecoveryProgress) { p.ScannedHash, p.TargetHash = "", "" },
		"zero_hash": func(p *wallet.RecoveryProgress) {
			p.ScannedHash, p.TargetHash = strings.Repeat("0", 64), strings.Repeat("0", 64)
		},
		"noncanonical_hex": func(p *wallet.RecoveryProgress) {
			p.ScannedHash, p.TargetHash = strings.Repeat("A", 64), strings.Repeat("A", 64)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			claim := poolRestoreCompleteProgress()
			mutate(&claim)
			session := poolRestoreSteps{status: func(context.Context) (wallet.RecoveryProgress, error) { return claim, nil }}
			source := poolRestoreSource{best: func(context.Context) (wallet.RecoveryTip, error) {
				t.Fatal("malformed completion reached source I/O")
				return wallet.RecoveryTip{}, nil
			}}
			if completePoolBackupRecovery(t.Context(), session, source) == nil {
				t.Fatal("incomplete scanner claim completed restoration")
			}
		})
	}
}

func TestPoolBackupRestoreDerivationAloneStillSteps(t *testing.T) {
	stop := errors.New("scanner has not completed")
	claim := poolRestoreCompleteProgress()
	claim.Ready, claim.Phase = false, "scanning"
	stepped := false
	session := poolRestoreSteps{
		status: func(context.Context) (wallet.RecoveryProgress, error) { return claim, nil },
		step: func(context.Context, wallet.RecoveryBlockSource) (wallet.RecoveryProgress, error) {
			stepped = true
			return wallet.RecoveryProgress{}, stop
		},
	}
	source := poolRestoreSource{best: func(context.Context) (wallet.RecoveryTip, error) {
		t.Fatal("derivation and matching heights bypassed scanner completion")
		return wallet.RecoveryTip{}, nil
	}}
	if err := completePoolBackupRecovery(t.Context(), session, source); !errors.Is(err, stop) || !stepped {
		t.Fatal("Ready=false was treated as completion", err)
	}
}

func TestPoolBackupRestoreCancellationJoinsStepAndFinalSource(t *testing.T) {
	for _, point := range []string{"step", "source"} {
		t.Run(point, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			entered := make(chan struct{})
			var exited atomic.Bool
			claim := poolRestoreCompleteProgress()
			session := poolRestoreSteps{
				status: func(context.Context) (wallet.RecoveryProgress, error) {
					p := claim
					p.Ready = point == "source"
					return p, nil
				},
				step: func(ctx context.Context, _ wallet.RecoveryBlockSource) (wallet.RecoveryProgress, error) {
					close(entered)
					<-ctx.Done()
					exited.Store(true)
					return wallet.RecoveryProgress{}, ctx.Err()
				},
			}
			source := poolRestoreSource{best: func(ctx context.Context) (wallet.RecoveryTip, error) {
				close(entered)
				<-ctx.Done()
				exited.Store(true)
				// Even a source that races cancellation and returns a matching tip
				// cannot make the cancelled operation successful.
				return wallet.RecoveryTip{Hash: *chaincfg.MainNetParams.GenesisHash}, nil
			}}
			done := make(chan error, 1)
			go func() { done <- completePoolBackupRecovery(ctx, session, source) }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("bounded operation did not enter fixture")
			}
			cancel()
			select {
			case err := <-done:
				if err == nil || !exited.Load() {
					t.Fatal("cancelled restore succeeded or escaped active work", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled restore did not join")
			}
		})
	}
}

type poolRestoreCloser func() error

func (fn poolRestoreCloser) Close() error { return fn() }

func TestPoolBackupRestoreCleanupIsJoinedStickyAndPreservesScratch(t *testing.T) {
	scratch := filepath.Join(t.TempDir(), "owned-scratch")
	if err := os.Mkdir(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var order []string
	var runtimeCalls, sourceCalls atomic.Int32
	runtimeFailure, sourceFailure := errors.New("runtime close failed"), errors.New("source close failed")
	owner := &poolBackupRestore{owners: []io.Closer{
		poolRestoreCloser(func() error { sourceCalls.Add(1); order = append(order, "source"); return sourceFailure }),
		poolRestoreCloser(func() error { order = append(order, "session"); return nil }),
		poolRestoreCloser(func() error {
			if runtimeCalls.Add(1) > 1 {
				return nil // A hypothetical retry must never erase the first error.
			}
			close(started)
			<-release
			order = append(order, "runtime")
			return runtimeFailure
		}),
	}}
	var removed atomic.Bool
	remove := func(path string) error { removed.Store(true); return os.RemoveAll(path) }
	done, joined := make(chan error, 1), make(chan error, 1)
	go func() { done <- cleanupPoolBackupScratch(scratch, owner.Close, remove) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not enter owned closer")
	}
	go func() { joined <- owner.Close() }()
	select {
	case <-done:
		t.Fatal("scratch cleanup returned before owner joined")
	case <-joined:
		t.Fatal("concurrent Close returned before first Close joined")
	case <-time.After(20 * time.Millisecond):
	}
	if removed.Load() || sourceCalls.Load() != 0 {
		t.Fatal("cleanup deleted scratch or passed its blocked reverse-order owner")
	}
	releaseOnce.Do(func() { close(release) })
	var first error
	select {
	case first = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not finish")
	}
	select {
	case other := <-joined:
		if other != first {
			t.Fatal("concurrent Close changed the original sticky result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent Close did not join")
	}
	var retained *poolBackupRestoreCloseError
	if !errors.Is(first, runtimeFailure) || !errors.Is(first, sourceFailure) || !errors.As(first, &retained) || retained.owner != owner {
		t.Fatal("close error lost the exact owners or one cleanup failure", first)
	}
	if !reflect.DeepEqual(order, []string{"runtime", "session", "source"}) {
		t.Fatal("all acquired owners were not closed in reverse order", order)
	}
	if again := cleanupPoolBackupScratch(scratch, owner.Close, remove); again != first || runtimeCalls.Load() != 1 || sourceCalls.Load() != 1 || removed.Load() {
		t.Fatal("later cleanup retried or converted uncertain close into deletion", again)
	}
	if _, err := os.Stat(scratch); err != nil {
		t.Fatal("failed cleanup removed scratch", err)
	}
}

func TestPoolBackupRestoreRejectsCancelledRunBeforeCreatingScratch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	path := filepath.Join(t.TempDir(), "never-created")
	owner := &poolBackupRestore{}
	if owner.Run(ctx, path, "zero256", recovery.Export{}, prospectnode.RPCConfig{}) == nil {
		t.Fatal("cancelled construction started")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cancelled construction touched scratch", err)
	}
	if err := owner.Close(); err != nil || len(owner.owners) != 0 {
		t.Fatal("cancelled construction acquired an owner", err)
	}
}

func TestPoolBackupRestoreRPCUsesOnlyOwnedLiteralLoopback(t *testing.T) {
	good := prospectnode.RPCConfig{Version: 1, URL: "http://127.0.0.1:29332", Username: "published-fixture-user", Password: "published-fixture-password"}
	for _, suffix := range []string{"", "/"} {
		config := good
		config.URL += suffix
		got, err := poolBackupRecoveryRPC(config)
		if err != nil || got.URL != "ws://127.0.0.1:29332/ws" || got.Username != good.Username || got.Password != good.Password || len(got.CACertificate) != 0 {
			t.Fatal("owned source conversion changed its endpoint or credential")
		}
	}
	mutations := map[string]func(*prospectnode.RPCConfig){
		"wrong_version":       func(c *prospectnode.RPCConfig) { c.Version = 2 },
		"dns_alias":           func(c *prospectnode.RPCConfig) { c.URL = "http://localhost:29332" },
		"foreign_host":        func(c *prospectnode.RPCConfig) { c.URL = "http://192.0.2.1:29332" },
		"other_loopback":      func(c *prospectnode.RPCConfig) { c.URL = "http://127.0.0.2:29332" },
		"ipv6_alias":          func(c *prospectnode.RPCConfig) { c.URL = "http://[::1]:29332" },
		"missing_port":        func(c *prospectnode.RPCConfig) { c.URL = "http://127.0.0.1" },
		"foreign_path":        func(c *prospectnode.RPCConfig) { c.URL += "/foreign" },
		"encoded_path":        func(c *prospectnode.RPCConfig) { c.URL += "/%2f" },
		"query":               func(c *prospectnode.RPCConfig) { c.URL += "?source=foreign" },
		"empty_query":         func(c *prospectnode.RPCConfig) { c.URL += "?" },
		"fragment":            func(c *prospectnode.RPCConfig) { c.URL += "#foreign" },
		"userinfo":            func(c *prospectnode.RPCConfig) { c.URL = "http://other@127.0.0.1:29332" },
		"different_transport": func(c *prospectnode.RPCConfig) { c.URL = "https://127.0.0.1:29332" },
		"websocket_input":     func(c *prospectnode.RPCConfig) { c.URL = "ws://127.0.0.1:29332/ws" },
		"certificate":         func(c *prospectnode.RPCConfig) { c.CACertificate = "unexpected certificate" },
		"missing_user":        func(c *prospectnode.RPCConfig) { c.Username = "" },
		"missing_password":    func(c *prospectnode.RPCConfig) { c.Password = "" },
		"ambiguous_user":      func(c *prospectnode.RPCConfig) { c.Username = "other:identity" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			config := good
			mutate(&config)
			got, err := poolBackupRecoveryRPC(config)
			if err == nil || got.URL != "" || got.Username != "" || got.Password != "" || len(got.CACertificate) != 0 {
				t.Fatal("foreign configuration returned a usable recovery source")
			}
		})
	}
}
