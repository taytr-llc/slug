//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/prospect"
)

func operatorStoreVector(t *testing.T, family, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile("testdata/operator-reference/authority-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256.Sum256(raw); hex.EncodeToString(got[:]) != "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4" {
		t.Fatal("independent corpus changed")
	}
	var corpus struct {
		Positives []struct {
			Family, Name string
			Value        json.RawMessage
		}
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	for _, row := range corpus.Positives {
		if row.Family == family && row.Name == name {
			if err = json.Unmarshal(row.Value, target); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("independent case absent", family, name)
}
func operatorStoreProfile(t *testing.T) OperatorProfile {
	t.Helper()
	var pin PublicManifestPin
	operatorStoreVector(t, "pin", "v1/operator_v2_distinct", &pin)
	profile, err := NewOperatorProfile(pin)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
func operatorStoreOwner(t *testing.T, profile OperatorProfile) *operatorauthority.Owner {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]string)
	for _, role := range []string{"admission", "publication", "cutoff"} {
		// Public synthetic recipe from the independent input catalog. Never an
		// operator's real key, and the online owner has no checkpoint path.
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		paths[role] = filepath.Join(root, role)
		if err = os.WriteFile(paths[role], seed[:], 0600); err != nil {
			t.Fatal(err)
		}
	}
	binding, err := operatorBinding(profile)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := operatorauthority.OpenOnline(context.Background(), binding, operatorauthority.OnlineSeedFiles{Admission: paths["admission"], Publication: paths["publication"], Cutoff: paths["cutoff"]})
	if owner != nil {
		t.Cleanup(func() {
			if e := owner.Close(); e != nil {
				t.Error(e)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
func operatorStoreFixture(t *testing.T, profile OperatorProfile, owner *operatorauthority.Owner) (*ManifestStore, *ManifestPublication, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "operator")
	d, err := fighterdescriptor.ResolveCandidate(profile.Pin().Source.DescriptorID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := CreateOperatorManifestAuthority(path, d, profile, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	p, err := s.OperatorPublication(profile)
	if err != nil {
		t.Fatal(err)
	}
	return s, p, path
}
func operatorSignEntry(t *testing.T, owner *operatorauthority.Owner, profile OperatorProfile, entry PublicPublicationEntry) (*operatorauthority.Operation, PublicPublicationEntry) {
	t.Helper()
	binding, err := operatorBinding(profile)
	if err != nil {
		t.Fatal(err)
	}
	op, err := owner.Begin(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = op.Close() })
	raw, err := publicUnsigned(entry)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := op.SignPublication(raw)
	if err != nil {
		t.Fatal(err)
	}
	entry.Signature = hex.EncodeToString(sig[:])
	return op, entry
}

func TestOperatorManifestGuardsAndAudit(t *testing.T) {
	profile := operatorStoreProfile(t)
	owner := operatorStoreOwner(t, profile)
	s, p, path := operatorStoreFixture(t, profile, owner)
	if _, err := s.PublicPublication(profile.Pin()); !errors.Is(err, ErrPublicOwnership) {
		t.Fatal("legacy borrow accepted operator", err)
	}
	if _, err := os.Stat(filepath.Join(path, publicMarkerName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("v1 marker created", err)
	}
	raw, err := os.ReadFile(filepath.Join(path, "SLUG-MANIFEST-OWNERSHIP-V2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var guard publicGuard
	if err = json.Unmarshal(bytes.TrimSuffix(raw, []byte{'\n'}), &guard); err != nil || guard.Version != 2 || guard.Pin != profile.Pin() {
		t.Fatal("v2 guard", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if p.Alive() == nil {
		t.Fatal("closed source retained lifetime")
	}
	for _, open := range []func() (*ManifestStore, error){
		func() (*ManifestStore, error) { return OpenPublicManifestAuthority(path, profile.Pin()) },
		func() (*ManifestStore, error) { return OpenManifestAuthority(path, profile.Pin().Source, false) },
	} {
		got, err := open()
		if got != nil {
			got.Close()
		}
		if err == nil {
			t.Fatal("old open adopted v2")
		}
	}
	audit, err := OpenOperatorManifestAuthorityAudit(path, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	read, err := audit.OperatorPublication(profile)
	if err != nil || read.operator != nil {
		t.Fatal("audit acquired owner", err)
	}
	if _, err = read.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = read.PublishNext(context.Background()); !errors.Is(err, ErrReadOnly) {
		t.Fatal("audit publication", err)
	}
	if _, err = read.PrepareChainPublication(context.Background(), PublicChainReference{0, profile.Pin().Source.Genesis, publicZero}); !errors.Is(err, ErrReadOnly) {
		t.Fatal("audit staged", err)
	}
	if err = audit.Close(); err != nil {
		t.Fatal(err)
	}
	// Mixing guards is never treated as an upgrade or fallback.
	if err = os.WriteFile(filepath.Join(path, publicMarkerName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := OpenOperatorManifestAuthority(path, profile, owner); err == nil {
		got.Close()
		t.Fatal("mixed markers opened")
	}
	if err = os.Remove(filepath.Join(path, publicMarkerName)); err != nil {
		t.Fatal(err)
	}
	if got, err := OpenOperatorManifestAuthority(path, profile, nil); err == nil {
		got.Close()
		t.Fatal("missing online owner accepted")
	}
	fresh := filepath.Join(filepath.Dir(path), "must-not-create")
	d, _ := fighterdescriptor.ResolveCandidate(profile.Pin().Source.DescriptorID)
	if got, err := CreateOperatorManifestAuthority(fresh, d, profile, nil); err == nil {
		got.Close()
		t.Fatal("nil owner created root")
	}
	if _, err := os.Stat(fresh); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("refused constructor mutated root", err)
	}
	changed := profile.Pin()
	changed.PublicationID = strings.Repeat("83", 32)
	otherProfile, err := NewOperatorProfile(changed)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := OpenOperatorManifestAuthority(path, otherProfile, owner); err == nil {
		got.Close()
		t.Fatal("changed profile accepted original owner")
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := OpenOperatorManifestAuthority(path, profile, owner); err == nil {
		got.Close()
		t.Fatal("closed owner reopened writer")
	}
	readOnly, err := OpenOperatorManifestAuthorityAudit(path, profile)
	if err != nil {
		t.Fatal("audit depended on closed signer", err)
	}
	readOnly.Close()

}

func TestOperatorPreparedChainExactOwnerAndReopen(t *testing.T) {
	profile := operatorStoreProfile(t)
	owner := operatorStoreOwner(t, profile)
	s, p, path := operatorStoreFixture(t, profile, owner)
	ctx := context.Background()
	ref := PublicChainReference{0, profile.Pin().Source.Genesis, publicZero}
	prepared, err := p.PrepareChainPublication(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(prepared.Entry())
	if _, err = p.PublishNext(ctx); !errors.Is(err, ErrBusy) {
		t.Fatal("generic publisher signed shaped chain", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenOperatorManifestAuthority(path, profile, owner)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err = s.OperatorPublication(profile)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := p.PendingChainPublication(ctx)
	if err != nil || retained == nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(retained.Entry())
	if !bytes.Equal(got, original) {
		t.Fatal("reopen resampled pending body/time")
	}
	other := operatorStoreOwner(t, profile)
	wrongOp, wrongSigned := operatorSignEntry(t, other, profile, retained.Entry())
	if _, err = retained.CommitOperatorSigned(ctx, wrongOp, wrongSigned); !errors.Is(err, operatorauthority.ErrBinding) {
		t.Fatal("equal replacement owner accepted", err)
	}
	_ = wrongOp.Close()
	op, signed := operatorSignEntry(t, owner, profile, retained.Entry())
	if _, err = retained.CommitSigned(ctx, signed); !errors.Is(err, ErrPublicOwnership) {
		t.Fatal("legacy commit accepted operator", err)
	}
	if _, err = prepared.CommitOperatorSigned(ctx, op, signed); err == nil {
		t.Fatal("closed original handle revived")
	}
	changed := signed
	changed.AttestedAtMS++
	if _, err = retained.CommitOperatorSigned(ctx, op, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed unsigned intent", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = retained.CommitOperatorSigned(cancelled, op, signed); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled commit", err)
	}
	if _, err = retained.CommitOperatorSigned(ctx, op, signed); err != nil {
		t.Fatal(err)
	}
	_ = op.Close()
	if _, err = retained.CommitOperatorSigned(ctx, op, signed); err == nil {
		t.Fatal("ended operation accepted")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	audit, err := OpenOperatorManifestAuthorityAudit(path, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	read, err := audit.OperatorPublication(profile)
	if err != nil {
		t.Fatal(err)
	}
	published, err := read.Publication(ctx, 1)
	if err != nil || !reflect.DeepEqual(published, signed) {
		t.Fatal("exact durable publication", err)
	}
}

func TestOperatorManifestAdmissionOutboxAndSignerJoin(t *testing.T) {
	// The original independent normal-PoW admission corpus supplies both
	// existing admission paths. This is a lifecycle composition test; byte
	// oracles for v2 signatures/profile separation are in authority-v2.json.
	f := readManifestCorpus(t)
	base := operatorStoreProfile(t).Pin()
	base.Source = f.Pin
	base.ScanOrigin = PublicChainPoint{0, f.Pin.Genesis}
	profile, err := NewOperatorProfile(base)
	if err != nil {
		t.Fatal(err)
	}
	owner := operatorStoreOwner(t, profile)
	s, p, path := operatorStoreFixture(t, profile, owner)
	random, err := hex.DecodeString(f.Random)
	if err != nil {
		t.Fatal(err)
	}
	s.random = bytes.NewReader(random[32:])
	now := f.Issued
	s.now = func() time.Time { return time.UnixMilli(now) }
	for _, row := range f.Jobs {
		got, err := s.IssueManifestJob(row.Job.Context, row.Job.PublicKey, func() error { return nil })
		if err != nil || got != row.Job {
			t.Fatal("original job", err)
		}
	}
	for n, tc := range f.Cases[:2] {
		now = tc.Accepted
		var got ManifestRecord
		var inserted bool
		if n == 0 {
			got, inserted, err = s.RecordManifestJobChecked(tc.Proof, tc.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
				return tc.Eligibility, nil
			})
		} else {
			prepared, e := s.PrepareManifestAdmission(tc.Proof, tc.Authorization)
			err = e
			if err == nil {
				got, inserted, err = s.CommitManifestAdmission(prepared, tc.Eligibility, func() error { return nil })
			}
		}
		if err != nil || !inserted || !reflect.DeepEqual(got, tc.Record) {
			t.Fatal("original admission", n, err)
		}
	}
	if status, err := p.PublishNext(context.Background()); err != nil || status.SignedAdmissionSequence != 1 || !status.PublicEvidencePending {
		t.Fatal("first exact outbox", status, err)
	}
	first, err := p.Admission(context.Background(), 1)
	if err != nil || !reflect.DeepEqual(first.Record, f.Cases[0].Record) {
		t.Fatal("authenticated original record", err)
	}
	// Block the second final sync, then close the original signing owner.
	// Close must join the whole publication, and no post-close result escapes.
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	originalWrite := s.writeBatch
	s.writeBatch = func(batch *leveldb.Batch, opts *opt.WriteOptions) error {
		changes := publicBatchChanges{}
		if e := batch.Replay(changes); e != nil {
			return e
		}
		if _, yes := changes[string(publicCounterKey("ownership/publication/", 2))]; yes {
			close(entered)
			<-release
		}
		return originalWrite(batch, opts)
	}
	publicationDone := make(chan error, 1)
	go func() { _, e := p.PublishNext(context.Background()); publicationDone <- e }()
	joined := false
	defer func() {
		unblock()
		if !joined {
			<-publicationDone
		}
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("publication never reached sync")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- owner.Close() }()
	closeJoined := false
	defer func() {
		unblock()
		if !closeJoined {
			<-closeDone
		}
	}()
	binding, _ := operatorBinding(profile)
	deadline := time.Now().Add(time.Second)
	for owner.Check(context.Background(), binding) == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if owner.Check(context.Background(), binding) == nil {
		t.Fatal("owner Close did not fence")
	}
	select {
	case <-closeDone:
		closeJoined = true
		t.Fatal("owner closed before publication joined")
	default:
	}
	unblock()
	err = <-publicationDone
	joined = true
	if err == nil {
		t.Fatal("closed signer disclosed successful publication")
	}
	err = <-closeDone
	closeJoined = true
	if err != nil {
		t.Fatal(err)
	}
	s.writeBatch = originalWrite
	if _, err = p.PublishNext(context.Background()); err == nil {
		t.Fatal("closed owner reused")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// The sync may be durable even though disclosure was refused. Exact reopen
	// reads that result; it does not sign a new time/body or revive old handles.
	replacement := operatorStoreOwner(t, profile)
	s, err = OpenOperatorManifestAuthority(path, profile, replacement)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err = s.OperatorPublication(profile)
	if err != nil {
		t.Fatal(err)
	}
	status, err := p.Status(context.Background())
	if err != nil || status.SignedAdmissionSequence != 2 || status.PublicationSequence != 2 || status.PublicEvidencePending {
		t.Fatal("durable uncertain result", status, err)
	}
}

func TestOperatorCheckpointImportAndMissingCutoffRefusal(t *testing.T) {
	profile := operatorStoreProfile(t)
	owner := operatorStoreOwner(t, profile)
	s, p, _ := operatorStoreFixture(t, profile, owner)
	var checkpoint chaincfg.SignedCheckpoint
	operatorStoreVector(t, "checkpoint", "v1/operator_v2_distinct", &checkpoint)
	if _, err := p.PublishCheckpoint(context.Background(), checkpoint); err != nil {
		t.Fatal("public checkpoint import", err)
	}
	if owner.Capabilities().Checkpoint {
		t.Fatal("online checkpoint custody")
	}
	payload, _ := json.Marshal(PublicCutoffReference{CutoffID: strings.Repeat("71", 32)})
	if _, err := p.stage(context.Background(), "cutoff", payload, ""); err != nil {
		t.Fatal(err)
	}
	pending, err := p.PendingCutoffPublication(context.Background())
	if err != nil || pending == nil {
		t.Fatal(err)
	}
	meta := s.public.meta
	op, entry := operatorSignEntry(t, owner, profile, pending.Entry())
	defer op.Close()
	if _, err = pending.CommitOperatorCutoffSigned(context.Background(), op, entry, PublicCutoffRecord{}); err == nil {
		t.Fatal("missing signed cutoff accepted")
	}
	if s.public.meta != meta || pending.Check(context.Background()) != nil {
		t.Fatal("failed cutoff changed exact pending")
	}
}
