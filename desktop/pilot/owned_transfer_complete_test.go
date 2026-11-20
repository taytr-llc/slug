// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

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
	"testing"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
)

// Use the independently produced complete mempool receipt and canonical state
// bytes, then open them through the real journal audit. The narrow action
// fixtures below intentionally have no wallet, node, or reconciliation owner;
// completed observation must not touch any of those mutation/network paths.
func completedObservationJournal(t *testing.T, prefix string) (*fightercarrier.Journal, fightercarrier.Operation, string) {
	t.Helper()
	if wire.TestPoW || !restoredSupported() {
		t.Skip("concrete journal requires normal supported profile")
	}
	raw, err := os.ReadFile("../../fightercarrier/testdata/reference/wallet-v1.json")
	sum := sha256.Sum256(raw)
	if err != nil || hex.EncodeToString(sum[:]) != "8648ac23d5e077df1a4dd1e3b8f07157922e650e92e9eea99c233a6993283495" {
		t.Fatal("independent complete input changed", err)
	}
	var reference struct {
		Positives []struct {
			Name  string
			Value json.RawMessage
		}
	}
	if err = json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	var identity fightercarrier.Identity
	var expected fightercarrier.Operation
	var marker, state []byte
	for _, c := range reference.Positives {
		if c.Name != prefix+"journal-marker" && c.Name != prefix+"stage-complete" && c.Name != prefix+"state-complete" {
			continue
		}
		var compact bytes.Buffer
		if err = json.Compact(&compact, c.Value); err != nil {
			t.Fatal(err)
		}
		switch c.Name {
		case prefix + "journal-marker":
			marker = append(compact.Bytes(), '\n')
			var m struct {
				JournalID string             `json:"journal_id"`
				Pin       fightercarrier.Pin `json:"pin"`
			}
			if err = json.Unmarshal(c.Value, &m); err != nil {
				t.Fatal(err)
			}
			identity = fightercarrier.Identity{JournalID: m.JournalID, Pin: m.Pin}
		case prefix + "stage-complete":
			if err = json.Unmarshal(c.Value, &expected); err != nil {
				t.Fatal(err)
			}
		case prefix + "state-complete":
			state = compact.Bytes()
		}
	}
	if len(state) == 0 || len(marker) == 0 || expected.Stage != "complete" || expected.SpendObservation == nil || expected.SpendObservation.State != "mempool" {
		t.Fatal("missing independent complete mempool receipt")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "journal")
	j, err := fightercarrier.Create(context.Background(), path, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(path, "SLUG-FIGHTER-CARRIER-JOURNAL.json"))
	if err != nil || !bytes.Equal(got, marker) {
		t.Fatal("exact independent marker", err)
	}
	if err = os.WriteFile(filepath.Join(path, "state.json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	j, err = fightercarrier.Open(context.Background(), path, identity)
	if err != nil {
		t.Fatal("real completed-state audit", err)
	}
	t.Cleanup(func() {
		if err := j.Close(); err != nil {
			t.Error(err)
		}
	})
	return j, expected, path
}

type completeObservationFile struct {
	Data           string
	Mode           os.FileMode
	Size, Modified int64
}

func completeObservationFiles(t *testing.T, path string) map[string]completeObservationFile {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]completeObservationFile)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			t.Fatal("journal file", err)
		}
		raw, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = completeObservationFile{string(raw), info.Mode(), info.Size(), info.ModTime().UnixNano()}
	}
	return out
}

