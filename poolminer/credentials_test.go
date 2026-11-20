// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"bytes"
	"context"
	"errors"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMinerAudienceRotationAndReopen(t *testing.T) {
	s, path, c, b, now := newMiner(t)
	ctx := minerContext(t)
	gym := c.Enrollments[0].GymID
	token := strings.Repeat("88", 32)
	other := strings.Repeat("99", 32)
	first, e := s.Rotate(ctx, gym, PoolAudience, 0, token, now+60000)
	if e != nil {
		t.Fatal(e)
	}
	before, e := s.db.Get(stateKey, nil)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.Rotate(ctx, gym, PoolAudience, 0, token, now+60000)
	if e != nil || retry != first {
		t.Fatal("lost response changed", e)
	}
	after, _ := s.db.Get(stateKey, nil)
	if !bytes.Equal(before, after) {
		t.Fatal("replay wrote")
	}
	if _, e = s.Rotate(ctx, gym, AdmissionAudience, 0, token, now+60000); !errors.Is(e, ErrConflict) {
		t.Fatal("cross-audience plaintext reuse", e)
	}
	if _, e = s.Rotate(ctx, gym, AdmissionAudience, 0, other, now+60000); e != nil {
		t.Fatal(e)
	}
	called := false
	fn := func(key string, check func() error) error {
		called = true
		if key != c.Enrollments[0].PublicKey {
			t.Error("key changed")
		}
		return check()
	}
	if e = s.AdmissionSessions().WithSession(ctx, token, fn); !errors.Is(e, ErrUnauthorized) || called {
		t.Fatal("audience crossed", e)
	}
	if e = s.PoolSessions().WithSession(ctx, other, fn); !errors.Is(e, ErrUnauthorized) || called {
		t.Fatal("audience crossed", e)
	}
	if e = s.PoolSessions().WithSession(ctx, token, fn); e != nil || !called {
		t.Fatal(e)
	}
	lease, e := mininglease.Capture(ctx, s.PoolSessions(), token)
	if e != nil {
		t.Fatal(e)
	}
	var escaped func() error
	if e = lease.Do(func(check func() error) error {
		escaped = check
		if _, e := s.Revoke(ctx, gym, PoolAudience, 1); !errors.Is(e, ErrBusy) {
			t.Error("revocation was not serialized", e)
		}
		if _, e := s.Rotate(ctx, gym, PoolAudience, 1, strings.Repeat("aa", 32), now+60000); !errors.Is(e, ErrBusy) {
			t.Error("rotation was not serialized", e)
		}
		return check()
	}); e != nil {
		t.Fatal(e)
	}
	if escaped() == nil {
		t.Fatal("escaped check remained active")
	}
	second, e := s.Rotate(ctx, gym, PoolAudience, 1, strings.Repeat("aa", 32), now+60000)
	if e != nil || second.Generation != 2 {
		t.Fatal(e)
	}
	if lease.Check() == nil {
		t.Fatal("old original lease revived")
	}
	revoked, e := s.Revoke(ctx, gym, PoolAudience, 2)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Revoke(ctx, gym, PoolAudience, 2)
	if e != nil || again != revoked {
		t.Fatal("revoke replay", e)
	}
	s = reopenMiner(t, s, path, c, b, now)
	if e = s.PoolSessions().WithSession(ctx, strings.Repeat("aa", 32), fn); e == nil {
		t.Fatal("revoked token reopened")
	}
	if _, e = s.Rotate(ctx, gym, PoolAudience, 2, token, now+60000); !errors.Is(e, ErrConflict) {
		t.Fatal("retired token reused", e)
	}
	if e = s.AdmissionSessions().WithSession(ctx, other, fn); e != nil {
		t.Fatal("independent audience lost", e)
	}
	raw, _ := s.db.Get(stateKey, nil)
	if bytes.Contains(raw, []byte(token)) || bytes.Contains(raw, []byte(other)) {
		t.Fatal("plaintext retained")
	}
	snapshot, e := s.Snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	snapshot.Config.Enrollments[0].PublicKey = "changed"
	snapshot.Credentials[0].TokenHash = "changed"
	if e = s.AdmissionSessions().WithSession(ctx, other, fn); e != nil {
		t.Fatal("DTO alias", e)
	}
}
func TestMinerCancellationExpiryAndJoinedClose(t *testing.T) {
	s, path, c, b, now := newMiner(t)
	ctx := minerContext(t)
	token := strings.Repeat("88", 32)
	gym := c.Enrollments[0].GymID
	if _, e := s.Rotate(ctx, gym, PoolAudience, 0, token, now+1000); e != nil {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	var escaped func() error
	if e := s.PoolSessions().WithSession(cancelled, token, func(_ string, check func() error) error {
		escaped = check
		cancel()
		if !errors.Is(check(), context.Canceled) {
			t.Error("parent cancellation missing")
		}
		return nil
	}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if escaped() == nil {
		t.Fatal("cancelled escaped check")
	}
	entered, release, done := make(chan func() error, 1), make(chan struct{}), make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		done <- s.PoolSessions().WithSession(ctx, token, func(_ string, check func() error) error { entered <- check; <-release; return check() })
	}()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		<-joined
	}()
	var check func() error
	select {
	case check = <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	short, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	e := s.Close(short)
	stop()
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("close did not join", e)
	}
	if !errors.Is(check(), ErrClosed) {
		t.Fatal("close did not invalidate")
	}
	if owner, e := Open(ctx, path, c, b); e == nil {
		owner.Close(ctx)
		t.Fatal("lock released before callback")
	}
	close(release)
	if e = <-done; e == nil {
		t.Fatal("closed callback disclosed")
	}
	<-joined
	if e = s.Close(ctx); e != nil {
		t.Fatal(e)
	}
	s = reopenMiner(t, s, path, c, b, now+1000)
	if e = s.PoolSessions().WithSession(ctx, token, func(string, func() error) error { t.Error("expired callback"); return nil }); !errors.Is(e, ErrUnauthorized) {
		t.Fatal(e)
	}
	s = reopenMiner(t, s, path, c, b, now+999)
	if e = s.PoolSessions().WithSession(ctx, token, func(string, func() error) error { return nil }); !errors.Is(e, ErrClock) {
		t.Fatal("durable clock rollback", e)
	}
}
func TestMinerLostAcknowledgementAndBoundedFailures(t *testing.T) {
	s, path, c, b, now := newMiner(t)
	ctx := minerContext(t)
	token := strings.Repeat("88", 32)
	gym := c.Enrollments[0].GymID
	child, cancel := context.WithCancel(ctx)
	s.checkpoint = func(stage string) {
		if stage == "committed" {
			cancel()
		}
	}
	if _, e := s.Rotate(child, gym, PoolAudience, 0, token, now+60000); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("lost ack hidden", e)
	}
	if _, e := s.Snapshot(ctx); !errors.Is(e, ErrPoisoned) {
		t.Fatal("uncertain owner reused", e)
	}
	s = reopenMiner(t, s, path, c, b, now)
	saved, e := s.Rotate(ctx, gym, PoolAudience, 0, token, now+60000)
	if e != nil || saved.Generation != 1 || saved.IssuedAtMS != now {
		t.Fatal("exact retry", e)
	}
	if _, e = s.Rotate(ctx, gym, PoolAudience, 0, token, now+60001); !errors.Is(e, ErrConflict) {
		t.Fatal("retry extended expiry", e)
	}
	if _, e = s.Rotate(ctx, gym, AdmissionAudience, 0, strings.Repeat("99", 32), now+MaxLifetimeMS+1); !errors.Is(e, ErrInvalid) {
		t.Fatal("unbounded lifetime", e)
	}
	s.state.Revision = ^uint32(0)
	raw, e := encodeState(s.state)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.db.Put(stateKey, raw, nil); e != nil {
		t.Fatal(e)
	}
	before, _ := s.db.Get(stateKey, nil)
	if _, e = s.Revoke(ctx, gym, PoolAudience, 1); !errors.Is(e, ErrCapacity) {
		t.Fatal("revision exhaustion", e)
	}
	after, _ := s.db.Get(stateKey, nil)
	if !bytes.Equal(before, after) {
		t.Fatal("capacity mutated")
	}
}
func TestMinerBindingAndOriginalFiles(t *testing.T) {
	c, b := minerBinding(t)
	if _, e := ConfigID(c, b); e != nil {
		t.Fatal(e)
	}
	changed := cloneConfig(c)
	changed.Enrollments[0].PayeeScript = "0014" + strings.Repeat("ab", 20)
	if ValidateConfig(changed, b) == nil {
		t.Fatal("payee substitution")
	}
	changed = cloneConfig(c)
	changed.Enrollments[0].PublicKey = c.Enrollments[1].PublicKey
	if ValidateConfig(changed, b) == nil {
		t.Fatal("key substitution")
	}
	other := b
	other.Source.PoolTarget = strings.Repeat("ab", 32)
	if ValidateConfig(c, other) == nil {
		t.Fatal("source substitution")
	}
	other = b
	other.Profile = registry.OperatorProfile{}
	if ValidateConfig(c, other) == nil {
		t.Fatal("zero profile")
	}
	if wire.TestPoW {
		return
	}
	s, path, c, b, now := newMiner(t)
	ctx := minerContext(t)
	mismatch := cloneConfig(c)
	mismatch.RuntimeID = strings.Repeat("ef", 32)
	if owner, e := Open(ctx, path, mismatch, b); e == nil {
		owner.Close(ctx)
		t.Fatal("foreign expected configuration")
	}
	original := filepath.Join(filepath.Dir(path), "original")
	if e := os.Rename(path, original); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Snapshot(ctx); !errors.Is(e, ErrPoisoned) {
		t.Fatal("root replacement undetected", e)
	}
	if e := s.Close(ctx); e != nil {
		t.Fatal(e)
	}
	if owner, e := Open(ctx, path, c, b); e == nil {
		owner.Close(ctx)
		t.Fatal("missing established owner recreated")
	}
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(original, path); e != nil {
		t.Fatal(e)
	}
	s = reopenMiner(t, s, path, c, b, now)
	if e := os.Chmod(filepath.Join(path, markerName), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Snapshot(ctx); !errors.Is(e, ErrPoisoned) {
		t.Fatal("marker permissions changed", e)
	}
}
func TestMinerRejectsCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	if _, e := Create(context.Background(), "", Config{}, Binding{}); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	var s Store
	if _, e := s.Snapshot(context.Background()); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}

func TestMinerUnknownWriteAndCallbackPanic(t *testing.T) {
	s, path, c, b, now := newMiner(t)
	ctx := minerContext(t)
	gym := c.Enrollments[0].GymID
	token := strings.Repeat("88", 32)
	write := s.write
	s.write = func(raw []byte) error {
		if e := write(raw); e != nil {
			return e
		}
		return errors.New("synthetic lost write acknowledgement")
	}
	if _, e := s.Rotate(ctx, gym, PoolAudience, 0, token, now+60000); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("unknown write hidden", e)
	}
	s = reopenMiner(t, s, path, c, b, now)
	if _, e := s.Rotate(ctx, gym, PoolAudience, 0, token, now+60000); e != nil {
		t.Fatal("written token not recovered", e)
	}
	var escaped func() error
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_ = s.PoolSessions().WithSession(ctx, token, func(_ string, check func() error) error { escaped = check; panic("synthetic callback panic") })
	}()
	if !panicked || escaped == nil || escaped() == nil {
		t.Fatal("panic failed to retire check")
	}
	if _, e := s.Revoke(ctx, gym, PoolAudience, 1); e != nil {
		t.Fatal("panic retained gate", e)
	}
}
