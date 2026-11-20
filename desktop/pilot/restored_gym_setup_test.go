package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

func restoredSetupFixture(t *testing.T) RestoredGymConfig {
	t.Helper()
	if wire.TestPoW || !restoredSupported() {
		t.Skip("normal supported profile")
	}
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("testdata/restored-gym/reference/setup-v1.json")
	if e != nil || restoredSHA(raw) != "3d31e7f195fa20456978739258f5517fbe2a4d97d93b379c9b8ade941daa3147" {
		t.Fatal("reference", e)
	}
	var corpus struct {
		Contexts []struct {
			Name string
			Init restoredSetupInit
		}
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("reference parse")
	}
	var initial restoredSetupInit
	for _, c := range corpus.Contexts {
		if c.Name == "zero256/base" {
			initial = c.Init
		}
	}
	if initial.Version != 1 {
		t.Fatal("reference case absent")
	}
	var entropy [32]byte
	bundle, e := recovery.NewExport(entropy, initial.OriginalManifest.Accounts)
	if e != nil {
		t.Fatal(e)
	}
	raw, e = recovery.MarshalExport(bundle)
	if e != nil {
		t.Fatal(e)
	}
	c := RestoredGymConfig{Directory: filepath.Join(dir, "owner"), RecoveryFile: filepath.Join(dir, "selected.json"), PublicConfigFile: filepath.Join(dir, "public.json"), NodeConfigFile: filepath.Join(dir, "node.json")}
	if e = os.WriteFile(c.RecoveryFile, raw, 0600); e != nil {
		t.Fatal(e)
	}
	clear(raw)
	raw, e = json.Marshal(restoredPublicConfig{Version: 1, Ownership: initial.Ownership, PublicRead: initial.PublicRead})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(c.PublicConfigFile, raw, 0600); e != nil {
		t.Fatal(e)
	}
	raw, e = json.Marshal(prospectnode.RPCConfig{Version: 1, URL: initial.NodeBinding.URL, Username: "published-fixture", Password: "published-fixture"})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(c.NodeConfigFile, raw, 0600); e != nil {
		t.Fatal(e)
	}
	clear(raw)
	return c
}