func TestOwnedTransferCompletedObservePreservesReceiptAndOriginalView(t *testing.T) {
	j, want, path := completedObservationJournal(t, "zero256/hosted/transfer/one/")
	ctx := context.Background()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: strings.Repeat("a", 64), MaxLoginAttempts: 8, Accounts: gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:complete-observe", Subject: "zero256", Fixture: "zero256"}, {Issuer: "fixture:complete-observe", Subject: "incrementing256", Fixture: "incrementing256"}}, MaxSessions: 8, MaxChallenges: 8, MaxEvents: 32}}
	accounts, err := gymaccount.CreateIdentity(filepath.Join(base, "accounts"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := accounts.Close(); err != nil {
			t.Error(err)
		}
	})
	login := func(epoch uint64) gymaccount.Session {
		attempt, e := accounts.IssueLogin(ctx, "fixture:complete-observe")
		if e != nil {
			t.Fatal(e)
		}
		now := time.Now().UnixMilli()
		v, e := accounts.LoginWithAssertion(ctx, attempt.ID, gymaccount.VerifiedLogin{VerifierConfigHash: cfg.VerifierConfigHash, Issuer: "fixture:complete-observe", Subject: "zero256", Nonce: attempt.Nonce, AuthenticatedAtMS: now, IssuedAtMS: now, ExpiresAtMS: now + 300000, CredentialEpoch: epoch})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	action := func(login gymaccount.Session) *OwnedTransferActions {
		life, cancel := context.WithCancel(ctx)
		s := &OwnedWalletSession{setup: &OwnedWalletSetup{ctx: life, accounts: accounts}, entry: &ownedWalletEntry{carrier: j}, login: login, epoch: func(work context.Context, fn func(func() error) error) error { return fn(work.Err) }}
		s.intent.Binding.PrincipalID = login.Account.PrincipalID
		s.ready.Store(true)
		o := &OwnedTransferActions{session: s, life: life, cancel: cancel, viewID: strings.Repeat("b", 64)}
		t.Cleanup(func() {
			if e := o.Close(); e != nil {
				t.Error(e)
			}
		})
		return o
	}
	original := action(login(1))
	before := completeObservationFiles(t, path)
	for _, leg := range []string{"commit", "spend", "spend"} {
		got, e := original.Observe(ctx, want.RequestID, leg)
		if e != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("complete receipt changed", leg, e)
		}
		got.SpendObservation.State = "fabricated"
		got.SignedPackage.Spend.Raw = "fabricated"
	}
	if _, e := original.Observe(ctx, want.RequestID, "other"); !errors.Is(e, fightercarrier.ErrInvalid) {
		t.Fatal("wrong leg", e)
	}
	if _, e := original.Observe(ctx, strings.Repeat("f", 64), "spend"); !errors.Is(e, fightercarrier.ErrNotFound) {
		t.Fatal("missing request", e)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, e := original.Observe(cancelled, want.RequestID, "spend"); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled caller disclosed receipt", e)
	}
	now := time.Now().UnixMilli()
	reset := gymaccount.VerifiedReset{VerifierConfigHash: cfg.VerifierConfigHash, Issuer: "fixture:complete-observe", Subject: "zero256", EventID: strings.Repeat("c", 64), Stage: "begin", FromEpoch: 1, ToEpoch: 2, VerifiedAtMS: now, IssuedAtMS: now, ExpiresAtMS: now + 300000}
	if _, _, err = accounts.BeginReset(ctx, reset); err != nil {
		t.Fatal(err)
	}
	if _, e := original.Observe(ctx, want.RequestID, "spend"); e == nil {
		t.Fatal("revoked original generation disclosed receipt")
	}
	reset.Stage = "complete"
	if _, _, err = accounts.CompleteReset(ctx, reset); err != nil {
		t.Fatal(err)
	}
	heldLogin := login(2)
	if heldLogin.Account.HoldUntilMS <= time.Now().UnixMilli() {
		t.Fatal("fixture not held")
	}
	held := action(heldLogin)
	got, err := held.Observe(ctx, want.RequestID, "spend")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("held View could not read saved receipt", err)
	}
	// Loss of the original parent fence after begin but before disclosure must
	// still refuse an otherwise available immutable receipt.
	fenced := action(login(2))
	checks := 0
	fenced.session.epoch = func(work context.Context, fn func(func() error) error) error {
		checks++
		if checks == 2 {
			return gymaccount.ErrUnauthorized
		}
		return fn(work.Err)
	}
	if _, e := fenced.Observe(ctx, want.RequestID, "spend"); !errors.Is(e, gymaccount.ErrUnauthorized) || checks != 2 {
		t.Fatal("lost final original fence disclosed receipt", e, checks)
	}
	held.cancel()
	if _, e := held.Observe(ctx, want.RequestID, "spend"); e == nil {
		t.Fatal("cancelled original lifetime disclosed receipt")
	}
	closed := action(login(2))
	if err = closed.Close(); err != nil {
		t.Fatal(err)
	}
	if _, e := closed.Observe(ctx, want.RequestID, "spend"); e == nil {
		t.Fatal("closed action disclosed receipt")
	}
	if !reflect.DeepEqual(before, completeObservationFiles(t, path)) {
		t.Fatal("completed reads mutated durable journal or created staged rows")
	}
}

func TestRestoredCompletedObservePreservesReceiptWithoutNetwork(t *testing.T) {
	j, want, path := completedObservationJournal(t, "zero256/restored/recover_reveal/recovery/")
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := openRestoredRoot(filepath.Join(base, "restored"), true)
	if err != nil {
		t.Fatal(err)
	}
	life, cancel := context.WithCancel(context.Background())
	s := &RestoredGymSession{life: life, cancel: cancel, gate: make(chan struct{}, 1), root: root, journal: j}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	before := completeObservationFiles(t, path)
	expected := restoredActionStatus(want)
	for n := 0; n < 2; n++ {
		got, e := s.ObserveAction(context.Background(), want.RequestID, "spend")
		if e != nil || !reflect.DeepEqual(got, expected) {
			t.Fatal("restored completed receipt changed", e)
		}
		got.SpendObservation.State = "fabricated"
	}
	if _, e := s.ObserveAction(context.Background(), want.RequestID, "commit"); !errors.Is(e, fightercarrier.ErrConflict) {
		t.Fatal("recovery invented commit dispatch", e)
	}
	if _, e := s.ObserveAction(context.Background(), want.RequestID, "other"); !errors.Is(e, fightercarrier.ErrInvalid) {
		t.Fatal("wrong restored leg", e)
	}
	if _, e := s.ObserveAction(context.Background(), strings.Repeat("f", 64), "spend"); !errors.Is(e, fightercarrier.ErrNotFound) {
		t.Fatal("wrong restored request", e)
	}
	// A selected-path substitution cannot use the still-open original root.
	rootPath := filepath.Join(base, "restored")
	moved := rootPath + "-moved"
	if err = os.Rename(rootPath, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(rootPath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, e := s.ObserveAction(context.Background(), want.RequestID, "spend"); e == nil {
		t.Fatal("substituted root disclosed receipt")
	}
	if err = os.Remove(rootPath); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(moved, rootPath); err != nil {
		t.Fatal(err)
	}
	if s.node != nil || s.backend != nil || s.deliveryNode != nil || s.carrier != nil {
		t.Fatal("completed read acquired network or wallet mutation owner")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, e := s.ObserveAction(cancelled, want.RequestID, "spend"); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled restored caller disclosed receipt", e)
	}
	cancel()
	if _, e := s.ObserveAction(context.Background(), want.RequestID, "spend"); e == nil {
		t.Fatal("cancelled restored lifetime disclosed receipt")
	}
	if !reflect.DeepEqual(before, completeObservationFiles(t, path)) {
		t.Fatal("restored completed reads mutated durable journal")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, e := s.ObserveAction(context.Background(), want.RequestID, "spend"); e == nil {
		t.Fatal("closed restored session disclosed receipt")
	}
}
