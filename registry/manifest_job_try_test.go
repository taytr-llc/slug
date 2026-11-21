//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectjob"
)

func tryJobRows(t *testing.T, s *ManifestStore) [32]byte {
	t.Helper()
	h := sha256.New()
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		_, _ = h.Write(it.Key())
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(it.Value())
		_, _ = h.Write([]byte{0})
	}
	if it.Error() != nil {
		t.Fatal(it.Error())
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

func TestManifestTryJobOriginalReadOnlyAndNonblocking(t *testing.T) {
	f := readManifestCorpus(t)
	s, _ := manifestRawFixtureStore(t, f)
	want := f.Jobs[0].Job
	before := tryJobRows(t, s)
	s.now = func() time.Time { t.Fatal("immutable read consulted clock"); return time.Time{} }
	ctx := context.Background()
	got, err := s.TryGetManifestJob(ctx, want.ID)
	if err != nil || got != want {
		t.Fatal("independent original job", err)
	}
	got.PublicKey = "caller mutation"
	again, err := s.TryGetManifestJob(ctx, want.ID)
	if err != nil || again != want {
		t.Fatal("caller changed original job", err)
	}
	if got, e := s.TryGetManifestJob(ctx, strings.Repeat("0", 64)); got != (prospectjob.ManifestJob{}) || !errors.Is(e, ErrJobNotFound) {
		t.Fatal("missing job invented", e)
	}
	if got, e := s.TryGetManifestJob(ctx, "invalid"); got != (prospectjob.ManifestJob{}) || e == nil {
		t.Fatal("invalid job ID adopted", e)
	}
	s.mu.Lock()
	done := make(chan error, 1)
	go func() { _, e := s.TryGetManifestJob(ctx, want.ID); done <- e }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrBusy) {
			t.Error("original gate ignored", err)
		}
	case <-time.After(time.Second):
		s.mu.Unlock()
		<-done
		t.Fatal("job read waited behind original operation")
	}
	s.mu.Unlock()
	s.readOnly = true
	if got, err = s.TryGetManifestJob(ctx, want.ID); err != nil || got != want {
		t.Fatal("read-only immutable job", err)
	}
	if tryJobRows(t, s) != before {
		t.Fatal("read changed durable source")
	}
}

func TestManifestTryJobCancellationAndClosedState(t *testing.T) {
	f := readManifestCorpus(t)
	s, _ := manifestRawFixtureStore(t, f)
	id := f.Jobs[0].Job.ID
	before := tryJobRows(t, s)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := s.TryGetManifestJob(canceled, id); got != (prospectjob.ManifestJob{}) || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read disclosed job", err)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	if got, err := s.TryGetManifestJob(expired, id); got != (prospectjob.ManifestJob{}) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired read disclosed job", err)
	}
	if got, err := s.TryGetManifestJob(nil, id); got != (prospectjob.ManifestJob{}) || !errors.Is(err, ErrManifestAuthority) {
		t.Fatal("nil context", err)
	}
	s.poisoned = errors.New("fixture uncertain commit")
	if got, err := s.TryGetManifestJob(context.Background(), id); got != (prospectjob.ManifestJob{}) || !errors.Is(err, ErrPoisoned) {
		t.Fatal("poisoned read disclosed job", err)
	}
	s.poisoned = nil
	if tryJobRows(t, s) != before {
		t.Fatal("refusal changed durable source")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := s.TryGetManifestJob(context.Background(), id); got != (prospectjob.ManifestJob{}) || !errors.Is(err, ErrClosed) {
		t.Fatal("closed read disclosed job", err)
	}
	var absent *ManifestStore
	if got, err := absent.TryGetManifestJob(context.Background(), id); got != (prospectjob.ManifestJob{}) || !errors.Is(err, ErrClosed) {
		t.Fatal("nil owner", err)
	}
}
