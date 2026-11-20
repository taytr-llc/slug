//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unsafe"

	"github.com/taytr-llc/slug/wire/v2"
)

// This fixture installs a modeled completed local replay state to isolate the
// read seam. It is not a claim that these transactions mined on a real chain.
// Authenticated public state literals come from the frozen independent corpus;
// full normal-node replay remains covered by separate actual acceptance.
func recoveryDiscoveryFixture(t *testing.T) (*Store, *HeadLease, *chainFake, EffectiveState, string) {
	t.Helper()
	s, source, _, _ := cutoffGenesisFixture(t)
	node, f := chainServer(t)
	corpus, _ := coreReference(t)
	var initial, final EffectiveState
	for _, c := range corpus.Positives {
		if c.Family != "state" {
			continue
		}
		if c.Name == "v1/zero256-unawarded/initial" {
			if e := json.Unmarshal(c.Value, &initial); e != nil {
				t.Fatal(e)
			}
		}
		if c.Name == "v1/zero256-unawarded/final" {
			if e := json.Unmarshal(c.Value, &final); e != nil {
				t.Fatal(e)
			}
		}
	}
	if initial.StateID == "" || final.OwnerPublicKey == "" {
		t.Fatal("missing inherited state")
	}
	s.mu.Lock()
	s.source, s.node, s.observation = source, node, f.observation
	s.head = HeadRef{Version: 1, PinID: s.pinID, ChainHeight: f.observation.Height, ChainHash: f.observation.Hash, ReplayDigest: strings.Repeat("7", 64)}
	s.ready = true
	s.commits = map[string]TransactionWitness{}
	s.states[initial.StateID] = initial
	s.portable[initial.StateID] = true
	s.canonical[1] = strings.Repeat("3", 64)
	s.canonical[2] = f.observation.Hash
	s.mu.Unlock()
	h, e := s.AcquireHead(context.Background(), s.head)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	return s, h, f, initial, final.OwnerPublicKey
}
func recoveryDiscoveryCommit(t *testing.T, s *Store, prior, recipient string, height, position uint32, tag byte) TransactionWitness {
	t.Helper()
	priorBytes, _ := hex.DecodeString(prior)
	recipientBytes, _ := hex.DecodeString(recipient)
	locator := append([]byte{0x6a, 0x4a}, []byte("SLUGFCM1\x01")...)
	locator = append(locator, priorBytes...)
	locator = append(locator, recipientBytes...)
	tx := wire.NewMsgTx(1)
	in := &wire.TxIn{Sequence: ^uint32(0)}
	in.PreviousOutPoint.Hash[0] = tag
	tx.AddTxIn(in)
	tx.AddTxOut(&wire.TxOut{Value: 500000, PkScript: []byte{0x51}})
	tx.AddTxOut(&wire.TxOut{PkScript: locator})
	tx.AddTxOut(&wire.TxOut{Value: 100000, PkScript: []byte{0x51}})
	var raw bytes.Buffer
	if e := tx.Serialize(&raw); e != nil {
		t.Fatal(e)
	}
	v := TransactionWitness{TxID: tx.TxHash().String(), Raw: hex.EncodeToString(raw.Bytes()), Height: height, BlockHash: s.canonical[height], TransactionIndex: position}
	s.commits[v.TxID] = v
	return v
}
func TestOwnershipRecoveryDiscoveryBoundedSortedExactCapture(t *testing.T) {
	s, h, _, prior, recipient := recoveryDiscoveryFixture(t)
	ctx := context.Background()
	b := recoveryDiscoveryCommit(t, s, prior.StateID, recipient, 2, 9, 1)
	a := recoveryDiscoveryCommit(t, s, prior.StateID, recipient, 1, 8, 2)
	c := recoveryDiscoveryCommit(t, s, prior.StateID, recipient, 2, 1, 3)
	// Unknown opaque predecessor is invalid in the complete reachable set.
	recoveryDiscoveryCommit(t, s, strings.Repeat("8", 64), recipient, 1, 0, 4)
	capture, e := s.CaptureRecoveryCommits(ctx, h, prior.OwnerPublicKey)
	if e != nil {
		t.Fatal(e)
	}
	if size := unsafe.Sizeof(recoveryCommitIndex{}); size > 64 || size*MaxRecoveryCommits*2 > 1024*1024 {
		t.Fatal("one old plus prospective index exceeded public cap", size)
	}
	page, e := s.RecoveryCommitPage(ctx, capture, 0, 2)
	if e != nil {
		t.Fatal(e)
	}
	if page.Complete || page.Next != 2 || len(page.Candidates) != 2 || page.Candidates[0].CommitTxID != a.TxID || page.Candidates[1].CommitTxID != c.TxID {
		t.Fatal("wrong sorted page", page)
	}
	page.Candidates[0].PriorStateID = "mutated public projection"
	page, e = s.RecoveryCommitPage(ctx, capture, 2, 2)
	if e != nil || !page.Complete || page.Next != 3 || len(page.Candidates) != 1 || page.Candidates[0].CommitTxID != b.TxID {
		t.Fatal("terminal page", page, e)
	}
	page, e = s.RecoveryCommitPage(ctx, capture, 3, 8)
	if e != nil || !page.Complete || len(page.Candidates) != 0 {
		t.Fatal("empty terminal", e)
	}
	for _, args := range [][2]uint32{{0, 0}, {0, 9}, {4, 8}} {
		if _, e = s.RecoveryCommitPage(ctx, capture, args[0], args[1]); !errors.Is(e, ErrInvalid) {
			t.Fatal("page bound", args, e)
		}
	}
	other, e := s.CaptureRecoveryCommits(ctx, h, recipient)
	if e != nil {
		t.Fatal(e)
	}
	if len(capture.entries) != 0 {
		t.Fatal("replaced capture retained its index")
	}
	if _, e = s.RecoveryCommitPage(ctx, capture, 0, 1); !errors.Is(e, ErrStale) {
		t.Fatal("old opaque slot revived", e)
	}
	page, e = s.RecoveryCommitPage(ctx, other, 0, 8)
	if e != nil || len(page.Candidates) != 0 || !page.Complete {
		t.Fatal("foreign gym", page, e)
	}
	other.Close()
	if _, e = s.RecoveryCommitPage(ctx, other, 0, 8); !errors.Is(e, ErrStale) {
		t.Fatal("closed capture revived", e)
	}
}
func TestOwnershipRecoveryDiscoveryGapsChronologyAndLifecycle(t *testing.T) {
	s, h, f, prior, recipient := recoveryDiscoveryFixture(t)
	ctx := context.Background()
	v := recoveryDiscoveryCommit(t, s, prior.StateID, recipient, 2, 1, 1)
	s.portable[prior.StateID] = false
	if _, e := s.CaptureRecoveryCommits(ctx, h, prior.OwnerPublicKey); !errors.Is(e, ErrUnavailable) {
		t.Fatal("known public evidence gap skipped", e)
	}
	s.portable[prior.StateID] = true
	old := s.states[prior.StateID]
	changed := old
	changed.Generation = 1
	changed.Evidence, _ = json.Marshal(FinalStateEvidence{SixtiethDescendantHeight: 2})
	s.states[prior.StateID] = changed
	c, e := s.CaptureRecoveryCommits(ctx, h, prior.OwnerPublicKey)
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.RecoveryCommitPage(ctx, c, 0, 8)
	if e != nil || len(p.Candidates) != 0 {
		t.Fatal("same-height final predecessor accepted", e)
	}
	s.states[prior.StateID] = old
	bad := v
	bad.Raw = "xx"
	s.commits[v.TxID] = bad
	if _, e = s.CaptureRecoveryCommits(ctx, h, prior.OwnerPublicKey); !errors.Is(e, ErrCorrupt) {
		t.Fatal("corrupt raw skipped", e)
	}
	s.commits[v.TxID] = v
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = s.CaptureRecoveryCommits(cancelled, h, prior.OwnerPublicKey); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel", e)
	}
	f.mu.Lock()
	f.observation.Revision = "2"
	f.mu.Unlock()
	if _, e = s.CaptureRecoveryCommits(ctx, h, prior.OwnerPublicKey); !errors.Is(e, ErrStale) {
		t.Fatal("fresh node ABA ignored", e)
	}
	f.mu.Lock()
	f.observation.Revision = "1"
	f.mu.Unlock()
	c, e = s.CaptureRecoveryCommits(ctx, h, prior.OwnerPublicKey)
	if e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	s.generation++
	s.mu.Unlock()
	if _, e = s.RecoveryCommitPage(ctx, c, 0, 8); !errors.Is(e, ErrStale) {
		t.Fatal("new generation", e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if len(c.entries) != 0 {
		t.Fatal("Close retained capture index")
	}
	if _, e = s.RecoveryCommitPage(ctx, c, 0, 8); !errors.Is(e, ErrClosed) {
		t.Fatal("closed store", e)
	}
}
func TestOwnershipRecoveryDiscoveryCapacityPrecedesTraversal(t *testing.T) {
	s, h, _, prior, _ := recoveryDiscoveryFixture(t)
	for n := 0; n <= MaxRecoveryCommits; n++ {
		s.commits[string(rune(n))] = TransactionWitness{Raw: "invalid"}
	}
	if _, e := s.CaptureRecoveryCommits(context.Background(), h, prior.OwnerPublicKey); !errors.Is(e, ErrCapacity) {
		t.Fatal("cap did not precede retained row walk", e)
	}
}
func TestOwnershipCanonicalEscrowAbsenceIsTypedOnlyAfterFreshChecks(t *testing.T) {
	s, _, f, prior, _ := recoveryDiscoveryFixture(t)
	ctx := context.Background()
	p := &PreparedRecovery{store: s, source: s.source, node: s.node, generation: s.generation, head: s.head, prior: prior, commit: TransactionWitness{TxID: strings.Repeat("4", 64), Height: 1, BlockHash: s.canonical[1]}}
	f.mu.Lock()
	f.txoutRaw = json.RawMessage("null")
	f.mu.Unlock()
	if e := p.Check(ctx); !errors.Is(e, ErrEscrowSpent) || !errors.Is(e, ErrUnavailable) {
		t.Fatal("exact canonical absence not distinguished", e)
	}
	for _, raw := range []string{`{}`, `{"value":0.005}`, `false`} {
		f.mu.Lock()
		f.txoutRaw = json.RawMessage(raw)
		f.mu.Unlock()
		if e := p.Check(ctx); !errors.Is(e, ErrUnavailable) || errors.Is(e, ErrEscrowSpent) {
			t.Fatal("missing/malformed evidence became spent", raw, e)
		}
	}
	f.mu.Lock()
	f.txoutRaw = json.RawMessage("null")
	f.malformedObservation = "null"
	f.mu.Unlock()
	if e := p.Check(ctx); !errors.Is(e, ErrUnavailable) || errors.Is(e, ErrEscrowSpent) {
		t.Fatal("null observation became output absence", e)
	}
	f.mu.Lock()
	f.malformedObservation = ""
	f.mu.Unlock()
	f.mu.Lock()
	f.txoutRaw = json.RawMessage("null")
	f.txoutAdvances = true
	f.mu.Unlock()
	if e := p.Check(ctx); !errors.Is(e, ErrStale) || errors.Is(e, ErrEscrowSpent) {
		t.Fatal("ABA became definite absence", e)
	}
}

func TestOwnershipEmptyRequiresPristineAuditedStore(t *testing.T) {
	s, _, _, path := cutoffGenesisFixture(t)
	ctx := context.Background()
	if empty, e := s.Empty(ctx); e != nil || !empty {
		t.Fatal("new store", empty, e)
	}
	s.gate <- struct{}{}
	if _, e := s.Empty(ctx); !errors.Is(e, ErrBusy) {
		t.Fatal("active mutation", e)
	}
	<-s.gate
	base := s.meta
	s.meta.Revision = 1
	if empty, e := s.Empty(ctx); e != nil || empty {
		t.Fatal("used but no current head", empty, e)
	}
	s.meta = base
	s.keySizes["work"] = 1
	if empty, e := s.Empty(ctx); e != nil || empty {
		t.Fatal("nonmetadata key", empty, e)
	}
	delete(s.keySizes, "work")
	s.meta.LogicalBytes++
	if _, e := s.Empty(ctx); !errors.Is(e, ErrCorrupt) {
		t.Fatal("logical byte mismatch", e)
	}
	s.meta = base
	pin, id := s.Pin(), s.Identity()
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Empty(ctx); !errors.Is(e, ErrClosed) {
		t.Fatal("closed", e)
	}
	reopened, e := OpenAudit(path, pin, id)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if empty, e := reopened.Empty(ctx); e != nil || !empty {
		t.Fatal("audited empty reopen", empty, e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := reopened.Empty(cancelled); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled", e)
	}
}
