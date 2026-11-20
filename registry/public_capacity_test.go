//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

// Counter injection selects an otherwise unreachable bound without fabricating
// signed history. The test proves classification/prewrite refusal, not an audited
// full-prefix store, checkpoint, or ownership authority.
func TestPublicCapacityClassificationAndPrewriteRefusal(t *testing.T) {
	s, p, _, pin := publicTestStore(t)
	ctx := context.Background()
	before := s.public.meta
	rawBefore, err := s.db.Get([]byte(publicMetaKey), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.public.meta.PublicationSequence = 16384
	ref := PublicChainReference{Height: 0, Hash: pin.Source.Genesis, PreviousChainEntryID: publicZero}
	if _, err = p.PrepareChainPublication(ctx, ref); err != ErrPublicCapacity || !errors.Is(err, ErrPublicOwnership) {
		t.Fatal("full prefix not specifically classified", err)
	}
	wrong := ref
	wrong.PreviousChainEntryID = strings.Repeat("51", 32)
	if _, err = p.PrepareChainPublication(ctx, wrong); err != ErrConflict || errors.Is(err, ErrPublicCapacity) {
		t.Fatal("full prefix masked exact predecessor conflict", err)
	}
	if _, err = p.prepareExternal(ctx, "chain", json.RawMessage(`{}`), nil); err != ErrPublicOwnership || errors.Is(err, ErrPublicCapacity) {
		t.Fatal("malformed payload became drain", err)
	}
	s.public.meta.PublicationSequence++
	if _, err = p.PrepareChainPublication(ctx, ref); err != ErrPublicOwnership {
		t.Fatal("overbound counter became capacity", err)
	}
	s.public.meta = before
	if got, err := s.db.Get([]byte(publicMetaKey), nil); err != nil || string(got) != string(rawBefore) {
		t.Fatal("refusal mutated public bytes", err)
	}
	if _, err = s.db.Get([]byte("ownership/pending"), nil); !errors.Is(err, leveldb.ErrNotFound) {
		t.Fatal("refusal staged an intent", err)
	}
	s.public.meta.AcceptedSequence = pin.Limits.Admissions
	if err = s.appendPublicAdmission(new(leveldb.Batch), ManifestRecord{Sequence: uint64(pin.Limits.Admissions) + 1}); err != ErrPublicCapacity {
		t.Fatal(err)
	}
	if err = s.appendPublicAdmission(new(leveldb.Batch), ManifestRecord{Sequence: uint64(pin.Limits.Admissions) + 2}); err != ErrPublicOwnership {
		t.Fatal("bad next sequence became capacity", err)
	}
	s.public.meta = before
}

func TestPublicCutoffCapacityChecksPredecessorAndStoredNamespaces(t *testing.T) {
	s, p, _, pin := publicTestStore(t)
	ctx := context.Background()
	lease, err := p.BeginBoundary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	before := s.public.meta
	defer func() { s.public.meta = before }()
	s.public.meta.CutoffCount = 256
	id, _ := PublicManifestPinID(pin)
	now := time.Now().UnixMilli()
	first := PublicBoundaryObservation{RequestStartMS: now - 2, ResponseEndMS: now - 1, InstanceID: strings.Repeat("71", 32), Revision: "1", Height: 0, Hash: pin.Source.Genesis}
	second := first
	second.RequestStartMS = now
	second.ResponseEndMS = now + 1
	status := lease.Status()
	intent := PublicCutoffIntent{Version: 1, PinID: id, EventID: strings.Repeat("72", 32), CutoffMS: now, First: first, Second: second, AcceptedAdmissionSequence: status.AcceptedSequence, PublicationSequence: status.PublicationSequence, PublicationHead: status.PublicationHead, FinalitySequence: 0, FinalityBodyHead: publicZero, PriorCutoffID: publicZero}
	wrong := intent
	wrong.PriorCutoffID = strings.Repeat("51", 32)
	if err = lease.CommitIntent(ctx, wrong); err != ErrConflict || errors.Is(err, ErrPublicCapacity) {
		t.Fatal("capacity hid previous cutoff conflict", err)
	}
	corrupt := publicIDKey("ownership/cutoff/", strings.Repeat("53", 32))
	if err = s.db.Put(corrupt, []byte("{"), nil); err != nil {
		t.Fatal(err)
	}
	if err = lease.CommitIntent(ctx, intent); err != ErrCorrupt || errors.Is(err, ErrPublicCapacity) {
		t.Fatal("capacity hid corrupt retained cutoff", err)
	}
	if err = s.db.Delete(corrupt, nil); err != nil {
		t.Fatal(err)
	}
	pendingKey := publicIDKey("ownership/cutoff-intent/", strings.Repeat("54", 32))
	pending := intent
	pending.EventID = strings.Repeat("54", 32)
	raw, _ := json.Marshal(pending)
	if err = s.db.Put(pendingKey, raw, nil); err != nil {
		t.Fatal(err)
	}
	if err = lease.CommitIntent(ctx, intent); err != ErrBusy || errors.Is(err, ErrPublicCapacity) {
		t.Fatal("capacity hid existing unpublished boundary", err)
	}
	if err = s.db.Delete(pendingKey, nil); err != nil {
		t.Fatal(err)
	}
	if err = lease.CommitIntent(ctx, intent); err != ErrPublicCapacity {
		t.Fatal("validated full cutoff count lost classification", err)
	}
	if _, err = s.db.Get(publicIDKey("ownership/cutoff-intent/", intent.EventID), nil); !errors.Is(err, leveldb.ErrNotFound) {
		t.Fatal("refused boundary was persisted", err)
	}
	// A real storage failure remains its original error; it cannot be converted
	// to a drain result just because the in-memory counter is at its cap.
	if err = s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = lease.CommitIntent(ctx, intent); !errors.Is(err, leveldb.ErrClosed) || errors.Is(err, ErrPublicCapacity) {
		t.Fatal("capacity hid storage failure", err)
	}
}

func TestPublicLogicalCapacityDoesNotMaskInvalidMetadata(t *testing.T) {
	s, _, _, pin := publicTestStore(t)
	// Adjust only the trusted cap in this private internal prewrite test, never
	// publish the changed pin or reopen it as a supported authority profile.
	limit := s.public.pin.Limits.LogicalBytes
	defer func() { s.public.pin.Limits.LogicalBytes = limit }()
	s.public.pin.Limits.LogicalBytes = s.public.meta.LogicalBytes + 8
	batch := new(leveldb.Batch)
	batch.Put([]byte("ownership/test-capacity"), make([]byte, 32))
	if _, err := s.stagePublicBatch(batch, s.meta); err != ErrPublicCapacity || !errors.Is(err, ErrPublicOwnership) {
		t.Fatal("logical exhaustion not classified", err)
	}
	bad := s.public.meta
	bad.SignedAdmissionSequence = bad.AcceptedSequence + 1
	raw, _ := json.Marshal(bad)
	batch.Put([]byte(publicMetaKey), raw)
	if _, err := s.stagePublicBatch(batch, s.meta); err != ErrPublicOwnership || errors.Is(err, ErrPublicCapacity) {
		t.Fatal("invalid metadata became drain", err)
	}
	if pin.Limits.LogicalBytes != limit {
		t.Fatal("fixture original cap lost")
	}
}