func TestRestoredGymSetupPendingResumeAndMissingStore(t *testing.T) {
	c := restoredSetupFixture(t)
	ctx := context.Background()
	s, e := CreateRestoredGymSession(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	initial := s.initial
	before, e := s.Status(ctx)
	if e != nil || before.Phase != "pending" || before.Recovery == nil {
		t.Fatal(before, e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenRestoredGymSession(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	after, e := s.Status(ctx)
	if e != nil || !restoredEqual(before, after) || !restoredEqual(initial, s.initial) {
		t.Fatal("resume drift", e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	selected, e := os.ReadFile(c.RecoveryFile)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(c.RecoveryFile, append(append([]byte{}, selected...), '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if opened, e := OpenRestoredGymSession(ctx, c); e == nil {
		opened.Close()
		t.Fatal("changed exact selected bytes accepted")
	}
	if e = os.WriteFile(c.RecoveryFile, selected, 0600); e != nil {
		t.Fatal(e)
	}
	clear(selected)
	if e = os.Rename(filepath.Join(c.Directory, "carrier"), filepath.Join(filepath.Dir(c.Directory), "saved-carrier")); e != nil {
		t.Fatal(e)
	}
	if opened, e := OpenRestoredGymSession(ctx, c); e == nil {
		opened.Close()
		t.Fatal("missing paired journal accepted")
	}
	if _, e = os.Lstat(filepath.Join(c.Directory, "carrier")); !os.IsNotExist(e) {
		t.Fatal("missing journal recreated")
	}
}

// This source supplies the unchanged public normal genesis only. These tests
// exercise real schema12/15 storage, not network or ownership replay truth.
type restoredGenesisSource struct{}

func (restoredGenesisSource) Identity(ctx context.Context) (wallet.RecoverySourceIdentity, error) {
	return wallet.RecoverySourceIdentity{Network: uint32(chaincfg.MainNetParams.Net), Genesis: *chaincfg.MainNetParams.GenesisHash, CSVHeight: 62, SegwitHeight: 62, TaprootHeight: 62}, ctx.Err()
}
func (restoredGenesisSource) BestBlock(ctx context.Context) (wallet.RecoveryTip, error) {
	return wallet.RecoveryTip{Hash: *chaincfg.MainNetParams.GenesisHash}, ctx.Err()
}
func (restoredGenesisSource) BlockHash(ctx context.Context, height int32) (chainhash.Hash, error) {
	if height != 0 {
		return chainhash.Hash{}, errors.New("unexpected fixture height")
	}
	return *chaincfg.MainNetParams.GenesisHash, ctx.Err()
}
func (restoredGenesisSource) Block(ctx context.Context, hash chainhash.Hash) (*wire.MsgBlock, error) {
	if hash != *chaincfg.MainNetParams.GenesisHash {
		return nil, errors.New("unexpected fixture hash")
	}
	var b bytes.Buffer
	_ = chaincfg.MainNetParams.GenesisBlock.Serialize(&b)
	var block wire.MsgBlock
	if e := block.Deserialize(bytes.NewReader(b.Bytes())); e != nil {
		return nil, e
	}
	return &block, ctx.Err()
}

func TestRestoredGymSetupActivationCrashWindows(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "before-schema15"
		if committed {
			name = "after-schema15"
		}
		t.Run(name, func(t *testing.T) {
			c := restoredSetupFixture(t)
			ctx := context.Background()
			s, e := CreateRestoredGymSession(ctx, c)
			if e != nil {
				t.Fatal(e)
			}
			for n := 0; n < 512; n++ {
				p, e := s.pending.Status(ctx)
				if e != nil {
					t.Fatal(e)
				}
				if p.Ready {
					break
				}
				if _, e = s.pending.Step(ctx, restoredGenesisSource{}); e != nil {
					t.Fatal(e)
				}
			}
			p, e := s.pending.Status(ctx)
			if e != nil || !p.Ready {
				t.Fatal("bounded fixture did not finish", e)
			}
			if e = s.pending.Close(); e != nil {
				t.Fatal(e)
			}
			s.pending = nil
			guard, e := s.loader.InspectRestoredCarrierActivation(ctx, []byte(fixtureWalletPublicPass), s.initial.OriginalManifest, s.journal)
			if e != nil {
				t.Fatal(e)
			}
			digest, e := waddrmgr.RestoredCarrierGuardDigest(guard)
			if e != nil {
				t.Fatal(e)
			}
			a := restoredSetupActivate{1, s.initial.SetupID, restoredDigest("setup", s.initial), guard, digest}
			if e = s.publishMarker(ctx, restoredActivateName, a, restoredActivateLimit); e != nil {
				t.Fatal(e)
			}
			s.activation = &a
			if committed {
				s.installed, e = s.loader.ActivateRestoredCarrierWalletExpected(ctx, []byte(fixtureWalletPublicPass), guard, s.journal)
				if e != nil {
					t.Fatal(e)
				}
			}
			initial := s.initial
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			if _, e = os.Lstat(filepath.Join(c.Directory, restoredReadyName)); !os.IsNotExist(e) {
				t.Fatal("READY prepublished")
			}
			s, e = OpenRestoredGymSession(ctx, c)
			if e != nil {
				t.Fatal("resume exact ACTIVATE", e)
			}
			if s.ready == nil || s.installed.Manager.CarrierSchema() != 15 || !restoredEqual(s.initial, initial) || !restoredEqual(s.activation.Guard, guard) {
				t.Fatal("guard/identity drift")
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			c.RecoveryFile = ""
			s, e = OpenRestoredGymSession(ctx, c)
			if e != nil {
				t.Fatal("READY needs no phrase file", e)
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			if e = os.Rename(filepath.Join(c.Directory, "wallet"), filepath.Join(filepath.Dir(c.Directory), "saved-wallet")); e != nil {
				t.Fatal(e)
			}
			if opened, e := OpenRestoredGymSession(ctx, c); e == nil {
				opened.Close()
				t.Fatal("missing ready wallet accepted")
			}
		})
	}
}

func TestRestoredGymSelectedFileAndRootRefusals(t *testing.T) {
	if !restoredSupported() {
		t.Skip("supported platform")
	}
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	p := filepath.Join(dir, "input")
	if e = os.WriteFile(p, []byte("public test"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(dir, "alias")
	if e = os.Link(p, link); e != nil {
		t.Fatal(e)
	}
	if _, e = readRestoredSelected(ctx, p, filepath.Join(dir, "owner"), 65536); e == nil {
		t.Fatal("hardlink accepted")
	}
	if e = os.Remove(link); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(p, link); e != nil {
		t.Fatal(e)
	}
	if _, e = readRestoredSelected(ctx, link, filepath.Join(dir, "owner"), 65536); e == nil {
		t.Fatal("symlink accepted")
	}
	r, e := openRestoredRoot(filepath.Join(dir, "owner"), true)
	if e != nil {
		t.Fatal(e)
	}
	if again, e := openRestoredRoot(filepath.Join(dir, "owner"), false); e == nil {
		again.Close()
		t.Fatal("root owner not exclusive")
	}
	if e = os.WriteFile(filepath.Join(dir, "owner", "unexpected"), []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = r.inventory(); e == nil {
		t.Fatal("unknown root member accepted")
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = readRestoredSelected(ctx, p, dir, 65536); e == nil {
		t.Fatal("input overlaps mutable owner")
	}
}

type restoredFailingCloser struct{ cause error }

func (c restoredFailingCloser) Close() error { return c.cause }

type restoredCountedCloser struct {
	calls int
	cause error
}

func (c *restoredCountedCloser) Close() error { c.calls++; return c.cause }

func TestRestoredGymFileCleanupRetainsExactFirstFailure(t *testing.T) {
	cause := errors.New("controlled first close failure")
	failed := &restoredCountedCloser{cause: cause}
	joined := &restoredCountedCloser{}
	e := restoredClose(ErrRestoredGymStorage, failed, joined)
	var owner *RestoredGymFileCleanupError
	if !errors.As(e, &owner) || !errors.Is(e, cause) || failed.calls != 1 || joined.calls != 1 || len(owner.owners) != 1 || owner.owners[0] != failed {
		t.Fatal("lost exact cleanup owner")
	}
	if owner.Close() != e || owner.Close() != e || failed.calls != 1 {
		t.Fatal("retried or laundered close")
	}
}

func TestRestoredGymCloseJoinsAndRetainsStickyOwner(t *testing.T) {
	if !restoredSupported() {
		t.Skip("supported platform")
	}
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	root, e := openRestoredRoot(filepath.Join(dir, "owner"), true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = root.Close() })
	life, cancel := context.WithCancel(context.Background())
	s := &RestoredGymSession{life: life, cancel: cancel, gate: make(chan struct{}, 1), root: root}
	failed := errors.New("controlled subordinate close failure")
	failedOwner := restoredClose(nil, restoredFailingCloser{failed})
	s.rememberCleanup(errors.Join(ErrRestoredGymStorage, failedOwner))
	if !s.poisoned.Load() || len(s.cleanup) != 1 {
		t.Fatal("joined cleanup owner was not adopted")
	}
	s.gate <- struct{}{}
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case <-life.Done():
	case <-time.After(time.Second):
		t.Fatal("close did not cancel")
	}
	select {
	case <-done:
		t.Fatal("close did not join active work")
	default:
	}
	<-s.gate
	var first error
	select {
	case first = <-done:
	case <-time.After(time.Second):
		t.Fatal("close did not finish")
	}
	var cleanup *RestoredGymCleanupError
	if !errors.As(first, &cleanup) || !errors.Is(first, failed) || s.Close() != first || cleanup.Close() != first {
		t.Fatal("sticky cleanup owner lost", first)
	}
	if second, e := openRestoredRoot(filepath.Join(dir, "owner"), false); e == nil {
		second.Close()
		t.Fatal("uncertain subordinate released root lock")
	}
}
