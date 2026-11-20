//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

type operatorCoreFixture struct {
	profile   registry.OperatorProfile
	owner     *operatorauthority.Owner
	authority *registry.ManifestStore
	source    *PublicSource
	node      *OwnedChainSource
	store     *Store
	root      string
}

func operatorCoreProfile(t *testing.T) registry.OperatorProfile {
	t.Helper()
	p, e := registry.NewOperatorProfile(operatorCoreCorpus(t).Compositions["v1/operator_v2_distinct"].Pin)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func operatorCoreOwner(t *testing.T, profile registry.OperatorProfile) *operatorauthority.Owner {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	paths := map[string]string{}
	for _, role := range []string{"admission", "publication", "cutoff"} {
		// Independently frozen public synthetic recipes, never operator secrets.
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		paths[role] = filepath.Join(root, role)
		if e = os.WriteFile(paths[role], seed[:], 0600); e != nil {
			t.Fatal(e)
		}
	}
	binding, e := operatorBinding(profile)
	if e != nil {
		t.Fatal(e)
	}
	owner, e := operatorauthority.OpenOnline(context.Background(), binding, operatorauthority.OnlineSeedFiles{Admission: paths["admission"], Publication: paths["publication"], Cutoff: paths["cutoff"]})
	if owner != nil {
		t.Cleanup(func() {
			if e := owner.Close(); e != nil {
				t.Error(e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	return owner
}
func newOperatorCoreFixture(t *testing.T) *operatorCoreFixture {
	t.Helper()
	ctx := context.Background()
	f := &operatorCoreFixture{profile: operatorCoreProfile(t)}
	f.owner = operatorCoreOwner(t, f.profile)
	var e error
	f.root, e = filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	descriptor, e := fighterdescriptor.ResolveCandidate(f.profile.Pin().Source.DescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	f.authority, e = registry.CreateOperatorManifestAuthority(filepath.Join(f.root, "issuer"), descriptor, f.profile, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := f.authority.Close(); e != nil {
			t.Error(e)
		}
	})
	p, e := f.authority.OperatorPublication(f.profile)
	if e != nil {
		t.Fatal(e)
	}
	f.source, e = NewOperatorPublicSource(ctx, f.profile, p, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	// Reuse only the concrete RPC fixture's exact normal genesis bytes. The
	// separate legacy store is not copied or used as operator authority.
	_, _, oldNode, _ := cutoffGenesisFixture(t)
	f.node, e = NewOperatorOwnedChainSource(ctx, f.profile, prospectnode.RPCConfig{Version: 1, URL: oldNode.endpoint, Username: oldNode.username, Password: oldNode.password}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := f.node.Close(); e != nil {
			t.Error(e)
		}
	})
	f.store, e = CreateOperator(filepath.Join(f.root, "ownership"), f.profile, StoreIdentity{strings.Repeat("a1", 32)}, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := f.store.Close(); e != nil {
			t.Error(e)
		}
	})
	return f
}
func operatorCoreReplay(t *testing.T, s *Store, source *PublicSource, node *OwnedChainSource) Progress {
	t.Helper()
	r, e := s.BeginReplay(context.Background(), source, node)
	if e != nil {
		t.Fatal(e)
	}
	return boundedReplayFinish(t, r)
}
func TestOwnershipOperatorIssuerReplicaAndGuards(t *testing.T) {
	ctx := context.Background()
	f := newOperatorCoreFixture(t)
	s := f.store
	otherOwner := operatorCoreOwner(t, f.profile)
	if _, e := NewOperatorPublicSource(ctx, f.profile, f.source.publisher, otherOwner); e == nil {
		t.Fatal("same keys replaced original owner")
	}
	if _, e := NewPublicSource(ctx, f.profile.Pin(), f.source.publisher); e == nil {
		t.Fatal("legacy source accepted v2")
	}
	var e error
	f.source, _, e = s.RecordChainObservation(ctx, f.source, f.node)
	if e != nil {
		t.Fatal("operator observation", e)
	}
	progress := operatorCoreReplay(t, s, f.source, f.node)
	if progress.Head == nil || progress.Head.ChainHeight != 0 || progress.Head.PublicationSequence != 1 {
		t.Fatal("genesis head")
	}
	handler, e := NewOperatorPublicReadServer(f.profile, f.source.publisher, s, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	if _, e = NewPublicReader(ctx, f.profile.Pin(), PublicReadConfig{URL: server.URL + publicReadPath}, ctx); e == nil {
		t.Fatal("legacy reader accepted operator profile")
	}
	reader, e := NewOperatorPublicReader(ctx, f.profile, PublicReadConfig{URL: server.URL + "/slug/ownership/v2"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	remote, e := NewOperatorRemotePublicSource(ctx, f.profile, reader)
	if e != nil {
		t.Fatal(e)
	}
	replicaPath := filepath.Join(f.root, "replica")
	replicaID := StoreIdentity{strings.Repeat("a2", 32)}
	replica, e := CreateOperatorReplica(replicaPath, f.profile, replicaID)
	if e != nil {
		t.Fatal(e)
	}
	defer replica.Close()
	if _, e = replica.BeginReplay(ctx, f.source, f.node); e == nil {
		t.Fatal("replica borrowed local signing source")
	}
	if _, e = s.BeginReplay(ctx, remote, f.node); e == nil {
		t.Fatal("issuer reinterpreted as remote replica")
	}
	rebuilt := operatorCoreReplay(t, replica, remote, f.node)
	if rebuilt.Head == nil || *rebuilt.Head != *progress.Head || replica.operator != nil || len(replica.frames) != 0 || len(replica.frameBodies) != 0 {
		t.Fatal("public reconstruction authority mismatch")
	}
	if _, _, e = replica.RecordChainObservation(ctx, remote, f.node); e == nil {
		t.Fatal("replica signed")
	}
	if e = replica.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenOperator(replicaPath, f.profile, replicaID, f.owner); e == nil {
		t.Fatal("replica reopened as issuer")
	}
	audit, e := OpenOperatorAudit(replicaPath, f.profile, replicaID, OperatorReplica)
	if e != nil {
		t.Fatal(e)
	}
	if audit.operator != nil || !audit.readOnly {
		t.Fatal("audit retained signer")
	}
	if _, e = audit.Head(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("audit granted current readiness", e)
	}
	if e = audit.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenOperatorAudit(replicaPath, f.profile, replicaID, OperatorIssuer); e == nil {
		t.Fatal("audit adopted role from marker")
	}
	path := filepath.Join(f.root, "ownership")
	id := s.Identity()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(path, s.Pin(), id); e == nil {
		t.Fatal("legacy opened v2")
	}
	if _, e = OpenOperatorReplica(path, f.profile, id); e == nil {
		t.Fatal("issuer reopened as replica")
	}
	markerPath := filepath.Join(path, s.codec.markerName())
	originalMarker, e := os.ReadFile(markerPath)
	if e != nil {
		t.Fatal(e)
	}
	var changed storeMarker
	if e = json.Unmarshal(originalMarker, &changed); e != nil {
		t.Fatal(e)
	}
	changed.OperatorRole = OperatorReplica
	tampered, _ := json.Marshal(changed)
	if e = os.WriteFile(markerPath, append(tampered, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenOperatorReplica(path, f.profile, id); e == nil {
		t.Fatal("marker role bypassed durable metadata role")
	}
	if e = os.WriteFile(markerPath, originalMarker, 0600); e != nil {
		t.Fatal(e)
	}
	f.store, e = OpenOperator(path, f.profile, id, f.owner)
	if e != nil {
		t.Fatal("exact issuer reopen", e)
	}
	if _, e = f.store.Head(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("reopen promoted unaudited current head", e)
	}
	operatorCoreReplay(t, f.store, f.source, f.node)
	if e = f.owner.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Head(ctx); e == nil {
		t.Fatal("closed original owner substituted")
	}
}
func TestOwnershipOperatorPendingChainExactResume(t *testing.T) {
	ctx := context.Background()
	f := newOperatorCoreFixture(t)
	pending, e := f.source.publisher.PrepareChainPublication(ctx, registry.PublicChainReference{Height: 0, Hash: f.profile.Pin().Source.Genesis, PreviousChainEntryID: zeroID})
	if e != nil {
		t.Fatal(e)
	}
	original := pending.Entry()
	revision := f.store.meta.Revision
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, e = f.store.ResumeChainPublication(canceled, f.source, f.node); !errors.Is(e, context.Canceled) {
		t.Fatal("cancellation", e)
	}
	binding, e := operatorBinding(f.profile)
	if e != nil {
		t.Fatal(e)
	}
	op, e := f.owner.Begin(ctx, binding)
	if e != nil {
		t.Fatal(e)
	}
	_, _, busy := f.store.ResumeChainPublication(ctx, f.source, f.node)
	if e = op.Close(); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(busy, operatorauthority.ErrBusy) {
		t.Fatal("original owner slot was bypassed", busy)
	}
	if f.store.meta.Revision != revision {
		t.Fatal("failed signing mutated core")
	}
	if e = f.authority.Close(); e != nil {
		t.Fatal(e)
	}
	f.authority, e = registry.OpenOperatorManifestAuthority(filepath.Join(f.root, "issuer"), f.profile, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	publisher, e := f.authority.OperatorPublication(f.profile)
	if e != nil {
		t.Fatal(e)
	}
	f.source, e = NewOperatorPublicSource(ctx, f.profile, publisher, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	next, resumed, e := f.store.ResumeChainPublication(ctx, f.source, f.node)
	if e != nil || !resumed {
		t.Fatal("exact pending resume", e)
	}
	entry, e := publisher.Publication(ctx, original.Sequence)
	if e != nil {
		t.Fatal(e)
	}
	signature := entry.Signature
	entry.Signature = ""
	a, _ := json.Marshal(entry)
	b, _ := json.Marshal(original)
	if signature == "" || !bytes.Equal(a, b) {
		t.Fatal("retargeted pending bytes/time")
	}
	if _, e = f.store.Head(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("publication granted head", e)
	}
	again, resumed, e := f.store.ResumeChainPublication(ctx, next, f.node)
	if e != nil || resumed || again != next {
		t.Fatal("empty resume", e)
	}
}
func TestOwnershipOperatorBoundaryCutoffAndReplica(t *testing.T) {
	ctx := context.Background()
	f := newOperatorCoreFixture(t)
	operatorCoreReplay(t, f.store, f.source, f.node)
	boundary, e := f.store.CaptureBoundary(ctx, f.source, f.node, strings.Repeat("a3", 32), time.Now().Add(150*time.Millisecond).UnixMilli())
	if e != nil {
		t.Fatal(e)
	}
	codec, _ := f.profile.Codec()
	if e = codec.ValidateBoundary(boundary); e != nil {
		t.Fatal(e)
	}
	r, e := f.store.BeginCutoffReplay(ctx, f.source, f.node, boundary)
	if e != nil {
		t.Fatal(e)
	}
	historical := boundedReplayFinish(t, r)
	if historical.Cutoff == nil || codec.ValidateCutoff(*historical.Cutoff) != nil {
		t.Fatal("operator cutoff")
	}
	id, e := codec.CutoffID(*historical.Cutoff)
	if e != nil || id == zeroID {
		t.Fatal("cutoff ID", e)
	}
	prepared, e := f.source.publisher.PrepareCutoffPublication(ctx, *historical.Cutoff)
	if e != nil {
		t.Fatal(e)
	}
	pending := prepared.Entry()
	// Reopen loses current readiness but retains the exact signed historical body.
	if e = f.store.Close(); e != nil {
		t.Fatal(e)
	}
	f.store, e = OpenOperator(filepath.Join(f.root, "ownership"), f.profile, f.store.Identity(), f.owner)
	if e != nil {
		t.Fatal(e)
	}
	next, resumed, e := f.store.ResumeCutoffPublication(ctx, f.source)
	if e != nil || !resumed {
		t.Fatal("cutoff resume", e)
	}
	f.source = next
	entry, e := next.publisher.Publication(ctx, pending.Sequence)
	if e != nil {
		t.Fatal(e)
	}
	entry.Signature = ""
	a, _ := json.Marshal(entry)
	b, _ := json.Marshal(pending)
	if !bytes.Equal(a, b) {
		t.Fatal("cutoff reference changed")
	}
	if _, e = f.store.Head(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("signed reference granted readiness", e)
	}
	operatorCoreReplay(t, f.store, f.source, f.node)
	handler, e := NewOperatorPublicReadServer(f.profile, f.source.publisher, f.store, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	reader, e := NewOperatorPublicReader(ctx, f.profile, PublicReadConfig{URL: server.URL + "/slug/ownership/v2"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	remote, e := NewOperatorRemotePublicSource(ctx, f.profile, reader)
	if e != nil {
		t.Fatal(e)
	}
	replica, e := CreateOperatorReplica(filepath.Join(f.root, "replica"), f.profile, StoreIdentity{strings.Repeat("a4", 32)})
	if e != nil {
		t.Fatal(e)
	}
	defer replica.Close()
	operatorCoreReplay(t, replica, remote, f.node)
	r, e = replica.BeginCutoffReplay(ctx, remote, f.node, boundary)
	if e != nil {
		t.Fatal(e)
	}
	restored := boundedReplayFinish(t, r)
	if restored.Cutoff == nil || *restored.Cutoff != *historical.Cutoff || replica.operator != nil || len(replica.frames) != 0 {
		t.Fatal("remote cutoff changed or signed")
	}
}
func TestOwnershipOperatorCloseJoinsBoundaryCommit(t *testing.T) {
	ctx := context.Background()
	f := newOperatorCoreFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var call sync.WaitGroup
	defer func() { once.Do(func() { close(release) }); call.Wait() }()
	write := f.store.writeBatch
	f.store.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error { close(entered); <-release; return write(b, o) }
	result := make(chan error, 1)
	call.Add(1)
	go func() {
		defer call.Done()
		_, e := f.store.CaptureBoundary(ctx, f.source, f.node, strings.Repeat("a5", 32), time.Now().Add(150*time.Millisecond).UnixMilli())
		result <- e
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("boundary commit did not enter")
	}
	closed := make(chan error, 1)
	call.Add(1)
	go func() { defer call.Done(); closed <- f.owner.Close() }()
	binding, _ := operatorBinding(f.profile)
	deadline := time.Now().Add(5 * time.Second)
	for f.owner.Check(ctx, binding) == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.owner.Check(ctx, binding) == nil {
		t.Fatal("owner did not close admission")
	}
	select {
	case e := <-closed:
		t.Fatal("owner did not join signed commit", e)
	default:
	}
	once.Do(func() { close(release) })
	select {
	case e := <-result:
		if e == nil {
			t.Fatal("closed signer disclosed boundary")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("boundary did not join")
	}
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner did not join")
	}
}

func TestOwnershipOperatorPublicAuditHasNoSigner(t *testing.T) {
	ctx := context.Background()
	f := newOperatorCoreFixture(t)
	var e error
	f.source, _, e = f.store.RecordChainObservation(ctx, f.source, f.node)
	if e != nil {
		t.Fatal(e)
	}
	operatorCoreReplay(t, f.store, f.source, f.node)
	if e = f.store.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.authority.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.owner.Close(); e != nil {
		t.Fatal(e)
	}
	audit, e := OpenOperatorAudit(filepath.Join(f.root, "ownership"), f.profile, f.store.Identity(), OperatorIssuer)
	if e != nil {
		t.Fatal(e)
	}
	defer audit.Close()
	public, e := registry.OpenOperatorManifestAuthorityAudit(filepath.Join(f.root, "issuer"), f.profile)
	if e != nil {
		t.Fatal(e)
	}
	defer public.Close()
	publisher, e := public.OperatorPublication(f.profile)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = NewOperatorPublicSource(ctx, f.profile, publisher, f.owner); e == nil {
		t.Fatal("audit attached closed owner")
	}
	handler, e := NewOperatorPublicReadServer(f.profile, publisher, audit, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	reader, e := NewOperatorPublicReader(ctx, f.profile, PublicReadConfig{URL: server.URL + "/slug/ownership/v2"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	status, e := reader.Status(ctx)
	if e != nil || status.Version != 2 || status.Publication.PublicationSequence != 1 {
		t.Fatal("ownerless public status", e)
	}
	if _, e = reader.Publication(ctx, 1); e != nil {
		t.Fatal("ownerless signed bytes", e)
	}
	if _, e = publisher.PublishNext(ctx); e == nil {
		t.Fatal("audit acquired publication authority")
	}
	if _, e = audit.Head(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("audit acquired current head", e)
	}
}
func TestOwnershipOperatorReplicaCannotPublishConditionalFinality(t *testing.T) {
	// Modeled final branch only: this is the final disclosure refusal, not proof
	// that these synthetic non-genesis hashes satisfy normal work.
	ctx := context.Background()
	f := newOperatorCoreFixture(t)
	handler, e := NewOperatorPublicReadServer(f.profile, f.source.publisher, f.store, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	reader, e := NewOperatorPublicReader(ctx, f.profile, PublicReadConfig{URL: server.URL + "/slug/ownership/v2"}, ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	source, e := NewOperatorRemotePublicSource(ctx, f.profile, reader)
	if e != nil {
		t.Fatal(e)
	}
	replica, e := CreateOperatorReplica(filepath.Join(f.root, "replica"), f.profile, StoreIdentity{strings.Repeat("a6", 32)})
	if e != nil {
		t.Fatal(e)
	}
	defer replica.Close()
	c := operatorCoreCorpus(t).Compositions["v1/operator_v2_distinct"]
	replica.admissions[1] = c.Admission
	b := &replayBranch{target: ReplayTarget{AdmissionSequence: 1, AdmissionHead: c.State.BirthAdmissionID, ChainHeight: 160, ChainHash: strings.Repeat("52", 32)}, effective: map[string]EffectiveState{c.State.ProspectID: c.State}, conflicted: map[string]EffectiveState{}}
	r := &Replay{store: replica, source: source, branches: []*replayBranch{b}, branch: 1, official: map[string]EffectiveState{}}
	revision := replica.meta.Revision
	if _, e = r.finish(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal("replica fabricated finality", e)
	}
	if replica.meta.Revision != revision || replica.meta.CurrentHead != nil || len(replica.frames) != 0 || len(replica.frameBodies) != 0 || replica.operator != nil {
		t.Fatal("refusal changed public authority")
	}
}
